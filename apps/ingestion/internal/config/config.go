// Package config loads and validates application configuration from
// environment variables.
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
}

// Load reads configuration from environment variables and returns an error
// if any required value is missing.
func Load() (*Config, error) {
	cfg := &Config{
		Port:                getEnvOrDefault("PORT", "8080"),
		DatabaseURL:         os.Getenv("DATABASE_URL"),
		StripeWebhookSecret: os.Getenv("STRIPE_WEBHOOK_SECRET"),
	}

	if cfg.DatabaseURL == "" {
		return nil, fmt.Errorf("DATABASE_URL is required")
	}
	if cfg.StripeWebhookSecret == "" {
		return nil, fmt.Errorf("STRIPE_WEBHOOK_SECRET is required")
	}

	return cfg, nil
}

func getEnvOrDefault(key, fallback string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return fallback
}
