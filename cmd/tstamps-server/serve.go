package main

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"

	"github.com/havspect/tstamps/internal/app"
	"github.com/havspect/tstamps/internal/config"
)

func serve(args []string, stderr io.Writer) int {
	cfg, err := config.Load(args, os.Getenv)
	if err != nil {
		fmt.Fprintf(stderr, "tstamps-server serve: %v\n", err)
		return 2
	}
	log := slog.New(slog.NewTextHandler(stderr, &slog.HandlerOptions{Level: cfg.LogLevel}))
	slog.SetDefault(log)

	if err := app.Run(context.Background(), app.FromConfig(cfg)); err != nil {
		log.Error("serve", "error", err)
		return 1
	}
	return 0
}
