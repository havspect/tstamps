package workspace

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/havspect/tstamps/internal/auth"
	"github.com/havspect/tstamps/internal/store"
)

func TestValidateSlug(t *testing.T) {
	for _, slug := range []string{"a", "example.com", "team-1.example"} {
		if err := ValidateSlug(slug); err != nil {
			t.Errorf("ValidateSlug(%q) = %v, want nil", slug, err)
		}
	}
	for _, slug := range []string{"", "Example.com", ".hidden", "trailing.", "a..b", "../x", "a/b", strings.Repeat("a", 64)} {
		if err := ValidateSlug(slug); !errors.Is(err, ErrInvalidSlug) {
			t.Errorf("ValidateSlug(%q) = %v, want ErrInvalidSlug", slug, err)
		}
	}
}

func TestCreateMakesAdmin(t *testing.T) {
	db := filepath.Join(t.TempDir(), "tstamps.db")
	password, err := Create(db, "example.com", "ada@example.com", "Ada Lovelace")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if _, err := os.Stat(db); err != nil {
		t.Fatalf("database was not created: %v", err)
	}

	st, err := openWorkspace(t, db, "example.com")
	if err != nil {
		t.Fatalf("open workspace: %v", err)
	}
	u, err := st.UserByEmail("ada@example.com")
	if err != nil {
		t.Fatalf("admin missing: %v", err)
	}
	if !u.IsAdmin || !u.Active || u.Name != "Ada Lovelace" {
		t.Fatalf("admin = %+v, want an active admin", u)
	}
	if !auth.VerifyPassword(u.PasswordHash, password) {
		t.Fatal("printed password does not verify")
	}

	if _, err := Create(db, "example.com", "other@example.com", "Other"); err == nil {
		t.Fatal("second Create overwrote or accepted an existing workspace")
	}
}

func TestBackup(t *testing.T) {
	db := filepath.Join(t.TempDir(), "tstamps.db")
	for _, slug := range []string{"alpha", "beta.example"} {
		if _, err := Create(db, slug, "admin@"+slug, "Admin"); err != nil {
			t.Fatalf("create %s: %v", slug, err)
		}
	}

	dest := filepath.Join(t.TempDir(), "snapshot-2026-10-24.db")
	n, err := Backup(db, dest)
	if err != nil {
		t.Fatalf("Backup: %v", err)
	}
	if n != 2 {
		t.Fatalf("Backup backed up %d, want 2", n)
	}
	info, err := os.Stat(dest)
	if err != nil {
		t.Fatalf("stat backup: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Fatalf("backup mode = %o, want 600", perm)
	}
	reg, err := store.OpenRegistry(dest)
	if err != nil {
		t.Fatalf("open backup: %v", err)
	}
	defer reg.Close()
	for _, slug := range []string{"alpha", "beta.example"} {
		w, err := reg.WorkspaceBySlug(slug)
		if err != nil {
			t.Errorf("backup missing workspace %s: %v", slug, err)
			continue
		}
		if _, err := reg.ForWorkspace(w.ID).UserByEmail("admin@" + slug); err != nil {
			t.Errorf("admin missing from backup %s: %v", slug, err)
		}
	}

	if _, err := Backup(db, dest); err == nil {
		t.Fatal("Backup replaced an existing destination")
	}
	if _, err := Backup(db, db); err == nil {
		t.Fatal("Backup accepted the source database as its destination")
	}
}

// openWorkspace returns a store scoped to slug in the database at db. The
// registry is kept open until the test ends; Store.Close is a no-op on it.
func openWorkspace(t *testing.T, db, slug string) (*store.Store, error) {
	t.Helper()
	reg, err := store.OpenRegistry(db)
	if err != nil {
		return nil, err
	}
	w, err := reg.WorkspaceBySlug(slug)
	if err != nil {
		_ = reg.Close()
		return nil, err
	}
	t.Cleanup(func() { _ = reg.Close() })
	return reg.ForWorkspace(w.ID), nil
}
