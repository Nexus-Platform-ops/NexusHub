package config

import (
	"strings"
	"testing"
)

func setEnvs(t *testing.T, kv map[string]string) {
	t.Helper()
	for k, v := range kv {
		t.Setenv(k, v)
	}
}

func TestFromEnvComplete(t *testing.T) {
	setEnvs(t, map[string]string{
		"DATABASE_URL":              "postgres://u:p@h/db",
		"DATABASE_DATING_URL":       "postgres://u:p@h/db2",
		"JWT_SECRET":                "s",
		"PAYMENT_SIGNING_KEY":       "k",
		"OTP_DEBUG":                 "1",
		"WHISPER_TTL_SECONDS":       "5",
		"RATE_LIMIT_PER_MIN":        "60",
		"ADDR":                      ":9090",
		"REDIS_ADDR":                "127.0.0.1:6380",
		"SENTRY_RELEASE":            "abc123",
		"SENTRY_TRACES_SAMPLE_RATE": "0.25",
	})
	cfg, err := FromEnv()
	if err != nil {
		t.Fatalf("FromEnv: %v", err)
	}
	if cfg.Addr != ":9090" || cfg.RedisAddr != "127.0.0.1:6380" {
		t.Fatalf("addr/redis: %+v", cfg)
	}
	if !cfg.OTPDebug || cfg.WhisperTTLSeconds != 5 || cfg.RateLimitPerMin != 60 {
		t.Fatalf("flags: %+v", cfg)
	}
	if cfg.SentryRelease != "abc123" || cfg.SentryTracesSampleRate != 0.25 {
		t.Fatalf("sentry: %+v", cfg)
	}
	if cfg.SentryEnvironment != EnvDevelopment {
		t.Fatalf("sentry environment = %q, want APP_ENV default", cfg.SentryEnvironment)
	}
}

func TestFromEnvRejectsUnknownProviders(t *testing.T) {
	setEnvs(t, map[string]string{
		"DATABASE_URL":        "postgres://u:p@h/db",
		"DATABASE_DATING_URL": "postgres://u:p@h/db2",
		"JWT_SECRET":          "s",
		"PAYMENT_SIGNING_KEY": "k",
		"SMS_PROVIDER":        "twilio",
		"PAYMENT_PROVIDER":    "kbz",
	})
	_, err := FromEnv()
	if err == nil {
		t.Fatal("expected error for unknown providers")
	}
	for _, want := range []string{"SMS_PROVIDER", "PAYMENT_PROVIDER"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error %q missing %q", err.Error(), want)
		}
	}
}

func TestSentrySampleRateFallsBackWhenInvalid(t *testing.T) {
	setEnvs(t, map[string]string{
		"DATABASE_URL":              "postgres://u:p@h/db",
		"DATABASE_DATING_URL":       "postgres://u:p@h/db2",
		"JWT_SECRET":                "s",
		"PAYMENT_SIGNING_KEY":       "k",
		"SENTRY_TRACES_SAMPLE_RATE": "3.5",
	})
	cfg, err := FromEnv()
	if err != nil {
		t.Fatalf("FromEnv: %v", err)
	}
	if cfg.SentryTracesSampleRate != 0 {
		t.Fatalf("sample rate = %v, want 0", cfg.SentryTracesSampleRate)
	}
}

func TestFromEnvMissingRequired(t *testing.T) {
	t.Setenv("DATABASE_URL", "")
	t.Setenv("DATABASE_DATING_URL", "")
	t.Setenv("JWT_SECRET", "")
	t.Setenv("PAYMENT_SIGNING_KEY", "")
	_, err := FromEnv()
	if err == nil {
		t.Fatal("expected error for missing required values")
	}
	for _, want := range []string{"DATABASE_URL", "DATABASE_DATING_URL", "JWT_SECRET", "PAYMENT_SIGNING_KEY"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error %q missing %q", err.Error(), want)
		}
	}
}

