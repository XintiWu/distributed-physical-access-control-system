package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

type swipeRequest struct {
	UserID    string `json:"userId"`
	DoorID    string `json:"doorId"`
	Direction string `json:"direction"`
	CardUID   string `json:"cardUid"`
	Timestamp string `json:"timestamp"`
}

type swipeResponse struct {
	Decision string  `json:"decision"`
	Reason   *string `json:"reason"`
	EventID  string  `json:"eventId"`
}

func main() {
	api := flag.String("api", "http://localhost:8080", "Access API base URL")
	user := flag.String("user", "22222222-2222-2222-2222-222222222222", "Employee UUID")
	door := flag.String("door", "11111111-1111-1111-1111-111111111111", "Door UUID")
	direction := flag.String("direction", "IN", "IN or OUT")
	card := flag.String("card", "CARD001", "Card UID")
	count := flag.Int("count", 1, "Number of swipes")
	interval := flag.Duration("interval", 0, "Interval between swipes")
	flag.Parse()

	client := &http.Client{Timeout: 5 * time.Second}
	url := fmt.Sprintf("%s/access/swipe", *api)

	for i := 0; i < *count; i++ {
		if i > 0 && *interval > 0 {
			time.Sleep(*interval)
		}
		reqBody := swipeRequest{
			UserID:    *user,
			DoorID:    *door,
			Direction: *direction,
			CardUID:   *card,
			Timestamp: time.Now().UTC().Format(time.RFC3339),
		}
		body, _ := json.Marshal(reqBody)

		start := time.Now()
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
		latency := time.Since(start)
		if err != nil {
			fmt.Fprintf(os.Stderr, "request failed: %v\n", err)
			os.Exit(1)
		}
		data, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		var result swipeResponse
		_ = json.Unmarshal(data, &result)

		reason := "null"
		if result.Reason != nil {
			reason = *result.Reason
		}
		fmt.Printf("[%d] status=%d decision=%s reason=%s eventId=%s latency_ms=%d\n",
			i+1, resp.StatusCode, result.Decision, reason, result.EventID, latency.Milliseconds())

		if resp.StatusCode >= 400 {
			fmt.Fprintf(os.Stderr, "body: %s\n", string(data))
			os.Exit(1)
		}
	}
}
