// Package config loads application configuration from environment variables.
// A local .env file is loaded if present (development convenience); real
// environments must provide variables directly. No secrets are hardcoded.
package config

import (
	"fmt"
	"os"
	"strings"

	"github.com/joho/godotenv"
)

// Config holds all runtime configuration for the API and worker processes.
type Config struct {
	APIAddr string

	DatabaseURL string
	RedisURL    string
	NATSURL     string

	S3Endpoint          string
	S3AccessKey         string
	S3SecretKey         string
	S3Region            string
	S3BucketAttachments string

	CORSAllowedOrigins []string

	LogLevel  string
	LogFormat string

	CookieSecure bool

	BootstrapAdminEmail    string
	BootstrapAdminPassword string
}

// Load reads configuration from the environment, optionally seeded by a .env
// file in the working directory. Required variables produce an error when
// missing so misconfiguration fails fast instead of half-working.
func Load() (*Config, error) {
	// Ignore the error: .env is optional and absent outside local development.
	_ = godotenv.Load()

	cfg := &Config{
		APIAddr:             getEnv("API_ADDR", ":8080"),
		DatabaseURL:         os.Getenv("DATABASE_URL"),
		RedisURL:            os.Getenv("REDIS_URL"),
		NATSURL:             os.Getenv("NATS_URL"),
		S3Endpoint:          os.Getenv("S3_ENDPOINT"),
		S3AccessKey:         os.Getenv("S3_ACCESS_KEY"),
		S3SecretKey:         os.Getenv("S3_SECRET_KEY"),
		S3Region:            getEnv("S3_REGION", "us-east-1"),
		S3BucketAttachments: getEnv("S3_BUCKET_ATTACHMENTS", "mail-attachments"),
		LogLevel:            getEnv("LOG_LEVEL", "info"),
		LogFormat:           getEnv("LOG_FORMAT", "json"),

		CookieSecure: getEnv("COOKIE_SECURE", "false") == "true",

		BootstrapAdminEmail:    os.Getenv("BOOTSTRAP_ADMIN_EMAIL"),
		BootstrapAdminPassword: os.Getenv("BOOTSTRAP_ADMIN_PASSWORD"),
	}

	if origins := os.Getenv("CORS_ALLOWED_ORIGINS"); origins != "" {
		for _, o := range strings.Split(origins, ",") {
			if o = strings.TrimSpace(o); o != "" {
				cfg.CORSAllowedOrigins = append(cfg.CORSAllowedOrigins, o)
			}
		}
	}

	var missing []string
	for name, val := range map[string]string{
		"DATABASE_URL": cfg.DatabaseURL,
		"REDIS_URL":    cfg.RedisURL,
		"NATS_URL":     cfg.NATSURL,
		"S3_ENDPOINT":  cfg.S3Endpoint,
	} {
		if val == "" {
			missing = append(missing, name)
		}
	}
	if len(missing) > 0 {
		return nil, fmt.Errorf("missing required environment variables: %s", strings.Join(missing, ", "))
	}
	return cfg, nil
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