func hardenedConfig() Config {
	return Config{
		Env:                EnvProduction,
		JWTSecret:          strings.Repeat("a", 64),
		PaymentSigningKey:  strings.Repeat("b", 64),
		SocialDatabaseURL:  "postgres://nexus_social:strongpw@db:5432/nexus?sslmode=require",
		DatingDatabaseURL:  "postgres://nexus_dating:otherpw@db:5432/nexus?sslmode=require",
		FacebookAppID:      "123456",
		SocialRedirectBase: "https://api.example.com/api/v1/auth/social",

		SMSProvider: "http",
		SMSAPIURL:   "https://sms.example.com/v1/send",
		SMSAPIKey:   strings.Repeat("c", 32),
		SMSSenderID: "Nexus",

		PaymentProvider:  "http",
		PaymentAPIURL:    "https://pay.example.com/v1/charges",
		PaymentAPIKey:    strings.Repeat("d", 32),
		PaymentReturnURL: "https://app.example.com/wallet",

		CryptoProvider:     "awskms",
		AWSRegion:          "ap-southeast-1",
		AWSAccessKeyID:     strings.Repeat("e", 16),
		AWSSecretAccessKey: strings.Repeat("f", 32),
		AWSKMSKeyID:        "arn:aws:kms:ap-southeast-1:123456789012:key/00000000-0000-0000-0000-000000000000",

		AttestationProvider:      "playintegrity",
		GooglePlayPackageName:    "mm.nexushub.nexus_app",
		GoogleServiceAccountJSON: `{"type":"service_account","project_id":"nexus","private_key":"` + strings.Repeat("g", 32) + `","client_email":"nexus@example.iam.gserviceaccount.com","token_uri":"https://oauth2.googleapis.com/token"}`,

		EmailProvider: "http",
		EmailAPIURL:   "https://mail.example.com/v1/send",
		EmailAPIKey:   strings.Repeat("h", 32),
		EmailFrom:     "Nexus <no-reply@example.com>",
	}
}

func TestValidateAcceptsDevelopmentDefaults(t *testing.T) {
	c := Config{Env: EnvDevelopment, JWTSecret: "s", PaymentSigningKey: "k", OTPDebug: true}
	if err := c.Validate(); err != nil {
		t.Fatalf("development config rejected: %v", err)
	}
}

func TestValidateRejectsUnknownEnv(t *testing.T) {
	c := hardenedConfig()
	c.Env = "prod"
	if err := c.Validate(); err == nil || !strings.Contains(err.Error(), "APP_ENV") {
		t.Fatalf("expected APP_ENV error, got %v", err)
	}
}

func TestValidateProductionRejectsUnsafeValues(t *testing.T) {
	cases := []struct {
		name  string
		mut   func(*Config)
		wants []string
	}{
		{"short jwt", func(c *Config) { c.JWTSecret = "short" }, []string{"JWT_SECRET"}},
		{"short payment key", func(c *Config) { c.PaymentSigningKey = "short" }, []string{"PAYMENT_SIGNING_KEY"}},
		{"compose dev jwt", func(c *Config) { c.JWTSecret = "dev-only-jwt-secret-change-in-production" }, []string{"placeholder"}},
		{"placeholder payment", func(c *Config) { c.PaymentSigningKey = "CHANGE_ME_openssl_rand_hex_32XX" }, []string{"placeholder"}},
		{"reused keys", func(c *Config) { c.PaymentSigningKey = c.JWTSecret }, []string{"different keys"}},
		{"otp debug", func(c *Config) { c.OTPDebug = true }, []string{"OTP_DEBUG"}},
		{"dev social password", func(c *Config) {
			c.SocialDatabaseURL = "postgres://nexus_social:dev_social@db/nexus"
		}, []string{"DATABASE_URL"}},
		{"dev dating password", func(c *Config) {
			c.DatingDatabaseURL = "postgres://nexus_dating:dev_dating@db/nexus"
		}, []string{"DATABASE_DATING_URL"}},
		{"http social redirect", func(c *Config) { c.SocialRedirectBase = "http://api.example.com/oauth" }, []string{"SOCIAL_REDIRECT_BASE"}},
		{"log sms provider", func(c *Config) { c.SMSProvider = "log" }, []string{"SMS_PROVIDER"}},
		{"http sms url", func(c *Config) { c.SMSAPIURL = "http://sms.example.com/send" }, []string{"SMS_API_URL"}},
		{"missing sms key", func(c *Config) { c.SMSAPIKey = "" }, []string{"SMS_API_KEY"}},
		{"missing sms sender", func(c *Config) { c.SMSSenderID = "" }, []string{"SMS_SENDER_ID"}},
		{"log payment provider", func(c *Config) { c.PaymentProvider = "log" }, []string{"PAYMENT_PROVIDER"}},
		{"http payment url", func(c *Config) { c.PaymentAPIURL = "http://pay.example.com/charges" }, []string{"PAYMENT_API_URL"}},
		{"missing payment key", func(c *Config) { c.PaymentAPIKey = "" }, []string{"PAYMENT_API_KEY"}},
		{"http return url", func(c *Config) { c.PaymentReturnURL = "http://app.example.com/wallet" }, []string{"PAYMENT_RETURN_URL"}},
		{"local crypto provider", func(c *Config) { c.CryptoProvider = "local" }, []string{"CRYPTO_PROVIDER"}},
		{"missing kms key", func(c *Config) { c.AWSKMSKeyID = "" }, []string{"KMS_KEY_ID"}},
		{"gcp kms without key resource", func(c *Config) {
			c.CryptoProvider = "gcpkms"
			c.GCPKMSKeyResource = ""
		}, []string{"GCP_KMS_KEY_RESOURCE"}},
		{"log attestation provider", func(c *Config) { c.AttestationProvider = "log" }, []string{"ATTESTATION_PROVIDER"}},
		{"play integrity without package", func(c *Config) { c.GooglePlayPackageName = "" }, []string{"GOOGLE_PLAY_PACKAGE_NAME"}},
		{"play integrity without sa", func(c *Config) { c.GoogleServiceAccountJSON = "" }, []string{"GOOGLE_SERVICE_ACCOUNT_JSON"}},
		{"app attest not implemented", func(c *Config) { c.AttestationProvider = "appattest" }, []string{"ATTESTATION_PROVIDER"}},
		{"devicecheck without team id", func(c *Config) {
			c.AttestationProvider = "devicecheck"
			c.AppleTeamID = ""
		}, []string{"APPLE_TEAM_ID"}},
		{"devicecheck without p8", func(c *Config) {
			c.AttestationProvider = "devicecheck"
			c.AppleP8 = ""
		}, []string{"APPLE_P8"}},
		{"log email provider", func(c *Config) { c.EmailProvider = "log" }, []string{"EMAIL_PROVIDER"}},
		{"http email without url", func(c *Config) { c.EmailAPIURL = "" }, []string{"EMAIL_API_URL"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := hardenedConfig()
			tc.mut(&c)
			err := c.Validate()
			if err == nil {
				t.Fatal("expected validation error")
			}
			for _, want := range tc.wants {
				if !strings.Contains(err.Error(), want) {
					t.Fatalf("error %q missing %q", err.Error(), want)
				}
			}
		})
	}
}

