// Package config resolves the server settings from flags and environment.
package config

import (
	"errors"
	"flag"
	"io"
	"log/slog"
)

// Config holds the server settings every command that opens a database needs.
// Exactly one of DB and WorkspaceDir is set.
type Config struct {
	Addr         string     // listen address
	DB           string     // one-workspace sqlite file path
	WorkspaceDir string     // hosted mode: one sqlite file per workspace
	LogLevel     slog.Level // minimum level written to stderr
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
		Addr:         envOr("TSTAMPS_ADDR", DefaultAddr),
		DB:           envOr("TSTAMPS_DB", DefaultDB),
		WorkspaceDir: getenv("TSTAMPS_WORKSPACE_DIR"),
		LogLevel:     slog.LevelInfo,
	}
	if v := getenv("TSTAMPS_LOG_LEVEL"); v != "" {
		if err := cfg.LogLevel.UnmarshalText([]byte(v)); err != nil {
			return Config{}, err
		}
	}

	fs := flag.NewFlagSet("tstamps-server", flag.ContinueOnError)
	fs.SetOutput(io.Discard) // the caller reports the error, so no flag chatter on stderr
	fs.StringVar(&cfg.Addr, "addr", cfg.Addr, "listen address")
	fs.StringVar(&cfg.DB, "db", cfg.DB, "sqlite file path (one workspace)")
	fs.StringVar(&cfg.WorkspaceDir, "workspace-dir", cfg.WorkspaceDir, "directory of workspace databases (hosted mode)")
	level := fs.String("log-level", cfg.LogLevel.String(), "debug, info, warn or error")
	if err := fs.Parse(args); err != nil {
		return Config{}, err
	}

	dbSet := false
	fs.Visit(func(f *flag.Flag) {
		if f.Name == "db" {
			dbSet = true
		}
	})
	// TSTAMPS_DB defaults to a path, so only an explicitly supplied value counts
	// against hosted mode.
	if cfg.WorkspaceDir != "" && (dbSet || getenv("TSTAMPS_DB") != "") {
		return Config{}, errors.New("set either --db or --workspace-dir, not both")
	}
	if cfg.WorkspaceDir != "" {
		cfg.DB = ""
	}
	if err := cfg.LogLevel.UnmarshalText([]byte(*level)); err != nil {
		return Config{}, err
	}
	return cfg, nil
}
