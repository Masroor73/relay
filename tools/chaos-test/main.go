// Command chaos-test exercises Relay's core reliability guarantees against
// a running instance, firing real signed webhook requests rather than
// inserting rows directly into Postgres — a genuine end-to-end
// demonstration, not a database-layer shortcut.
package main

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"time"
)

func main() {
	targetURL := flag.String("url", "http://localhost:8080", "base URL of the running cmd/server instance")
	secret := flag.String("secret", os.Getenv("STRIPE_WEBHOOK_SECRET"), "Stripe webhook signing secret (defaults to $STRIPE_WEBHOOK_SECRET)")
	mode := flag.String("mode", "", "which scenario to run: dedup or failure")
	flag.Parse()

	if *secret == "" {
		log.Fatal("signing secret is required: pass -secret or set STRIPE_WEBHOOK_SECRET")
	}

	switch *mode {
	case "dedup":
		runDedupScenario(*targetURL, *secret)
	case "failure":
		runFailureScenario(*targetURL, *secret)
	default:
		log.Fatal("must specify -mode dedup or -mode failure")
	}
}

// runDedupScenario fires the identical event twice and demonstrates that
// only one order results — the idempotency guarantee proven in ENG-21,
// now demonstrable against any running instance without hand-typed SQL.
func runDedupScenario(baseURL, secret string) {
	eventID := fmt.Sprintf("evt_chaos_dedup_%d", time.Now().Unix())
	payload := []byte(fmt.Sprintf(
		`{"id":"%s","api_version":"2025-02-24.acacia","type":"payment_intent.succeeded","amount":1999}`,
		eventID,
	))

	fmt.Println("Firing the identical event twice...")
	status1 := sendWebhook(baseURL, secret, payload)
	fmt.Printf("  First delivery:  %d\n", status1)
	status2 := sendWebhook(baseURL, secret, payload)
	fmt.Printf("  Second delivery: %d\n", status2)

	fmt.Println("\nBoth should be 200 — the second is a deduplicated acknowledgment,")
	fmt.Println("not a reprocessing. Check the dashboard or /api/orders: exactly one")
	fmt.Printf("order should exist for event %s.\n", eventID)
}

// runFailureScenario fires an event with a payload the processor's
// handler can't parse, demonstrating retry/backoff and eventual
// dead-letter promotion — the same manual test repeated by hand
// throughout Milestones 3-5, now a single command.
func runFailureScenario(baseURL, secret string) {
	eventID := fmt.Sprintf("evt_chaos_failure_%d", time.Now().Unix())
	payload := []byte(fmt.Sprintf(
		`{"id":"%s","api_version":"2025-02-24.acacia","type":"payment_intent.succeeded","amount":"not-a-number"}`,
		eventID,
	))

	fmt.Println("Firing an event with a payload the processor cannot parse...")
	status := sendWebhook(baseURL, secret, payload)
	fmt.Printf("  Delivery accepted: %d\n", status)

	fmt.Println("\nThe processor will retry this on the real backoff schedule")
	fmt.Println("(1s, 5s, 30s, 2m, 10m) and dead-letter it after 6 attempts —")
	fmt.Printf("watch the processor's logs or the dashboard's DLQ view for event %s.\n", eventID)
	fmt.Println("Full cycle takes roughly 13 minutes.")
}

func sendWebhook(baseURL, secret string, payload []byte) int {
	timestamp := time.Now().Unix()
	signature := computeSignature(timestamp, payload, secret)

	req, err := http.NewRequest(http.MethodPost, baseURL+"/webhooks/stripe", bytes.NewReader(payload))
	if err != nil {
		log.Fatalf("failed to build request: %v", err)
	}
	req.Header.Set("Stripe-Signature", fmt.Sprintf("t=%d,v1=%s", timestamp, signature))

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)

	return resp.StatusCode
}

// computeSignature replicates Stripe's real signature scheme
// (HMAC-SHA256 over "{timestamp}.{payload}"), the same construction
// ENG-8's verification logic expects — not a simplified stand-in.
func computeSignature(timestamp int64, payload []byte, secret string) string {
	signedPayload := fmt.Sprintf("%d.%s", timestamp, payload)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(signedPayload))
	return hex.EncodeToString(mac.Sum(nil))
}
