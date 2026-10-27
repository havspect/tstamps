package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/havspect/tstamps/internal/config"
	"github.com/havspect/tstamps/internal/onboarding"
	gormrepo "github.com/havspect/tstamps/internal/repository/gorm"
)

func workspaceCmd(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 || args[0] != "create" || len(args) < 2 {
		return workspaceUsage(stderr)
	}
	cfg, err := config.Load(nil, os.Getenv)
	if err != nil {
		fmt.Fprintf(stderr, "tstamps-server workspace create: %v\n", err)
		return 1
	}

	slug := args[1]
	fs := flag.NewFlagSet("workspace create", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	db := fs.String("db", cfg.DB, "sqlite database file")
	email := fs.String("email", "", "first admin email")
	name := fs.String("name", "", "first admin name")
	if err := fs.Parse(args[2:]); err != nil {
		return workspaceUsage(stderr)
	}
	if fs.NArg() != 0 || *db == "" || *email == "" || *name == "" {
		return workspaceUsage(stderr)
	}

	gdb, err := gormrepo.Open(*db)
	if err != nil {
		fmt.Fprintf(stderr, "tstamps-server workspace create: %v\n", err)
		return 1
	}
	sqlDB, _ := gdb.DB()
	if sqlDB != nil {
		defer sqlDB.Close()
	}

	if err := gormrepo.Migrate(gdb); err != nil {
		fmt.Fprintf(stderr, "tstamps-server workspace create: %v\n", err)
		return 1
	}

	repo := gormrepo.NewOnboardingRepository(gdb)
	svc := onboarding.NewService(repo)
	_, password, err := svc.CreateWorkspace(context.Background(), slug, *email, *name)
	if err != nil {
		fmt.Fprintf(stderr, "tstamps-server workspace create: %v\n", err)
		return 1
	}
	fmt.Fprintf(stdout, "created workspace %s with admin %s and initial password: %s\n", slug, *email, password)
	return 0
}

func workspaceUsage(stderr io.Writer) int {
	fmt.Fprintln(stderr, "usage: tstamps-server workspace create <slug> --email x --name y [--db path]")
	return 2
}
