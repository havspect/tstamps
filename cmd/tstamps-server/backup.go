package main

import (
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/havspect/tstamps/internal/config"
	"github.com/havspect/tstamps/internal/workspace"
)

// backup writes a consistent snapshot of the live database. The destination must
// not exist, so a nightly name never overwrites yesterday's.
func backup(args []string, stdout, stderr io.Writer) int {
	cfg, err := config.Load(nil, os.Getenv)
	if err != nil {
		fmt.Fprintf(stderr, "tstamps-server backup: %v\n", err)
		return 1
	}

	fs := flag.NewFlagSet("backup", flag.ContinueOnError)
	fs.SetOutput(io.Discard) // the caller reports the error in one line
	db := fs.String("db", cfg.DB, "sqlite database file")
	if err := fs.Parse(args); err != nil || fs.NArg() != 1 || *db == "" {
		return backupUsage(stderr)
	}
	dest := fs.Arg(0)

	n, err := workspace.Backup(*db, dest)
	if err != nil {
		fmt.Fprintf(stderr, "tstamps-server backup: %v\n", err)
		return 1
	}
	fmt.Fprintf(stdout, "backed up %d workspaces from %s to %s\n", n, *db, dest)
	return 0
}

func backupUsage(stderr io.Writer) int {
	fmt.Fprintln(stderr, "usage: tstamps-server backup [--db path] <destination>")
	return 2
}
