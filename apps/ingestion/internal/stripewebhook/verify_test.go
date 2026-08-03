package stripewebhook

import (
	"testing"
	"time"

	"github.com/stripe/stripe-go/v81/webhook"
)

const testSecret = "whsec_test_secret"
const testAPIVersion = `"api_version":"2025-02-24.acacia",`

func TestVerifySignature_ValidSignature_Succeeds(t *testing.T) {
	payload := []byte(`{"id":"evt_test",` + testAPIVersion + `"type":"payment_intent.succeeded"}`)

	signed := webhook.GenerateTestSignedPayload(&webhook.UnsignedPayload{
		Payload: payload,
		Secret:  testSecret,
	})

	_, err := VerifySignature(signed.Payload, signed.Header, testSecret)
	if err != nil {
		t.Fatalf("expected valid signature to succeed, got error: %v", err)
	}
}

func TestVerifySignature_WrongSecret_Fails(t *testing.T) {
	payload := []byte(`{"id":"evt_test",` + testAPIVersion + `"type":"payment_intent.succeeded"}`)

	signed := webhook.GenerateTestSignedPayload(&webhook.UnsignedPayload{
		Payload: payload,
		Secret:  testSecret,
	})

	_, err := VerifySignature(signed.Payload, signed.Header, "whsec_wrong_secret")
	if err == nil {
		t.Fatal("expected verification to fail with wrong secret, got nil error")
	}
}

func TestVerifySignature_TamperedPayload_Fails(t *testing.T) {
	original := []byte(`{"id":"evt_test",` + testAPIVersion + `"type":"payment_intent.succeeded"}`)

	signed := webhook.GenerateTestSignedPayload(&webhook.UnsignedPayload{
		Payload: original,
		Secret:  testSecret,
	})

	tampered := []byte(`{"id":"evt_test",` + testAPIVersion + `"type":"payment_intent.refunded"}`)

	_, err := VerifySignature(tampered, signed.Header, testSecret)
	if err == nil {
		t.Fatal("expected verification to fail on tampered payload, got nil error")
	}
}

func TestVerifySignature_ExpiredTimestamp_Fails(t *testing.T) {
	payload := []byte(`{"id":"evt_test",` + testAPIVersion + `"type":"payment_intent.succeeded"}`)

	signed := webhook.GenerateTestSignedPayload(&webhook.UnsignedPayload{
		Payload:   payload,
		Secret:    testSecret,
		Timestamp: time.Now().Add(-1 * time.Hour),
	})

	_, err := VerifySignature(signed.Payload, signed.Header, testSecret)
	if err == nil {
		t.Fatal("expected verification to fail on expired timestamp, got nil error")
	}
}
