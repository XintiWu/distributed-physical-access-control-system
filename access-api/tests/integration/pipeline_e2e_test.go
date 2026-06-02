//go:build integration

package integration_test

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"os"
	"testing"
	"time"

	_ "github.com/ClickHouse/clickhouse-go/v2"
)

func TestPipelineE2E(t *testing.T) {
	if os.Getenv("E2E_PIPELINE") != "1" {
		t.Skip("set E2E_PIPELINE=1 and run 'make up' first")
	}

	apiURL := envOr("API_URL", "http://localhost:8080")
	chDSN := envOr("CH_DSN", "clickhouse://default:@127.0.0.1:9000/access_control")
	userID := envOr("DEMO_USER", "22222222-2222-2222-2222-222222222222")
	doorID := envOr("DEMO_DOOR", "11111111-1111-1111-1111-111111111111")

	apiKey := os.Getenv("API_KEY")
	if apiKey == "" {
		t.Fatal("API_KEY must be set for E2E pipeline tests")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	checkHealth(t, ctx, apiURL)

	db := connectClickHouse(t, ctx, chDSN)
	defer db.Close()

	eventID, decision := sendSwipe(t, ctx, apiURL, userID, doorID, apiKey)

	pollClickHouseAndVerify(t, ctx, db, eventID, userID, decision)
}

func checkHealth(t *testing.T, ctx context.Context, apiURL string) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL+"/health", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("access-api not reachable at %s: %v (run make up)", apiURL, err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("health check status %d", resp.StatusCode)
	}
}

func connectClickHouse(t *testing.T, ctx context.Context, chDSN string) *sql.DB {
	db, err := sql.Open("clickhouse", chDSN)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		t.Fatalf("clickhouse not reachable: %v", err)
	}
	return db
}

func sendSwipe(t *testing.T, ctx context.Context, apiURL, userID, doorID, apiKey string) (string, string) {
	body, _ := json.Marshal(map[string]interface{}{
		"userId":    userID,
		"doorId":    doorID,
		"direction": "IN",
		"cardUid":   "CARD001",
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	})
	swipeCtx, swipeCancel := context.WithTimeout(ctx, 10*time.Second)
	defer swipeCancel()
	swipeReq, err := http.NewRequestWithContext(swipeCtx, http.MethodPost, apiURL+"/access/swipe", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	swipeReq.Header.Set("Content-Type", "application/json")
	if apiKey != "" {
		swipeReq.Header.Set("X-API-Key", apiKey)
	}
	swipeResp, err := http.DefaultClient.Do(swipeReq)
	if err != nil {
		t.Fatal(err)
	}
	defer swipeResp.Body.Close()
	if swipeResp.StatusCode != http.StatusOK {
		t.Fatalf("swipe status %d", swipeResp.StatusCode)
	}

	var swipe struct {
		Decision string `json:"decision"`
		EventID  string `json:"eventId"`
	}
	if err := json.NewDecoder(swipeResp.Body).Decode(&swipe); err != nil {
		t.Fatal(err)
	}
	if swipe.EventID == "" {
		t.Fatal("swipe response missing eventId")
	}
	return swipe.EventID, swipe.Decision
}

func pollClickHouseAndVerify(t *testing.T, ctx context.Context, db *sql.DB, eventID, userID, decision string) {
	deadline := time.Now().Add(30 * time.Second)
	var (
		employeeID string
		direction  string
		status     string
		err        error
	)
	for time.Now().Before(deadline) {
		row := db.QueryRowContext(ctx, `
			SELECT toString(employee_id), toString(direction), toString(status)
			FROM inout_events WHERE id = ?`, eventID)
		err = row.Scan(&employeeID, &direction, &status)
		if err == nil {
			break
		}
		if err != sql.ErrNoRows {
			t.Fatal(err)
		}
		time.Sleep(2 * time.Second)
	}

	if employeeID == "" {
		t.Fatalf("event %s not found in ClickHouse inout_events within 30s", eventID)
	}
	if employeeID != userID {
		t.Fatalf("employee_id: got %s want %s", employeeID, userID)
	}
	if direction != "IN" {
		t.Fatalf("direction: got %s want IN", direction)
	}
	if status != decision {
		t.Fatalf("status: got %s want %s", status, decision)
	}
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