func TestValidateAcceptsHardenedConfig(t *testing.T) {
	c := hardenedConfig()
	if err := c.Validate(); err != nil {
		t.Fatalf("hardened config rejected: %v", err)
	}
	// Staging applies the same rules.
	c.Env = EnvStaging
	if err := c.Validate(); err != nil {
		t.Fatalf("staging config rejected: %v", err)
	}
}

func TestValidateAcceptsDeviceCheck(t *testing.T) {
	c := hardenedConfig()
	c.AttestationProvider = "devicecheck"
	c.AppleTeamID = "TEAM1"
	c.AppleKeyID = "KEY1"
	c.AppleBundleID = "mm.nexushub.nexusApp"
	c.AppleP8 = "-----BEGIN PRIVATE KEY-----\n...\n-----END PRIVATE KEY-----"
	if err := c.Validate(); err != nil {
		t.Fatalf("devicecheck config rejected: %v", err)
	}
}

func TestIsProduction(t *testing.T) {
	if (Config{Env: EnvProduction}).IsProduction() != true {
		t.Fatal("production env must report production")
	}
	for _, env := range []string{EnvDevelopment, EnvStaging, ""} {
		if (Config{Env: env}).IsProduction() {
			t.Fatalf("env %q must not report production", env)
		}
	}
}

func TestFromEnvDefaults(t *testing.T) {
	setEnvs(t, map[string]string{
		"DATABASE_URL":        "x",
		"DATABASE_DATING_URL": "y",
		"JWT_SECRET":          "s",
		"PAYMENT_SIGNING_KEY": "k",
	})
	cfg, err := FromEnv()
	if err != nil {
		t.Fatalf("FromEnv: %v", err)
	}
	if cfg.Addr != ":8080" || cfg.RedisAddr != "127.0.0.1:6379" ||
		cfg.RateLimitPerMin != 120 || cfg.WhisperTTLSeconds != 5 {
		t.Fatalf("defaults wrong: %+v", cfg)
	}
	if cfg.SMSProvider != "log" || cfg.PaymentProvider != "log" {
		t.Fatalf("provider defaults = %q/%q, want log/log", cfg.SMSProvider, cfg.PaymentProvider)
	}
	if cfg.Env != EnvDevelopment {
		t.Fatalf("default env = %q want development", cfg.Env)
	}
	// Invalid numeric env falls back to the default.
	t.Setenv("WHISPER_TTL_SECONDS", "not-a-number")
	cfg2, err := FromEnv()
	if err != nil || cfg2.WhisperTTLSeconds != 5 {
		t.Fatalf("ttl fallback: %+v err %v", cfg2, err)
	}
}
