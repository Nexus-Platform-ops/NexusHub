// Package config loads Nexus API configuration from the environment.
// No defaults are accepted for secrets: a missing JWT secret or database
// URL must fail startup instead of silently degrading security.
package config

import (
	"errors"
	"os"
	"strconv"
	"strings"
)

// Deployment environments. Staging and production apply the strict checks
// in Validate; development keeps the short local secrets working.
const (
	EnvDevelopment = "development"
	EnvStaging     = "staging"
	EnvProduction  = "production"
)

// Config carries every runtime setting the API needs.
type Config struct {
	Env               string // APP_ENV: development (default), staging, production
	Addr              string // HTTP listen address
	SocialDatabaseURL string // social/ledger/market/chat role
	DatingDatabaseURL string // dating-schema role (isolated)
	RedisAddr         string
	JWTSecret         string
	PaymentSigningKey string // HMAC key used to verify coin top-up callbacks
	OTPDebug          bool   // expose OTP codes in responses (development only)
	RateLimitPerMin   int    // requests per minute per principal
	WhisperTTLSeconds int    // Redis TTL for whisper messages

	// Outbound SMS for OTP delivery. "log" is a delivery stub for
	// development; staging and production require "http" with an https
	// endpoint, API key and sender id.
	SMSProvider string
	SMSAPIURL   string
	SMSAPIKey   string
	SMSSenderID string

	// Payment provider for coin top-ups. "log" creates intents without a
	// checkout URL and is development-only; production requires "http".
	PaymentProvider  string
	PaymentAPIURL    string
	PaymentAPIKey    string
	PaymentReturnURL string

	// Social login providers (Facebook / TikTok / Telegram). Optional: the
	// endpoints answer 503 while a provider is unconfigured. The *Base
	// fields override official endpoints and exist for tests only.
	FacebookAppID      string
	FacebookAppSecret  string
	FBGraphBase        string
	FBOAuthBase        string
	TikTokClientKey    string
	TikTokClientSecret string
	TikTokAPIBase      string
	TikTokAuthBase     string
	TelegramBotToken   string
	SocialRedirectBase string

	// Sentry error reporting. Optional: an empty DSN disables reporting.
	// No request bodies, headers, query strings or user identifiers are
	// ever attached (see internal/observability).
	SentryDSN              string
	SentryEnvironment      string
	SentryRelease          string
	SentryTracesSampleRate float64

	// Envelope encryption at rest (Phase D). Every payload is encrypted
	// with a fresh DEK and the DEK is wrapped by the configured master key:
	// AWS KMS, GCP KMS, or (development only) a local key.
	CryptoProvider string
	CryptoLocalKey string // base64 32-byte key for provider "local"
	AWSRegion      string
	AWSAccessKeyID string
	AWSSecretAccessKey string
	AWSKMSKeyID    string
	GCPKMSKeyResource  string
	GCPServiceAccountJSON string

	// Device attestation (Play Integrity / App Attest). Optional in
	// development ("log" stub); staging/production require "playintegrity".
	AttestationProvider   string
	GooglePlayPackageName string
	GoogleServiceAccountJSON string

	// Transactional email for DSR flows. "log" is development-only;
	// staging/production require "http" with an https endpoint.
	EmailProvider string
	EmailAPIURL   string
	EmailAPIKey   string
	EmailFrom     string

	// MetricsAddr is the Prometheus scrape/admin listen address. Empty
	// disables the listener (metrics still exist, they just aren't served).
	// OTel tracing is configured through the standard OTEL_* environment
	// variables (see internal/observability.TracingConfigFromEnv).
	MetricsAddr string
}

func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// firstEnv returns the first non-empty environment value.
func firstEnv(keys ...string) string {
	for _, k := range keys {
		if v := os.Getenv(k); v != "" {
			return v
		}
	}
	return ""
}

func atoiDefault(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	return def
}

func rateDefault(key string, def float64) float64 {
	if v := os.Getenv(key); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil && f >= 0 && f <= 1 {
			return f
		}
	}
	return def
}

