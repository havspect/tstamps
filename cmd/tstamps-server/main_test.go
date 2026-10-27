package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/havspect/tstamps/internal/auth"
	gormrepo "github.com/havspect/tstamps/internal/repository/gorm"
)

func TestUsageErrors(t *testing.T) {
	cases := []struct {
		name string
		args []string
	}{
		{"none", nil},
		{"unknown command", []string{"unknown"}},
		{"admin removed", []string{"admin", "create"}},
		{"backup without destination", []string{"backup"}},
		{"workspace without subcommand", []string{"workspace"}},
		{"workspace create without slug", []string{"workspace", "create", "--email", "a@example.com", "--name", "Ada", "--db", t.TempDir() + "/x.db"}},
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
	db := filepath.Join(t.TempDir(), "tstamps.db")

	var stdout, stderr bytes.Buffer
	if code := run([]string{"workspace", "create", "example.com", "--email", "ada@example.com", "--name", "Ada", "--db", db}, &stdout, &stderr); code != 0 {
		t.Fatalf("workspace create exit %d: %s", code, stderr.String())
	}

	dest := filepath.Join(t.TempDir(), "snapshot.db")
	stdout.Reset()
	stderr.Reset()
	if code := run([]string{"backup", "--db", db, dest}, &stdout, &stderr); code != 0 {
		t.Fatalf("backup exit %d: %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), dest) {
		t.Fatalf("stdout %q does not name the destination", stdout.String())
	}

	dbConn, err := gormrepo.Open(dest)
	if err != nil {
		t.Fatalf("open backup: %v", err)
	}
	sqlDB, _ := dbConn.DB()
	if sqlDB != nil {
		defer sqlDB.Close()
	}
	onbRepo := gormrepo.NewOnboardingRepository(dbConn)
	w, err := onbRepo.GetWorkspaceBySlug(context.Background(), "example.com")
	if err != nil {
		t.Fatalf("workspace missing from backup: %v", err)
	}
	userRepo := gormrepo.NewUserRepository(dbConn)
	ctx := gormrepo.WithWorkspaceID(context.Background(), w.ID)
	if _, err := userRepo.GetByEmail(ctx, "ada@example.com"); err != nil {
		t.Fatalf("admin missing from backup: %v", err)
	}

	// A second backup to the same name must fail rather than replace it.
	if code := run([]string{"backup", "--db", db, dest}, &stdout, &stderr); code != 1 {
		t.Fatalf("backup over an existing file exit %d, want 1", code)
	}
}

func TestWorkspaceCreate(t *testing.T) {
	db := filepath.Join(t.TempDir(), "tstamps.db")
	var stdout, stderr bytes.Buffer
	code := run([]string{"workspace", "create", "example.com", "--email", "ada@example.com", "--name", "Ada Lovelace", "--db", db}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit %d, want 0 (stderr %q)", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "example.com") || strings.Count(stdout.String(), "\n") != 1 {
		t.Fatalf("stdout = %q, want one line naming the workspace", stdout.String())
	}
	password := passwordFrom(t, stdout.String())

	dbConn, err := gormrepo.Open(db)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	sqlDB, _ := dbConn.DB()
	if sqlDB != nil {
		defer sqlDB.Close()
	}
	onbRepo := gormrepo.NewOnboardingRepository(dbConn)
	w, err := onbRepo.GetWorkspaceBySlug(context.Background(), "example.com")
	if err != nil {
		t.Fatalf("open workspace: %v", err)
	}
	userRepo := gormrepo.NewUserRepository(dbConn)
	ctx := gormrepo.WithWorkspaceID(context.Background(), w.ID)
	u, err := userRepo.GetByEmail(ctx, "ada@example.com")
	if err != nil {
		t.Fatalf("admin missing: %v", err)
	}
	if !u.IsAdmin || !auth.VerifyPassword(u.PasswordHash, password) {
		t.Fatal("workspace admin or printed password is wrong")
	}

	// An invalid slug is refused without adding a workspace.
	stdout.Reset()
	stderr.Reset()
	if code := run([]string{"workspace", "create", "../bad", "--email", "x@example.com", "--name", "X", "--db", db}, &stdout, &stderr); code != 1 {
		t.Fatalf("invalid slug exit %d, want 1", code)
	}
}

func TestBackupWorkspaces(t *testing.T) {
	db := filepath.Join(t.TempDir(), "tstamps.db")
	for _, slug := range []string{"alpha", "beta"} {
		var stdout, stderr bytes.Buffer
		if code := run([]string{"workspace", "create", slug, "--email", "admin@" + slug + ".example", "--name", "Admin", "--db", db}, &stdout, &stderr); code != 0 {
			t.Fatalf("create %s exit %d: %s", slug, code, stderr.String())
		}
	}

	dest := filepath.Join(t.TempDir(), "snapshot.db")
	var stdout, stderr bytes.Buffer
	if code := run([]string{"backup", "--db", db, dest}, &stdout, &stderr); code != 0 {
		t.Fatalf("backup exit %d: %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "2 workspaces") {
		t.Fatalf("stdout = %q, want the workspace count", stdout.String())
	}
	dbConn, err := gormrepo.Open(dest)
	if err != nil {
		t.Fatalf("open backup: %v", err)
	}
	sqlDB, _ := dbConn.DB()
	if sqlDB != nil {
		defer sqlDB.Close()
	}
	onbRepo := gormrepo.NewOnboardingRepository(dbConn)
	userRepo := gormrepo.NewUserRepository(dbConn)
	for _, slug := range []string{"alpha", "beta"} {
		w, err := onbRepo.GetWorkspaceBySlug(context.Background(), slug)
		if err != nil {
			t.Errorf("backup missing workspace %s: %v", slug, err)
			continue
		}
		ctx := gormrepo.WithWorkspaceID(context.Background(), w.ID)
		if _, err := userRepo.GetByEmail(ctx, "admin@"+slug+".example"); err != nil {
			t.Errorf("admin missing from backup %s: %v", slug, err)
		}
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
