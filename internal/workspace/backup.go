package workspace

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/havspect/tstamps/internal/store"
)

// BackupAll snapshots every workspace database into a new destination directory.
// Each file is internally consistent; the set is not a cross-workspace point in
// time. Nothing is published unless every snapshot succeeds.
func BackupAll(source, dest string) (int, error) {
	paths, err := Databases(source)
	if err != nil {
		return 0, err
	}
	if len(paths) == 0 {
		return 0, errors.New("no workspaces to back up")
	}
	srcAbs, err := filepath.Abs(source)
	if err != nil {
		return 0, err
	}
	destAbs, err := filepath.Abs(dest)
	if err != nil {
		return 0, err
	}
	rel, err := filepath.Rel(srcAbs, destAbs)
	if err != nil {
		return 0, err
	}
	if rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))) {
		return 0, errors.New("backup destination must be outside the workspace directory")
	}
	if _, err := os.Stat(dest); !errors.Is(err, os.ErrNotExist) {
		return 0, fmt.Errorf("destination %s already exists", dest)
	}

	stage, err := os.MkdirTemp(filepath.Dir(dest), "."+filepath.Base(dest)+".tmp-")
	if err != nil {
		return 0, err
	}
	defer os.RemoveAll(stage)

	for _, path := range paths {
		slug := strings.TrimSuffix(filepath.Base(path), ".db")
		if err := backupOne(path, filepath.Join(stage, slug+".db")); err != nil {
			return 0, fmt.Errorf("%s: %w", slug, err)
		}
	}
	if err := os.Rename(stage, dest); err != nil {
		return 0, err
	}
	return len(paths), nil
}

func backupOne(source, dest string) error {
	st, err := store.Open(source)
	if err != nil {
		return err
	}
	defer st.Close()
	return st.BackupTo(dest)
}
