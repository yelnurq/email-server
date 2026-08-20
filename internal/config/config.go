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

	// Mail core (Stalwart) integration. Provider "none" disables the mail
	// core: provisioning is recorded as skipped and health reports it as not
	// configured.
	MailCoreProvider  string // none | stalwart
	StalwartBaseURL   string
	StalwartAdminUser string
	StalwartAdminPass string

	// Connection parameters surfaced to users on Settings → Mail clients.
	// These describe how mail clients reach the mail core from the outside.
	MailClientHost     string
	MailClientSMTPPort string
	MailClientIMAPPort string
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

		MailCoreProvider:  getEnv("MAIL_CORE_PROVIDER", "none"),
		StalwartBaseURL:   os.Getenv("STALWART_BASE_URL"),
		StalwartAdminUser: getEnv("STALWART_ADMIN_USER", "admin"),
		StalwartAdminPass: os.Getenv("STALWART_ADMIN_PASSWORD"),

		MailClientHost:     getEnv("MAIL_CLIENT_HOST", "localhost"),
		MailClientSMTPPort: getEnv("MAIL_CLIENT_SMTP_PORT", "1587"),
		MailClientIMAPPort: getEnv("MAIL_CLIENT_IMAP_PORT", "1993"),
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
	switch cfg.MailCoreProvider {
	case "none":
	case "stalwart":
		if cfg.StalwartBaseURL == "" || cfg.StalwartAdminPass == "" {
			return nil, fmt.Errorf("MAIL_CORE_PROVIDER=stalwart requires STALWART_BASE_URL and STALWART_ADMIN_PASSWORD")
		}
	default:
		return nil, fmt.Errorf("MAIL_CORE_PROVIDER must be none or stalwart, got %q", cfg.MailCoreProvider)
	}
	return cfg, nil
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
