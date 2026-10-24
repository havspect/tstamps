package main

import (
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/havspect/tstamps/internal/config"
	"github.com/havspect/tstamps/internal/workspace"
)

func admin(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 || args[0] != "create" {
		return adminUsage(stderr)
	}

	// Same default as serve, so TSTAMPS_DB reaches both commands.
	cfg, err := config.Load(nil, os.Getenv)
	if err != nil {
		fmt.Fprintf(stderr, "tstamps admin create: %v\n", err)
		return 1
	}

	fs := flag.NewFlagSet("admin create", flag.ContinueOnError)
	fs.SetOutput(io.Discard) // the caller reports the error in one line
	email := fs.String("email", "", "email")
	name := fs.String("name", "", "name")
	db := fs.String("db", cfg.DB, "sqlite file path")
	if err := fs.Parse(args[1:]); err != nil {
		fmt.Fprintf(stderr, "tstamps admin create: %v\n", err)
		return 2
	}
	if *email == "" || *name == "" || *db == "" {
		return adminUsage(stderr)
	}

	password, err := workspace.AddAdmin(*db, *email, *name)
	if err != nil {
		fmt.Fprintf(stderr, "tstamps admin create: %v\n", err)
		return 1
	}

	fmt.Fprintf(stdout, "created admin %s with initial password: %s\n", *email, password)
	return 0
}

func adminUsage(stderr io.Writer) int {
	fmt.Fprintln(stderr, "usage: tstamps admin create --email x --name y [--db path]")
	return 2
}
