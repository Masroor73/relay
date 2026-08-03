// Package stripewebhook verifies that inbound webhook payloads genuinely
// originated from Stripe, using Stripe's official signature scheme.
package stripewebhook

import (
	"github.com/stripe/stripe-go/v81"
	"github.com/stripe/stripe-go/v81/webhook"
)

// VerifySignature validates the Stripe-Signature header against the given
// payload and webhook signing secret. It returns the parsed Stripe event on
// success. On failure - a missing, malformed, tampered, or expired
// signature - it returns an error and the caller must reject the request
// with 401 before any persistence occurs.
func VerifySignature(payload []byte, signatureHeader, secret string) (stripe.Event, error) {
	return webhook.ConstructEvent(payload, signatureHeader, secret)
}
