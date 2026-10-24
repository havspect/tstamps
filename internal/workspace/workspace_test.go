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
	dir := filepath.Join(t.TempDir(), "workspaces")
	password, err := Create(dir, "example.com", "ada@example.com", "Ada Lovelace")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	db, err := DatabasePath(dir, "example.com")
	if err != nil {
		t.Fatalf("DatabasePath: %v", err)
	}
	if _, err := os.Stat(db); err != nil {
		t.Fatalf("database was not published: %v", err)
	}

	st, err := store.Open(db)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer st.Close()
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

	if _, err := Create(dir, "example.com", "other@example.com", "Other"); err == nil {
		t.Fatal("second Create overwrote or accepted an existing workspace")
	}
}

func TestBackupAll(t *testing.T) {
	source := filepath.Join(t.TempDir(), "workspaces")
	for _, slug := range []string{"alpha", "beta.example"} {
		if _, err := Create(source, slug, "admin@"+slug, "Admin"); err != nil {
			t.Fatalf("create %s: %v", slug, err)
		}
	}

	dest := filepath.Join(t.TempDir(), "snapshot-2026-10-24")
	n, err := BackupAll(source, dest)
	if err != nil {
		t.Fatalf("BackupAll: %v", err)
	}
	if n != 2 {
		t.Fatalf("BackupAll backed up %d, want 2", n)
	}
	for _, slug := range []string{"alpha", "beta.example"} {
		db := filepath.Join(dest, slug+".db")
		st, err := store.Open(db)
		if err != nil {
			t.Fatalf("open backup %s: %v", slug, err)
		}
		if _, err := st.UserByEmail("admin@" + slug); err != nil {
			t.Errorf("admin missing from backup %s: %v", slug, err)
		}
		_ = st.Close()
	}

	if _, err := BackupAll(source, dest); err == nil {
		t.Fatal("BackupAll replaced an existing destination")
	}
	if _, err := BackupAll(source, filepath.Join(source, "nested")); err == nil {
		t.Fatal("BackupAll accepted a destination inside the source")
	}
}
