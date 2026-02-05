package stripe

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/BDNK1/sflowg/runtime/plugin"
)

// Config holds Stripe plugin configuration
type Config struct {
	WebhookSecret    string `yaml:"webhook_secret" validate:"required"`
	ToleranceSeconds int    `yaml:"tolerance_seconds" default:"300"` // 5 minutes default
}

// VerifySignatureInput defines input for stripe.verify_signature task
type VerifySignatureInput struct {
	Signature string `json:"signature" validate:"required"` // Stripe-Signature header
	Payload   string `json:"payload" validate:"required"`   // Raw request body
}

// VerifySignatureOutput defines output for stripe.verify_signature task
type VerifySignatureOutput struct {
	Valid     bool   `json:"valid"`
	Timestamp int64  `json:"timestamp"`
	Error     string `json:"error,omitempty"`
}

// StripePlugin provides Stripe webhook signature verification
type StripePlugin struct {
	Config Config
}

// Initialize is called when the plugin is loaded
func (p *StripePlugin) Initialize(exec *plugin.Execution) error {
	if p.Config.WebhookSecret == "" {
		return fmt.Errorf("stripe: webhook_secret is required")
	}
	return nil
}

// Shutdown is called when the plugin is unloaded
func (p *StripePlugin) Shutdown(exec *plugin.Execution) error {
	return nil
}

// VerifySignature verifies a Stripe webhook signature
// Stripe-Signature header format: t=timestamp,v1=signature,v1=signature2...
func (p *StripePlugin) VerifySignature(exec *plugin.Execution, input VerifySignatureInput) (VerifySignatureOutput, error) {
	slog.Info("VerifySignature called",
		"signature_header", input.Signature,
		"payload_length", len(input.Payload),
		"payload_preview", truncate(input.Payload, 100))

	fmt.Println("LOG from stripe plugin")
	// Parse the signature header
	timestamp, signatures, err := parseSignatureHeader(input.Signature)
	if err != nil {
		slog.Error("Failed to parse signature header", "error", err)
		return VerifySignatureOutput{
			Valid: false,
			Error: fmt.Sprintf("invalid signature header: %v", err),
		}, nil
	}

	slog.Info("Parsed signature header",
		"timestamp", timestamp,
		"signatures_count", len(signatures))

	// Check timestamp tolerance
	if p.Config.ToleranceSeconds > 0 {
		age := time.Now().Unix() - timestamp
		if age < 0 {
			age = -age
		}
		slog.Info("Checking timestamp tolerance",
			"age_seconds", age,
			"tolerance_seconds", p.Config.ToleranceSeconds)

		if age > int64(p.Config.ToleranceSeconds) {
			slog.Warn("Timestamp outside tolerance window", "age", age)
			return VerifySignatureOutput{
				Valid:     false,
				Timestamp: timestamp,
				Error:     fmt.Sprintf("timestamp outside tolerance window (%d seconds old)", age),
			}, nil
		}
	}

	// Compute expected signature
	// signed_payload = timestamp + "." + payload
	signedPayload := fmt.Sprintf("%d.%s", timestamp, input.Payload)
	expectedSig := computeSignature(signedPayload, p.Config.WebhookSecret)

	slog.Info("Computed expected signature",
		"signed_payload_preview", truncate(signedPayload, 100),
		"expected_sig", expectedSig,
		"webhook_secret_length", len(p.Config.WebhookSecret))

	// Check if any signature matches
	for i, sig := range signatures {
		slog.Info("Comparing signature",
			"index", i,
			"provided_sig", sig,
			"expected_sig", expectedSig,
			"match", sig == expectedSig)

		if hmac.Equal([]byte(sig), []byte(expectedSig)) {
			slog.Info("Signature verification SUCCESS")
			return VerifySignatureOutput{
				Valid:     true,
				Timestamp: timestamp,
			}, nil
		}
	}

	slog.Warn("Signature verification FAILED - no matching signature found")
	return VerifySignatureOutput{
		Valid:     false,
		Timestamp: timestamp,
		Error:     "signature verification failed",
	}, nil
}

// truncate returns first n characters of s
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

// parseSignatureHeader parses the Stripe-Signature header
// Format: t=timestamp,v1=signature,v1=signature2...
func parseSignatureHeader(header string) (int64, []string, error) {
	var timestamp int64
	var signatures []string

	parts := strings.Split(header, ",")
	for _, part := range parts {
		kv := strings.SplitN(part, "=", 2)
		if len(kv) != 2 {
			continue
		}

		key := strings.TrimSpace(kv[0])
		value := strings.TrimSpace(kv[1])

		switch key {
		case "t":
			ts, err := strconv.ParseInt(value, 10, 64)
			if err != nil {
				return 0, nil, fmt.Errorf("invalid timestamp: %v", err)
			}
			timestamp = ts
		case "v1":
			signatures = append(signatures, value)
		}
	}

	if timestamp == 0 {
		return 0, nil, fmt.Errorf("missing timestamp")
	}

	if len(signatures) == 0 {
		return 0, nil, fmt.Errorf("missing v1 signature")
	}

	return timestamp, signatures, nil
}

// computeSignature computes HMAC-SHA256 signature
func computeSignature(payload, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(payload))
	return hex.EncodeToString(mac.Sum(nil))
}