// FromEnv builds a Config from environment variables, returning an error
// listing every missing required value so operators can fix them in one pass.
func FromEnv() (Config, error) {
	c := Config{
		Env:               strings.ToLower(strings.TrimSpace(getenv("APP_ENV", EnvDevelopment))),
		Addr:              getenv("ADDR", ":8080"),
		SocialDatabaseURL: os.Getenv("DATABASE_URL"),
		DatingDatabaseURL: os.Getenv("DATABASE_DATING_URL"),
		RedisAddr:         getenv("REDIS_ADDR", "127.0.0.1:6379"),
		JWTSecret:         os.Getenv("JWT_SECRET"),
		PaymentSigningKey: os.Getenv("PAYMENT_SIGNING_KEY"),
		OTPDebug:          os.Getenv("OTP_DEBUG") == "1",
		RateLimitPerMin:   atoiDefault("RATE_LIMIT_PER_MIN", 120),
		WhisperTTLSeconds: atoiDefault("WHISPER_TTL_SECONDS", 5),

		SMSProvider: strings.ToLower(strings.TrimSpace(getenv("SMS_PROVIDER", "log"))),
		SMSAPIURL:   os.Getenv("SMS_API_URL"),
		SMSAPIKey:   os.Getenv("SMS_API_KEY"),
		SMSSenderID: os.Getenv("SMS_SENDER_ID"),

		PaymentProvider:  strings.ToLower(strings.TrimSpace(getenv("PAYMENT_PROVIDER", "log"))),
		PaymentAPIURL:    os.Getenv("PAYMENT_API_URL"),
		PaymentAPIKey:    os.Getenv("PAYMENT_API_KEY"),
		PaymentReturnURL: os.Getenv("PAYMENT_RETURN_URL"),

		FacebookAppID:      os.Getenv("FACEBOOK_APP_ID"),
		FacebookAppSecret:  os.Getenv("FACEBOOK_APP_SECRET"),
		FBGraphBase:        os.Getenv("FB_GRAPH_BASE"),
		FBOAuthBase:        os.Getenv("FB_OAUTH_BASE"),
		TikTokClientKey:    os.Getenv("TIKTOK_CLIENT_KEY"),
		TikTokClientSecret: os.Getenv("TIKTOK_CLIENT_SECRET"),
		TikTokAPIBase:      os.Getenv("TIKTOK_API_BASE"),
		TikTokAuthBase:     os.Getenv("TIKTOK_AUTH_BASE"),
		TelegramBotToken:   os.Getenv("TELEGRAM_BOT_TOKEN"),
		SocialRedirectBase: os.Getenv("SOCIAL_REDIRECT_BASE"),

		SentryDSN:              os.Getenv("SENTRY_DSN"),
		SentryEnvironment:      os.Getenv("SENTRY_ENVIRONMENT"),
		SentryRelease:          os.Getenv("SENTRY_RELEASE"),
		SentryTracesSampleRate: rateDefault("SENTRY_TRACES_SAMPLE_RATE", 0),

		CryptoProvider: strings.ToLower(strings.TrimSpace(getenv("CRYPTO_PROVIDER", "local"))),
		CryptoLocalKey: os.Getenv("CRYPTO_LOCAL_KEY"),
		AWSRegion:      getenv("AWS_REGION", "ap-southeast-1"),
		AWSAccessKeyID: os.Getenv("AWS_ACCESS_KEY_ID"),
		AWSSecretAccessKey: os.Getenv("AWS_SECRET_ACCESS_KEY"),
		AWSKMSKeyID:    os.Getenv("KMS_KEY_ID"),
		GCPKMSKeyResource: os.Getenv("GCP_KMS_KEY_RESOURCE"),
		GCPServiceAccountJSON: firstEnv("GCP_SERVICE_ACCOUNT_JSON", "GOOGLE_SERVICE_ACCOUNT_JSON"),

		AttestationProvider: strings.ToLower(strings.TrimSpace(getenv("ATTESTATION_PROVIDER", "log"))),
		GooglePlayPackageName: os.Getenv("GOOGLE_PLAY_PACKAGE_NAME"),
		GoogleServiceAccountJSON: os.Getenv("GOOGLE_SERVICE_ACCOUNT_JSON"),

		EmailProvider: strings.ToLower(strings.TrimSpace(getenv("EMAIL_PROVIDER", "log"))),
		EmailAPIURL:   os.Getenv("EMAIL_API_URL"),
		EmailAPIKey:   os.Getenv("EMAIL_API_KEY"),
		EmailFrom:     os.Getenv("EMAIL_FROM"),

		MetricsAddr: getenv("METRICS_ADDR", ":9090"),
	}
	if c.SentryEnvironment == "" {
		c.SentryEnvironment = c.Env
	}
	if c.WhisperTTLSeconds <= 0 {
		c.WhisperTTLSeconds = 5
	}
	var errs []error
	if c.SMSProvider != "log" && c.SMSProvider != "http" {
		errs = append(errs, errors.New("SMS_PROVIDER must be log or http"))
	}
	if c.PaymentProvider != "log" && c.PaymentProvider != "http" {
		errs = append(errs, errors.New("PAYMENT_PROVIDER must be log or http"))
	}
	if c.SocialDatabaseURL == "" {
		errs = append(errs, errors.New("DATABASE_URL is required"))
	}
	if c.DatingDatabaseURL == "" {
		errs = append(errs, errors.New("DATABASE_DATING_URL is required"))
	}
	if c.JWTSecret == "" {
		errs = append(errs, errors.New("JWT_SECRET is required"))
	}
	if c.PaymentSigningKey == "" {
		errs = append(errs, errors.New("PAYMENT_SIGNING_KEY is required"))
	}
	if len(errs) > 0 {
		return c, errors.Join(errs...)
	}
	return c, nil
}

