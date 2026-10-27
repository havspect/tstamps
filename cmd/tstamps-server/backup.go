package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/havspect/tstamps/internal/config"
	gormrepo "github.com/havspect/tstamps/internal/repository/gorm"
)

// backup writes a consistent snapshot of the live database. The destination must
// not exist, so a nightly name never overwrites yesterday's.
func backup(args []string, stdout, stderr io.Writer) int {
	cfg, err := config.Load(nil, os.Getenv)
	if err != nil {
		fmt.Fprintf(stderr, "tstamps-server backup: %v\n", err)
		return 1
	}

	fsCmd := flag.NewFlagSet("backup", flag.ContinueOnError)
	fsCmd.SetOutput(io.Discard) // the caller reports the error in one line
	db := fsCmd.String("db", cfg.DB, "sqlite database file")
	if err := fsCmd.Parse(args); err != nil || fsCmd.NArg() != 1 || *db == "" {
		return backupUsage(stderr)
	}
	dest := fsCmd.Arg(0)

	n, err := runBackup(*db, dest)
	if err != nil {
		fmt.Fprintf(stderr, "tstamps-server backup: %v\n", err)
		return 1
	}
	fmt.Fprintf(stdout, "backed up %d workspaces from %s to %s\n", n, *db, dest)
	return 0
}

func runBackup(dbPath, dest string) (int, error) {
	if _, err := os.Stat(dbPath); err != nil {
		return 0, errors.New("no workspaces to back up")
	}

	db, err := gormrepo.Open(dbPath)
	if err != nil {
		return 0, err
	}
	sqlDB, _ := db.DB()
	if sqlDB != nil {
		defer sqlDB.Close()
	}

	var count int64
	if err := db.Model(&gormrepo.Workspace{}).Count(&count).Error; err != nil {
		return 0, err
	}
	if count == 0 {
		return 0, errors.New("no workspaces to back up")
	}

	if _, err := os.Stat(dest); !errors.Is(err, os.ErrNotExist) {
		return 0, fmt.Errorf("destination %s already exists", dest)
	}

	stage, err := os.MkdirTemp(filepath.Dir(dest), "."+filepath.Base(dest)+".tmp-")
	if err != nil {
		return 0, err
	}
	defer os.RemoveAll(stage)
	stageFile := filepath.Join(stage, "snapshot.db")

	if err := db.Exec("VACUUM INTO ?", stageFile).Error; err != nil {
		return 0, err
	}
	// The snapshot holds password and session hashes; do not inherit the umask.
	if err := os.Chmod(stageFile, 0o600); err != nil {
		return 0, err
	}
	// Link, not rename: link refuses an existing destination atomically, so two
	// overlapping backups cannot replace each other.
	if err := os.Link(stageFile, dest); err != nil {
		if errors.Is(err, fs.ErrExist) {
			return 0, fmt.Errorf("destination %s already exists", dest)
		}
		return 0, err
	}

	return int(count), nil
}

func backupUsage(stderr io.Writer) int {
	fmt.Fprintln(stderr, "usage: tstamps-server backup [--db path] <destination>")
	return 2
}
