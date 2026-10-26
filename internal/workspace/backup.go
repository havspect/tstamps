package workspace

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/havspect/tstamps/internal/store"
)

// Backup snapshots the one database into a new file. The snapshot is one point
// in time for every workspace and is published only when complete.
func Backup(db, dest string) (int, error) {
	if _, err := os.Stat(db); err != nil {
		return 0, errors.New("no workspaces to back up")
	}
	reg, err := store.OpenRegistry(db)
	if err != nil {
		return 0, err
	}
	defer reg.Close()
	workspaces, err := reg.Workspaces()
	if err != nil {
		return 0, err
	}
	if len(workspaces) == 0 {
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

	if err := reg.BackupTo(stageFile); err != nil {
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
	return len(workspaces), nil
}
