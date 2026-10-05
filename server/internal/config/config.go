// Package config loads server configuration from environment variables.
package config

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	HTTPAddr    string
	DatabaseURL string
	RedisURL    string
	LogLevel    slog.Level

	// AppBaseURL is the public URL of the web app, used in emails and OAuth redirects.
	AppBaseURL string
	// AllowedOrigins are the origins allowed to send state-changing requests.
	AllowedOrigins []string
	// TrustProxy makes the server read the client IP from X-Forwarded-For / X-Real-IP.
	TrustProxy bool

	CookieSecure bool
	SessionTTL   time.Duration
	// CSRFSecret signs CSRF tokens. Must be at least 32 bytes.
	CSRFSecret []byte

	GameConfigDir string

	SignupBonus int64

	GoogleClientID     string
	GoogleClientSecret string
	GoogleRedirectURL  string

	// Mailer selects how emails are sent. Only "log" (development) exists today.
	Mailer string
}

func (c Config) GoogleEnabled() bool {
	return c.GoogleClientID != "" && c.GoogleClientSecret != ""
}

// Load reads the configuration from the environment.
func Load() (Config, error) {
	var errs []error
	c := Config{
		HTTPAddr:           env("HTTP_ADDR", ":8080"),
		DatabaseURL:        os.Getenv("DATABASE_URL"),
		RedisURL:           env("REDIS_URL", "redis://localhost:6379/0"),
		AppBaseURL:         strings.TrimRight(env("APP_BASE_URL", "http://localhost:5173"), "/"),
		GameConfigDir:      env("GAME_CONFIG_DIR", "../game-configs"),
		GoogleClientID:     os.Getenv("GOOGLE_CLIENT_ID"),
		GoogleClientSecret: os.Getenv("GOOGLE_CLIENT_SECRET"),
		GoogleRedirectURL:  os.Getenv("GOOGLE_REDIRECT_URL"),
		Mailer:             env("MAILER", "log"),
		CSRFSecret:         []byte(os.Getenv("CSRF_SECRET")),
	}
	if c.DatabaseURL == "" {
		errs = append(errs, errors.New("DATABASE_URL is required"))
	}
	if len(c.CSRFSecret) < 32 {
		errs = append(errs, errors.New("CSRF_SECRET must be at least 32 bytes"))
	}
	if c.GoogleRedirectURL == "" {
		c.GoogleRedirectURL = c.AppBaseURL + "/api/v1/auth/google/callback"
	}

	if err := c.LogLevel.UnmarshalText([]byte(env("LOG_LEVEL", "info"))); err != nil {
		errs = append(errs, fmt.Errorf("LOG_LEVEL: %w", err))
	}

	origins := env("ALLOWED_ORIGINS", c.AppBaseURL)
	for _, o := range strings.Split(origins, ",") {
		if o = strings.TrimSpace(o); o != "" {
			c.AllowedOrigins = append(c.AllowedOrigins, strings.TrimRight(o, "/"))
		}
	}

	var err error
	if c.CookieSecure, err = strconv.ParseBool(env("COOKIE_SECURE", "true")); err != nil {
		errs = append(errs, fmt.Errorf("COOKIE_SECURE: %w", err))
	}
	if c.TrustProxy, err = strconv.ParseBool(env("TRUST_PROXY", "false")); err != nil {
		errs = append(errs, fmt.Errorf("TRUST_PROXY: %w", err))
	}
	if c.SessionTTL, err = time.ParseDuration(env("SESSION_TTL", "720h")); err != nil {
		errs = append(errs, fmt.Errorf("SESSION_TTL: %w", err))
	}
	if c.SignupBonus, err = strconv.ParseInt(env("SIGNUP_BONUS", "10000"), 10, 64); err != nil || c.SignupBonus < 0 {
		errs = append(errs, fmt.Errorf("SIGNUP_BONUS must be a non-negative integer"))
	}
	if c.Mailer != "log" {
		errs = append(errs, fmt.Errorf("MAILER %q is not supported (use \"log\")", c.Mailer))
	}
	return c, errors.Join(errs...)
}

func env(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return fallback
}
