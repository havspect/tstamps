package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/havspect/tstamps/internal/auth"
	"github.com/havspect/tstamps/internal/store"
)

func TestUsageErrors(t *testing.T) {
	cases := []struct {
		name string
		args []string
	}{
		{"none", nil},
		{"unknown command", []string{"unknown"}},
		{"admin without subcommand", []string{"admin"}},
		{"unknown admin subcommand", []string{"admin", "delete"}},
		{"backup without destination", []string{"backup"}},
		{"workspace without subcommand", []string{"workspace"}},
		{"workspace create without slug", []string{"workspace", "create", "--email", "a@example.com", "--name", "Ada", "--workspace-dir", t.TempDir()}},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			if code := run(tt.args, &stdout, &stderr); code != 2 {
				t.Errorf("run(%q) exit %d, want 2", tt.args, code)
			}
			if stderr.Len() == 0 {
				t.Errorf("run(%q) wrote no usage to stderr", tt.args)
			}
		})
	}
}

func TestAdminCreate(t *testing.T) {
	db := filepath.Join(t.TempDir(), "tstamps.db")

	var stdout, stderr bytes.Buffer
	code := run([]string{"admin", "create", "--email", "ada@example.com", "--name", "Ada Lovelace", "--db", db}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit %d, want 0 (stderr %q)", code, stderr.String())
	}
	if n := strings.Count(stdout.String(), "\n"); n != 1 {
		t.Fatalf("stdout has %d lines, want exactly 1: %q", n, stdout.String())
	}
	if !strings.Contains(stdout.String(), "ada@example.com") {
		t.Fatalf("stdout %q does not name the admin", stdout.String())
	}
	if strings.Contains(stderr.String(), "initial password") {
		t.Fatalf("password leaked to stderr: %q", stderr.String())
	}

	password := passwordFrom(t, stdout.String())

	st, err := store.Open(db)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer st.Close()

	u, err := st.UserByEmail("ada@example.com")
	if err != nil {
		t.Fatalf("load created user: %v", err)
	}
	if !u.IsAdmin || !u.Active {
		t.Fatalf("user admin=%v active=%v, want both true", u.IsAdmin, u.Active)
	}
	if u.Name != "Ada Lovelace" {
		t.Fatalf("name %q, want %q", u.Name, "Ada Lovelace")
	}
	if !auth.VerifyPassword(u.PasswordHash, password) {
		t.Fatalf("printed password does not verify against the stored hash")
	}
}

func TestAdminCreateUsesEnvDB(t *testing.T) {
	db := filepath.Join(t.TempDir(), "env.db")
	t.Setenv("TSTAMPS_DB", db)

	var stdout, stderr bytes.Buffer
	if code := run([]string{"admin", "create", "--email", "grace@example.com", "--name", "Grace Hopper"}, &stdout, &stderr); code != 0 {
		t.Fatalf("exit %d, want 0 (stderr %q)", code, stderr.String())
	}
	if _, err := os.Stat(db); err != nil {
		t.Fatalf("TSTAMPS_DB %s was not written: %v", db, err)
	}

	st, err := store.Open(db)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer st.Close()

	if _, err := st.UserByEmail("grace@example.com"); err != nil {
		t.Fatalf("admin not in TSTAMPS_DB database: %v", err)
	}
}

