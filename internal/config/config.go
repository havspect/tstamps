// Package config resolves the server settings from flags and environment.
package config

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

// Config holds the server settings every command that opens a database needs.
type Config struct {
	Addr     string     // listen address
	DB       string     // sqlite database file holding every workspace
	LogLevel slog.Level // minimum level written to stderr

	// LegalDir holds imprint.html and privacy.html served at /imprint and
	// /privacy. Empty means no legal links are shown.
	LegalDir string

	// Self-service signup email. Enabled only when all three of EmailAPIKey,
	// EmailFrom and PublicURL are set.
	EmailAPIKey string
	EmailAPIURL string
	EmailFrom   string
	PublicURL   string
}

const (
	DefaultAddr = ":8080"
	DefaultDB   = "./tstamps.db"
)

// Load resolves flags over env over defaults. getenv is injected so tests need no
// process environment. args excludes the program name.
func Load(args []string, getenv func(string) string) (Config, error) {
	envOr := func(key, def string) string {
		if v := getenv(key); v != "" {
			return v
		}
		return def
	}

	cfg := Config{
		Addr:        envOr("TSTAMPS_ADDR", DefaultAddr),
		DB:          envOr("TSTAMPS_DB", DefaultDB),
		LogLevel:    slog.LevelInfo,
		LegalDir:    getenv("TSTAMPS_LEGAL_DIR"),
		EmailAPIKey: getenv("TSTAMPS_EMAIL_API_KEY"),
		EmailAPIURL: getenv("TSTAMPS_EMAIL_API_URL"),
		EmailFrom:   getenv("TSTAMPS_EMAIL_FROM"),
		PublicURL:   getenv("TSTAMPS_PUBLIC_URL"),
	}
	if v := getenv("TSTAMPS_LOG_LEVEL"); v != "" {
		if err := cfg.LogLevel.UnmarshalText([]byte(v)); err != nil {
			return Config{}, err
		}
	}

	fs := flag.NewFlagSet("tstamps-server", flag.ContinueOnError)
	fs.SetOutput(io.Discard) // the caller reports the error, so no flag chatter on stderr
	fs.StringVar(&cfg.Addr, "addr", cfg.Addr, "listen address")
	fs.StringVar(&cfg.DB, "db", cfg.DB, "sqlite database file")
	fs.StringVar(&cfg.LegalDir, "legal-dir", cfg.LegalDir, "directory holding imprint.html and privacy.html")
	fs.StringVar(&cfg.EmailAPIKey, "email-api-key", cfg.EmailAPIKey, "email API key for self-service signup")
	fs.StringVar(&cfg.EmailAPIURL, "email-api-url", cfg.EmailAPIURL, "email API endpoint (default https://api.resend.com/emails)")
	fs.StringVar(&cfg.EmailFrom, "email-from", cfg.EmailFrom, "From address for signup email")
	fs.StringVar(&cfg.PublicURL, "public-url", cfg.PublicURL, "public origin used in signup links")
	level := fs.String("log-level", cfg.LogLevel.String(), "debug, info, warn or error")
	if err := fs.Parse(args); err != nil {
		return Config{}, err
	}
	if cfg.LegalDir != "" {
		for _, name := range []string{"imprint.html", "privacy.html"} {
			if _, err := os.Stat(filepath.Join(cfg.LegalDir, name)); err != nil {
				return Config{}, fmt.Errorf("--legal-dir: %w", err)
			}
		}
	}
	if err := cfg.LogLevel.UnmarshalText([]byte(*level)); err != nil {
		return Config{}, err
	}
	if err := cfg.validateSignup(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

// SignupEnabled reports whether this server can offer self-service signup: the
// three email settings it needs are present.
func (c Config) SignupEnabled() bool {
	return c.EmailAPIKey != "" && c.EmailFrom != "" && c.PublicURL != ""
}

// validateSignup refuses a half-configured email path, which would otherwise
// only surface when the first person tries to sign up.
func (c *Config) validateSignup() error {
	set := 0
	for _, v := range []string{c.EmailAPIKey, c.EmailFrom, c.PublicURL} {
		if v != "" {
			set++
		}
	}
	if set == 0 {
		return nil
	}
	if set != 3 {
		return errors.New("self-service signup needs --email-api-key, --email-from and --public-url together")
	}
	u, err := url.Parse(c.PublicURL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return errors.New("--public-url must be an absolute http or https URL")
	}
	c.PublicURL = strings.TrimSuffix(c.PublicURL, "/")
	if c.EmailAPIURL != "" {
		e, err := url.Parse(c.EmailAPIURL)
		if err != nil || e.Scheme == "" || e.Host == "" {
			return errors.New("--email-api-url must be an absolute URL")
		}
	}
	return nil
}
