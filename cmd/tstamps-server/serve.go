package main

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/havspect/tstamps/internal/api"
	"github.com/havspect/tstamps/internal/clock"
	"github.com/havspect/tstamps/internal/config"
	"github.com/havspect/tstamps/internal/workspace"
)

func serve(args []string, stderr io.Writer) int {
	cfg, err := config.Load(args, os.Getenv)
	if err != nil {
		fmt.Fprintf(stderr, "tstamps-server serve: %v\n", err)
		return 2
	}
	log := slog.New(slog.NewTextHandler(stderr, &slog.HandlerOptions{Level: cfg.LogLevel}))
	slog.SetDefault(log)

	var signup http.Handler
	if cfg.SignupEnabled() {
		endpoint := cfg.EmailAPIURL
		if endpoint == "" {
			endpoint = workspace.DefaultEmailEndpoint
		}
		svc := workspace.NewSignup(cfg.DB, cfg.PublicURL, &workspace.Resend{
			Endpoint: endpoint, Key: cfg.EmailAPIKey, From: cfg.EmailFrom,
		}, clock.System{})
		signup = api.NewSignupHandler(svc, clock.System{})
		log.Info("self-service signup enabled", "public_url", cfg.PublicURL)
	}

	host, err := workspace.New(cfg.DB, clock.System{}, signup)
	if err != nil {
		log.Error("open database", "db", cfg.DB, "error", err)
		return 1
	}
	defer host.Close()

	srv := &http.Server{
		Addr:              cfg.Addr,
		Handler:           host,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	ln, err := net.Listen("tcp", cfg.Addr)
	if err != nil {
		log.Error("listen", "addr", cfg.Addr, "error", err)
		return 1
	}

	log.Info("serving", "addr", ln.Addr().String(), "db", cfg.DB)

	listenErr := make(chan error, 1)
	go func() { listenErr <- srv.Serve(ln) }()

	select {
	case err := <-listenErr:
		log.Error("listen", "addr", cfg.Addr, "error", err)
		return 1
	case <-ctx.Done():
	}

	stop() // the second signal kills the process instead of waiting again

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Error("shutdown", "error", err)
		return 1
	}
	log.Info("stopped")
	return 0
}
