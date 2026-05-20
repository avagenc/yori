// Package config provides application configuration loaded from environment variables.
// For AWS Lambda deployments, environment variables are set via the Lambda console or SAM/CDK templates.
// For local development, a .env file is loaded via godotenv.
package config

import (
	"fmt"
	"os"
	"sync"

	"github.com/joho/godotenv"
)

// Config holds all required environment variables for the Gmail agent.
type Config struct {
	DatabaseURL    string
	GmailBaseURL   string
	ContactBaseURL string
	GeminiAPIKey   string
	Model          string
}

var (
	cfg     *Config
	loadErr error
	once    sync.Once
)

// Load reads environment variables and returns a validated Config.
// The result is memoised so warm Lambda invocations skip re-parsing.
// On the first failure, subsequent calls return the same error.
func Load() (*Config, error) {
	once.Do(func() {
		// Best-effort .env load for local development; absence is expected in Lambda.
		_ = godotenv.Load()

		c := &Config{
			DatabaseURL:    os.Getenv("DATABASE_URL"),
			GmailBaseURL:   os.Getenv("GMAIL_BASE_URL"),
			ContactBaseURL: os.Getenv("CONTACT_BASE_URL"),
			GeminiAPIKey:   os.Getenv("GEMINI_API_KEY"),
			Model:          os.Getenv("MODEL"),
		}

		if err := c.validate(); err != nil {
			loadErr = err
			return
		}

		cfg = c
	})

	return cfg, loadErr
}

func (c *Config) validate() error {
	required := map[string]string{
		"DATABASE_URL":     c.DatabaseURL,
		"GMAIL_BASE_URL":   c.GmailBaseURL,
		"CONTACT_BASE_URL": c.ContactBaseURL,
		"GEMINI_API_KEY":   c.GeminiAPIKey,
		"MODEL":            c.Model,
	}

	for name, value := range required {
		if value == "" {
			return fmt.Errorf("config: required environment variable %s is not set", name)
		}
	}
	return nil
}