func TestAdminCreateDuplicateEmail(t *testing.T) {
	db := filepath.Join(t.TempDir(), "tstamps.db")
	args := []string{"admin", "create", "--email", "ada@example.com", "--name", "Ada", "--db", db}

	var stdout, stderr bytes.Buffer
	if code := run(args, &stdout, &stderr); code != 0 {
		t.Fatalf("first create exit %d, want 0 (stderr %q)", code, stderr.String())
	}

	stdout.Reset()
	stderr.Reset()
	if code := run(args, &stdout, &stderr); code != 1 {
		t.Fatalf("second create exit %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "already exists") {
		t.Fatalf("stderr %q does not explain the duplicate", stderr.String())
	}
	if stdout.Len() != 0 {
		t.Fatalf("second create printed %q, want nothing", stdout.String())
	}
}

func TestAdminCreateMissingEmail(t *testing.T) {
	db := filepath.Join(t.TempDir(), "tstamps.db")
	var stdout, stderr bytes.Buffer
	if code := run([]string{"admin", "create", "--name", "Ada", "--db", db}, &stdout, &stderr); code != 2 {
		t.Fatalf("exit %d, want 2", code)
	}
}

// passwordFrom extracts the only secret on the single output line.
func passwordFrom(t *testing.T, line string) string {
	t.Helper()
	const marker = "initial password: "
	i := strings.Index(line, marker)
	if i < 0 {
		t.Fatalf("output %q has no %q", line, marker)
	}
	return strings.TrimSpace(line[i+len(marker):])
}

func TestBackup(t *testing.T) {
	dir := t.TempDir()
	db := filepath.Join(dir, "tstamps.db")

	var stdout, stderr bytes.Buffer
	if code := run([]string{"admin", "create", "--email", "ada@example.com", "--name", "Ada", "--db", db}, &stdout, &stderr); code != 0 {
		t.Fatalf("admin create exit %d: %s", code, stderr.String())
	}

	dest := filepath.Join(dir, "snapshot.db")
	stdout.Reset()
	stderr.Reset()
	if code := run([]string{"backup", "--db", db, dest}, &stdout, &stderr); code != 0 {
		t.Fatalf("backup exit %d: %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), dest) {
		t.Fatalf("stdout %q does not name the destination", stdout.String())
	}

	backup, err := store.Open(dest)
	if err != nil {
		t.Fatalf("open backup: %v", err)
	}
	defer backup.Close()
	if _, err := backup.UserByEmail("ada@example.com"); err != nil {
		t.Fatalf("admin missing from backup: %v", err)
	}

	// A second backup to the same name must fail rather than replace it.
	if code := run([]string{"backup", "--db", db, dest}, &stdout, &stderr); code != 1 {
		t.Fatalf("backup over an existing file exit %d, want 1", code)
	}
}

func TestWorkspaceCreate(t *testing.T) {
	dir := t.TempDir()
	var stdout, stderr bytes.Buffer
	code := run([]string{"workspace", "create", "example.com", "--email", "ada@example.com", "--name", "Ada Lovelace", "--workspace-dir", dir}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit %d, want 0 (stderr %q)", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "example.com") || strings.Count(stdout.String(), "\n") != 1 {
		t.Fatalf("stdout = %q, want one line naming the workspace", stdout.String())
	}
	password := passwordFrom(t, stdout.String())

	st, err := store.Open(filepath.Join(dir, "example.com.db"))
	if err != nil {
		t.Fatalf("open workspace: %v", err)
	}
	defer st.Close()
	u, err := st.UserByEmail("ada@example.com")
	if err != nil {
		t.Fatalf("admin missing: %v", err)
	}
	if !u.IsAdmin || !auth.VerifyPassword(u.PasswordHash, password) {
		t.Fatal("workspace admin or printed password is wrong")
	}

	// An invalid slug is refused without writing a file.
	stdout.Reset()
	stderr.Reset()
	if code := run([]string{"workspace", "create", "../bad", "--email", "x@example.com", "--name", "X", "--workspace-dir", dir}, &stdout, &stderr); code != 1 {
		t.Fatalf("invalid slug exit %d, want 1", code)
	}
}

func TestBackupWorkspaces(t *testing.T) {
	dir := t.TempDir()
	for _, slug := range []string{"alpha", "beta"} {
		var stdout, stderr bytes.Buffer
		if code := run([]string{"workspace", "create", slug, "--email", "admin@" + slug + ".example", "--name", "Admin", "--workspace-dir", dir}, &stdout, &stderr); code != 0 {
			t.Fatalf("create %s exit %d: %s", slug, code, stderr.String())
		}
	}

	dest := filepath.Join(t.TempDir(), "snapshot")
	var stdout, stderr bytes.Buffer
	if code := run([]string{"backup", "--workspace-dir", dir, dest}, &stdout, &stderr); code != 0 {
		t.Fatalf("backup exit %d: %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "2 workspaces") {
		t.Fatalf("stdout = %q, want the workspace count", stdout.String())
	}
	for _, slug := range []string{"alpha", "beta"} {
		if _, err := os.Stat(filepath.Join(dest, slug+".db")); err != nil {
			t.Errorf("backup missing %s: %v", slug, err)
		}
	}

	// The hosted directory cannot be mixed with a single database path.
	stderr.Reset()
	if code := run([]string{"backup", "--workspace-dir", dir, "--db", "one.db", filepath.Join(t.TempDir(), "other")}, &stdout, &stderr); code != 1 {
		t.Fatalf("mixed backup exit %d, want 1", code)
	}
}

func TestBackupMissingDatabase(t *testing.T) {
	dir := t.TempDir()
	db := filepath.Join(dir, "missing.db")

	var stdout, stderr bytes.Buffer
	if code := run([]string{"backup", "--db", db, filepath.Join(dir, "snapshot.db")}, &stdout, &stderr); code != 1 {
		t.Fatalf("backup missing database exit %d, want 1", code)
	}
	if _, err := os.Stat(db); !os.IsNotExist(err) {
		t.Fatalf("backup created the missing database %s", db)
	}
}
