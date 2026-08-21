package config

import "testing"

// prodConfig returns a fully valid production config; tests mutate one field
// at a time to prove each guard fires.
func prodConfig() *Config {
	return &Config{
		AppEnv:                 "production",
		CookieSecure:           true,
		StalwartInsecureTLS:    false,
		BootstrapAdminPassword: "a-strong-secret-9021",
		MailHostname:           "mail.real.example",
		PublicAppURL:           "https://mail.real.example",
		MailCoreProvider:       "stalwart",
		StalwartAdminPass:      "prod-admin-secret",
		StalwartMasterPass:     "prod-master-secret",
		DatabaseURL:            "postgres://u:strongpw@db/mail?sslmode=require",
		S3SecretKey:            "prod-s3-secret",
	}
}

func TestValidateProductionAcceptsSecureConfig(t *testing.T) {
	if err := prodConfig().validateProduction(); err != nil {
		t.Fatalf("secure production config rejected: %v", err)
	}
}

func TestValidateProductionRejectsInsecure(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*Config)
	}{
		{"insecure cookie", func(c *Config) { c.CookieSecure = false }},
		{"self-signed tls", func(c *Config) { c.StalwartInsecureTLS = true }},
		{"default admin pw", func(c *Config) { c.BootstrapAdminPassword = "change-me-please" }},
		{"dev mail hostname", func(c *Config) { c.MailHostname = "mail.company.test" }},
		{"localhost app url", func(c *Config) { c.PublicAppURL = "http://localhost:3000" }},
		{"http app url", func(c *Config) { c.PublicAppURL = "http://mail.real.example" }},
		{"default stalwart admin", func(c *Config) { c.StalwartAdminPass = "stalwart_dev_admin" }},
		{"default stalwart master", func(c *Config) { c.StalwartMasterPass = "stalwart_dev_master" }},
		{"dev db password", func(c *Config) { c.DatabaseURL = "postgres://u:mailplatform_dev@db/mail?sslmode=require" }},
		{"db tls disabled", func(c *Config) { c.DatabaseURL = "postgres://u:pw@db/mail?sslmode=disable" }},
		{"default s3 secret", func(c *Config) { c.S3SecretKey = "minioadmin_dev" }},
	}
	for _, tc := range cases {
		c := prodConfig()
		tc.mutate(c)
		if err := c.validateProduction(); err == nil {
			t.Errorf("%s: expected production validation to fail", tc.name)
		}
	}
}