// IsProduction reports whether strict production rules apply.
func (c Config) IsProduction() bool { return c.Env == EnvProduction }

var devSecretMarkers = []string{
	"dev-only", "change-in-production", "change_me", "changeme", "dev_secret",
}

func looksLikeDevSecret(s string) bool {
	low := strings.ToLower(s)
	for _, marker := range devSecretMarkers {
		if strings.Contains(low, marker) {
			return true
		}
	}
	return false
}

// Validate enforces deployment safety rules. Missing values are already
// rejected by FromEnv; this catches values that would be safe in local
// development but dangerous in staging or production (default credentials,
// OTP leakage, weak or reused keys).
func (c Config) Validate() error {
	switch c.Env {
	case EnvDevelopment, EnvStaging, EnvProduction:
	default:
		return errors.New("APP_ENV must be development, staging or production")
	}
	if c.Env == EnvDevelopment {
		return nil
	}

	var errs []error
	if len(c.JWTSecret) < 32 {
		errs = append(errs, errors.New("JWT_SECRET must be at least 32 characters outside development"))
	}
	if len(c.PaymentSigningKey) < 32 {
		errs = append(errs, errors.New("PAYMENT_SIGNING_KEY must be at least 32 characters outside development"))
	}
	if looksLikeDevSecret(c.JWTSecret) {
		errs = append(errs, errors.New("JWT_SECRET still contains a development placeholder"))
	}
	if looksLikeDevSecret(c.PaymentSigningKey) {
		errs = append(errs, errors.New("PAYMENT_SIGNING_KEY still contains a development placeholder"))
	}
	if c.JWTSecret == c.PaymentSigningKey {
		errs = append(errs, errors.New("JWT_SECRET and PAYMENT_SIGNING_KEY must be different keys"))
	}
	if c.OTPDebug {
		errs = append(errs, errors.New("OTP_DEBUG must be 0 outside development (it leaks OTP codes in responses)"))
	}
	if strings.Contains(c.SocialDatabaseURL, "dev_social") {
		errs = append(errs, errors.New("DATABASE_URL still uses the development role password"))
	}
	if strings.Contains(c.DatingDatabaseURL, "dev_dating") {
		errs = append(errs, errors.New("DATABASE_DATING_URL still uses the development role password"))
	}
	if (c.FacebookAppID != "" || c.TikTokClientKey != "") &&
		!strings.HasPrefix(c.SocialRedirectBase, "https://") {
		errs = append(errs, errors.New("SOCIAL_REDIRECT_BASE must be an https:// URL when Facebook or TikTok login is enabled"))
	}
	// Without a real SMS provider the API cannot deliver OTPs and every new
	// user is locked out, so production refuses to start on the log stub.
	if c.SMSProvider != "http" {
		errs = append(errs, errors.New("SMS_PROVIDER must be http outside development"))
	} else {
		if !strings.HasPrefix(c.SMSAPIURL, "https://") {
			errs = append(errs, errors.New("SMS_API_URL must be an https:// URL"))
		}
		if c.SMSAPIKey == "" {
			errs = append(errs, errors.New("SMS_API_KEY is required when SMS_PROVIDER=http"))
		}
		if looksLikeDevSecret(c.SMSAPIKey) {
			errs = append(errs, errors.New("SMS_API_KEY still contains a development placeholder"))
		}
		if c.SMSSenderID == "" {
			errs = append(errs, errors.New("SMS_SENDER_ID is required when SMS_PROVIDER=http"))
		}
	}
	// A log payment provider would hand clients intents with no checkout URL,
	// so paid top-ups could never complete.
	if c.PaymentProvider != "http" {
		errs = append(errs, errors.New("PAYMENT_PROVIDER must be http outside development"))
	} else {
		if !strings.HasPrefix(c.PaymentAPIURL, "https://") {
			errs = append(errs, errors.New("PAYMENT_API_URL must be an https:// URL"))
		}
		if c.PaymentAPIKey == "" {
			errs = append(errs, errors.New("PAYMENT_API_KEY is required when PAYMENT_PROVIDER=http"))
		}
		if looksLikeDevSecret(c.PaymentAPIKey) {
			errs = append(errs, errors.New("PAYMENT_API_KEY still contains a development placeholder"))
		}
		if !strings.HasPrefix(c.PaymentReturnURL, "https://") {
			errs = append(errs, errors.New("PAYMENT_RETURN_URL must be an https:// URL"))
		}
	}
	// Phase D: a local master key would put key material in an env var
	// outside development, and unverifiable devices cannot be gated.
	switch c.CryptoProvider {
	case "awskms":
		if c.AWSKMSKeyID == "" || c.AWSRegion == "" || c.AWSAccessKeyID == "" || c.AWSSecretAccessKey == "" {
			errs = append(errs, errors.New("CRYPTO_PROVIDER=awskms requires KMS_KEY_ID, AWS_REGION, AWS_ACCESS_KEY_ID and AWS_SECRET_ACCESS_KEY"))
		}
	case "gcpkms":
		if c.GCPKMSKeyResource == "" || c.GCPServiceAccountJSON == "" {
			errs = append(errs, errors.New("CRYPTO_PROVIDER=gcpkms requires GCP_KMS_KEY_RESOURCE and a service account JSON"))
		}
	default:
		errs = append(errs, errors.New("CRYPTO_PROVIDER must be awskms or gcpkms outside development"))
	}
	switch c.AttestationProvider {
	case "playintegrity":
		if c.GooglePlayPackageName == "" || c.GoogleServiceAccountJSON == "" {
			errs = append(errs, errors.New("ATTESTATION_PROVIDER=playintegrity requires GOOGLE_PLAY_PACKAGE_NAME and GOOGLE_SERVICE_ACCOUNT_JSON"))
		}
	default:
		errs = append(errs, errors.New("ATTESTATION_PROVIDER must be playintegrity outside development"))
	}
	if c.EmailProvider != "http" {
		errs = append(errs, errors.New("EMAIL_PROVIDER must be http outside development"))
	} else {
		if !strings.HasPrefix(c.EmailAPIURL, "https://") {
			errs = append(errs, errors.New("EMAIL_API_URL must be an https:// URL"))
		}
		if c.EmailAPIKey == "" || c.EmailFrom == "" {
			errs = append(errs, errors.New("EMAIL_API_KEY and EMAIL_FROM are required when EMAIL_PROVIDER=http"))
		}
	}
	return errors.Join(errs...)
}
