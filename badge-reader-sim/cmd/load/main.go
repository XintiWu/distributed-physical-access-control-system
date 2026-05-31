// Shift-change load generator: simulates many employees swiping IN at once.
// Use --unique-users for 90k distinct UUIDs (no cardUid → no card master-data required).
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"sort"
	"sync"
	"sync/atomic"
	"time"
)

type swipeRequest struct {
	UserID    string `json:"userId"`
	DoorID    string `json:"doorId"`
	Direction string `json:"direction"`
	Timestamp string `json:"timestamp"`
}

type swipeResponse struct {
	Decision string  `json:"decision"`
	Reason   *string `json:"reason"`
}

var demoUsers = []string{
	"22222222-2222-2222-2222-222222222222",
	"cccccccc-cccc-cccc-cccc-cccccccccccc",
	"bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb",
	"aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa",
}

// loadUserUUID returns a deterministic RFC-4122-shaped UUID for load index i (0 .. 89999+).
func loadUserUUID(i int) string {
	return fmt.Sprintf("00000000-0000-4000-a000-%012x", i)
}

func main() {
	api := flag.String("api", "http://localhost:8080", "Access API base URL")
	door := flag.String("door", "11111111-1111-1111-1111-111111111111", "Door UUID")
	count := flag.Int("count", 90000, "Total swipe requests")
	workers := flag.Int("workers", 150, "Concurrent workers (150–200 typical for local Docker)")
	direction := flag.String("direction", "IN", "IN (shift enter) or OUT")
	uniqueUsers := flag.Bool("unique-users", true, "One distinct userId per request index (90k people simulation)")
	ramp := flag.Duration("ramp", 0, "Spread requests evenly over this duration (0 = burst)")
	flag.Parse()

	if *count < 1 {
		fmt.Fprintln(os.Stderr, "count must be >= 1")
		os.Exit(1)
	}

	url := fmt.Sprintf("%s/access/swipe", *api)
	transport := &http.Transport{
		MaxIdleConns:        *workers + 10,
		MaxIdleConnsPerHost: *workers + 10,
		IdleConnTimeout:     90 * time.Second,
		DialContext: (&net.Dialer{
			Timeout:   5 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
	}
	client := &http.Client{Timeout: 15 * time.Second, Transport: transport}

	var okCount, failCount, allowCount, denyCount, sentCount uint64
	latencies := make([]int64, 0, *count)
	var latMu sync.Mutex

	jobs := make(chan int, *workers*4)
	var wg sync.WaitGroup

	startAll := time.Now()
	for w := 0; w < *workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range jobs {
				var user string
				if *uniqueUsers {
					user = loadUserUUID(i)
				} else {
					user = demoUsers[i%len(demoUsers)]
				}
				body, _ := json.Marshal(swipeRequest{
					UserID:    user,
					DoorID:    *door,
					Direction: *direction,
					Timestamp: time.Now().UTC().Format(time.RFC3339),
				})
				t0 := time.Now()
				req, err := http.NewRequest("POST", url, bytes.NewReader(body))
				var resp *http.Response
				if err == nil {
					req.Header.Set("Content-Type", "application/json")
					apiKey := os.Getenv("API_KEY")
					if apiKey == "" {
						apiKey = "dev-api-key-2026"
					}
					req.Header.Set("X-API-Key", apiKey)
					resp, err = client.Do(req)
				}
				ms := time.Since(t0).Milliseconds()
				latMu.Lock()
				latencies = append(latencies, ms)
				latMu.Unlock()

				currSent := atomic.AddUint64(&sentCount, 1)
				if currSent % 5000 == 0 {
					fmt.Printf("[%s] Progress: Sent %d / %d requests (%.1f%%)...\n",
						time.Now().Format("15:04:05"), currSent, *count, float64(currSent)/float64(*count)*100)
				}

				if err != nil {
					if atomic.AddUint64(&failCount, 1) <= 20 {
						fmt.Printf("Request error: %v\n", err)
					}
					continue
				}
				if resp == nil {
					if atomic.AddUint64(&failCount, 1) <= 20 {
						fmt.Printf("Response is nil\n")
					}
					continue
				}
				data, _ := io.ReadAll(resp.Body)
				resp.Body.Close()
				if resp.StatusCode >= 200 && resp.StatusCode < 300 {
					atomic.AddUint64(&okCount, 1)
					var sr swipeResponse
					if json.Unmarshal(data, &sr) == nil {
						if sr.Decision == "ALLOW" {
							atomic.AddUint64(&allowCount, 1)
						} else {
							atomic.AddUint64(&denyCount, 1)
						}
					}
				} else {
					if atomic.AddUint64(&failCount, 1) <= 20 {
						fmt.Printf("HTTP status error: %d, body: %s\n", resp.StatusCode, string(data))
					}
				}
			}
		}()
	}

	go func() {
		if *ramp <= 0 {
			for i := 0; i < *count; i++ {
				jobs <- i
			}
			close(jobs)
			return
		}
		interval := *ramp / time.Duration(*count)
		for i := 0; i < *count; i++ {
			jobs <- i
			if interval > 0 {
				time.Sleep(interval)
			}
		}
		close(jobs)
	}()

	wg.Wait()
	elapsed := time.Since(startAll)

	sort.Slice(latencies, func(i, j int) bool { return latencies[i] < latencies[j] })
	p := func(q float64) int64 {
		if len(latencies) == 0 {
			return 0
		}
		idx := int(float64(len(latencies)-1) * q)
		if idx < 0 {
			idx = 0
		}
		return latencies[idx]
	}

	uniqueLabel := "yes (synthetic UUID per index)"
	if !*uniqueUsers {
		uniqueLabel = fmt.Sprintf("no (%d demo users rotated)", len(demoUsers))
	}

	fmt.Printf("shift-change load test complete\n")
	fmt.Printf("  requests=%d unique_users=%s direction=%s workers=%d ramp=%s\n",
		*count, uniqueLabel, *direction, *workers, *ramp)
	fmt.Printf("  elapsed=%s http_ok=%d http_fail=%d allow=%d deny=%d qps=%.1f\n",
		elapsed.Round(time.Millisecond), okCount, failCount, allowCount, denyCount, float64(*count)/elapsed.Seconds())
	fmt.Printf("  latency_ms p50=%d p95=%d p99=%d max=%d\n", p(0.50), p(0.95), p(0.99), latencies[len(latencies)-1])
	fmt.Printf("  grafana: http://localhost:3001 → Shift Change Monitor\n")
	if p(0.99) > 50 {
		fmt.Printf("  hint: p99>50ms — try workers=100, ramp=30s, or docker compose up -d --build access-api\n")
	}

	if failCount > uint64(*count)/10 {
		os.Exit(1)
	}
}
