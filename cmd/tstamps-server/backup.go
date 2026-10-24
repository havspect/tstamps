package main

import (
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/havspect/tstamps/internal/config"
	"github.com/havspect/tstamps/internal/store"
	"github.com/havspect/tstamps/internal/workspace"
)

// backup writes a consistent snapshot of the live database, or one snapshot per
// workspace. The destination must not exist, so a nightly name never overwrites
// yesterday's.
func backup(args []string, stdout, stderr io.Writer) int {
	cfg, err := config.Load(nil, os.Getenv)
	if err != nil {
		fmt.Fprintf(stderr, "tstamps-server backup: %v\n", err)
		return 1
	}

	fs := flag.NewFlagSet("backup", flag.ContinueOnError)
	fs.SetOutput(io.Discard) // the caller reports the error in one line
	db := fs.String("db", cfg.DB, "sqlite file path (one workspace)")
	dir := fs.String("workspace-dir", cfg.WorkspaceDir, "directory of workspace databases (hosted mode)")
	if err := fs.Parse(args); err != nil {
		return backupUsage(stderr)
	}
	if fs.NArg() != 1 {
		return backupUsage(stderr)
	}
	dest := fs.Arg(0)

	dbSet := false
	fs.Visit(func(f *flag.Flag) {
		if f.Name == "db" {
			dbSet = true
		}
	})
	if *dir != "" {
		if dbSet || os.Getenv("TSTAMPS_DB") != "" {
			fmt.Fprintln(stderr, "tstamps-server backup: set either --db or --workspace-dir, not both")
			return 1
		}
		n, err := workspace.BackupAll(*dir, dest)
		if err != nil {
			fmt.Fprintf(stderr, "tstamps-server backup: %v\n", err)
			return 1
		}
		fmt.Fprintf(stdout, "backed up %d workspaces from %s to %s\n", n, *dir, dest)
		return 0
	}

	// Open would create a missing file, and a typo must not look like a backup.
	if _, err := os.Stat(*db); err != nil {
		fmt.Fprintf(stderr, "tstamps-server backup: %v\n", err)
		return 1
	}
	st, err := store.Open(*db)
	if err != nil {
		fmt.Fprintf(stderr, "tstamps-server backup: open %s: %v\n", *db, err)
		return 1
	}
	defer st.Close()

	if err := st.BackupTo(dest); err != nil {
		fmt.Fprintf(stderr, "tstamps-server backup: %v\n", err)
		return 1
	}
	fmt.Fprintf(stdout, "backed up %s to %s\n", *db, dest)
	return 0
}

func backupUsage(stderr io.Writer) int {
	fmt.Fprintln(stderr, "usage: tstamps-server backup [--db path | --workspace-dir dir] <destination>")
	return 2
}
