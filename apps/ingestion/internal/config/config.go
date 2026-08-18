// Package config loads and validates configuration for Relay's services
// from environment variables.
package config

import (
	"fmt"
	"os"
)

// Config holds runtime settings for the ingestion service.
type Config struct {
	Port                string
	DatabaseURL         string
	StripeWebhookSecret string
	DashboardPassword   string
}

// Load reads configuration for the ingestion service and returns an error
// if any required value is missing.
func Load() (*Config, error) {
	cfg := &Config{
		Port:                getEnvOrDefault("PORT", "8080"),
		DatabaseURL:         os.Getenv("DATABASE_URL"),
		StripeWebhookSecret: os.Getenv("STRIPE_WEBHOOK_SECRET"),
		DashboardPassword:   os.Getenv("DASHBOARD_PASSWORD"),
	}

	if cfg.DatabaseURL == "" {
		return nil, fmt.Errorf("DATABASE_URL is required")
	}
	if cfg.StripeWebhookSecret == "" {
		return nil, fmt.Errorf("STRIPE_WEBHOOK_SECRET is required")
	}
	if cfg.DashboardPassword == "" {
		return nil, fmt.Errorf("DASHBOARD_PASSWORD is required")
	}

	return cfg, nil
}

// ProcessorConfig holds runtime settings for the processor service. Kept
// deliberately separate from Config rather than reusing it with an unused
// field — the processor never touches Stripe signatures and should not
// fail to start over a secret it will never use.
type ProcessorConfig struct {
	DatabaseURL string
}

// LoadProcessor reads configuration for the processor service and returns
// an error if any required value is missing.
func LoadProcessor() (*ProcessorConfig, error) {
	cfg := &ProcessorConfig{
		DatabaseURL: os.Getenv("DATABASE_URL"),
	}

	if cfg.DatabaseURL == "" {
		return nil, fmt.Errorf("DATABASE_URL is required")
	}

	return cfg, nil
}

func getEnvOrDefault(key, fallback string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return fallback
}
