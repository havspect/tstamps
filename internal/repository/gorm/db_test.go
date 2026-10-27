package gorm

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	gormdriver "gorm.io/gorm"
)

func newTestDB(t *testing.T) *gormdriver.DB {
	t.Helper()
	db, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open(:memory:): %v", err)
	}
	if err := Migrate(db); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	return db
}

func TestOpenAndMigrate(t *testing.T) {
	db := newTestDB(t)

	expectedTables := []string{
		"workspaces",
		"pending_signups",
		"server_secrets",
		"users",
		"sessions",
		"api_tokens",
		"teams",
		"team_memberships",
		"projects",
		"entries",
		"audit_log",
		"o_auth_clients",
		"o_auth_codes",
		"o_auth_access_tokens",
		"o_auth_refresh_tokens",
		"o_auth_device_codes",
	}

	for _, table := range expectedTables {
		if !db.Migrator().HasTable(table) {
			t.Errorf("expected table %q to exist", table)
		}
	}

	// Verify server_secrets row ID 1 exists
	var secret ServerSecret
	if err := db.First(&secret, 1).Error; err != nil {
		t.Fatalf("failed to find server secret 1: %v", err)
	}
	if len(secret.Secret) != 32 {
		t.Errorf("expected 32-byte secret, got %d bytes", len(secret.Secret))
	}

	// Ensure Migrate is idempotent
	if err := Migrate(db); err != nil {
		t.Fatalf("second Migrate failed: %v", err)
	}
}

func TestFilePermissionsAndPragmas(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.db")

	db, err := Open(path)
	if err != nil {
		t.Fatalf("Open(%s): %v", path, err)
	}
	if err := Migrate(db); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	fi, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if perm := fi.Mode().Perm(); perm != 0o600 {
		t.Errorf("expected file mode 0600, got %o", perm)
	}

	var fk int
	if err := db.Raw("PRAGMA foreign_keys").Scan(&fk).Error; err != nil {
		t.Fatalf("check foreign_keys: %v", err)
	}
	if fk != 1 {
		t.Errorf("expected foreign_keys=1, got %d", fk)
	}
}

func TestTenancyCallbacks(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()

	// 1. Non-tenant models can be queried and inserted without workspace ID in context
	ws := &Workspace{Slug: "acme"}
	if err := db.WithContext(ctx).Create(ws).Error; err != nil {
		t.Fatalf("create workspace without wsID: %v", err)
	}
	var foundWs Workspace
	if err := db.WithContext(ctx).First(&foundWs, ws.ID).Error; err != nil {
		t.Fatalf("find workspace without wsID: %v", err)
	}

	// 2. Tenant model query without workspace ID in context must fail
	var users []User
	err := db.WithContext(ctx).Find(&users).Error
	if err == nil {
		t.Fatal("expected error querying User without workspace ID, got nil")
	}
	if !strings.Contains(err.Error(), "workspace ID required for tenant model") {
		t.Errorf("unexpected error message: %v", err)
	}

	// 3. Tenant model create without workspace ID in context must fail
	u := &User{Email: "user@example.com", Name: "Test User", PasswordHash: "x"}
	err = db.WithContext(ctx).Create(u).Error
	if err == nil {
		t.Fatal("expected error creating User without workspace ID, got nil")
	}
	if !strings.Contains(err.Error(), "workspace ID required for tenant model") {
		t.Errorf("unexpected error message: %v", err)
	}

	// 4. Create tenant models with workspace ID
	ctxWs1 := WithWorkspaceID(ctx, 1)
	ctxWs2 := WithWorkspaceID(ctx, 2)

	u1 := &User{Email: "user1@example.com", Name: "User 1", PasswordHash: "x"}
	if err := db.WithContext(ctxWs1).Create(u1).Error; err != nil {
		t.Fatalf("create user 1: %v", err)
	}
	if u1.WorkspaceID != 1 {
		t.Errorf("expected u1.WorkspaceID=1, got %d", u1.WorkspaceID)
	}

	u2 := &User{Email: "user2@example.com", Name: "User 2", PasswordHash: "x"}
	if err := db.WithContext(ctxWs2).Create(u2).Error; err != nil {
		t.Fatalf("create user 2: %v", err)
	}
	if u2.WorkspaceID != 2 {
		t.Errorf("expected u2.WorkspaceID=2, got %d", u2.WorkspaceID)
	}

	// 5. Querying with ctxWs1 sees only u1
	var ws1Users []User
	if err := db.WithContext(ctxWs1).Find(&ws1Users).Error; err != nil {
		t.Fatalf("find ws1 users: %v", err)
	}
	if len(ws1Users) != 1 || ws1Users[0].ID != u1.ID {
		t.Errorf("expected only u1 in ws1, got %+v", ws1Users)
	}

	// 6. Querying with ctxWs2 sees only u2
	var ws2Users []User
	if err := db.WithContext(ctxWs2).Find(&ws2Users).Error; err != nil {
		t.Fatalf("find ws2 users: %v", err)
	}
	if len(ws2Users) != 1 || ws2Users[0].ID != u2.ID {
		t.Errorf("expected only u2 in ws2, got %+v", ws2Users)
	}

	// 7. Mismatched workspace ID on model vs context should error
	uMismatch := &User{WorkspaceID: 99, Email: "mismatch@example.com", Name: "Mismatch", PasswordHash: "x"}
	err = db.WithContext(ctxWs1).Create(uMismatch).Error
	if err == nil {
		t.Fatal("expected error on workspace ID mismatch, got nil")
	}
	if !strings.Contains(err.Error(), "workspace ID mismatch") {
		t.Errorf("unexpected error message: %v", err)
	}

	// 8. Delete scoped to workspace
	if err := db.WithContext(ctxWs1).Delete(&User{}, u2.ID).Error; err != nil {
		t.Fatalf("delete scoped: %v", err)
	}
	// u2 in ws2 must still exist
	var u2Check User
	if err := db.WithContext(ctxWs2).First(&u2Check, u2.ID).Error; err != nil {
		t.Fatalf("u2 should still exist in ws2: %v", err)
	}
}

func TestTxRunner(t *testing.T) {
	db := newTestDB(t)
	runner := NewTxRunner(db)
	ctx := WithWorkspaceID(context.Background(), 1)

	// 1. Transaction commit
	err := runner.Run(ctx, func(txCtx context.Context) error {
		txDB := DBFrom(txCtx, db)
		if txDB == db {
			t.Error("expected DBFrom inside transaction to return tx handle, not fallback")
		}
		return txDB.Create(&User{Email: "tx@example.com", Name: "Tx User", PasswordHash: "x"}).Error
	})
	if err != nil {
		t.Fatalf("runner.Run commit: %v", err)
	}

	var u User
	if err := db.WithContext(ctx).Where("email = ?", "tx@example.com").First(&u).Error; err != nil {
		t.Fatalf("expected committed user to exist: %v", err)
	}

	// 2. Transaction rollback
	err = runner.Run(ctx, func(txCtx context.Context) error {
		txDB := DBFrom(txCtx, db)
		if err := txDB.Create(&User{Email: "rollback@example.com", Name: "Rollback User", PasswordHash: "x"}).Error; err != nil {
			return err
		}
		return errors.New("force rollback")
	})
	if err == nil {
		t.Fatal("expected error from runner.Run, got nil")
	}

	var uRollback User
	err = db.WithContext(ctx).Where("email = ?", "rollback@example.com").First(&uRollback).Error
	if !errors.Is(err, gormdriver.ErrRecordNotFound) {
		t.Fatalf("expected record not found for rolled back user, got %v", err)
	}

	// 3. DBFrom fallback when not in transaction
	fallbackDB := DBFrom(ctx, db)
	if fallbackDB == nil {
		t.Fatal("expected non-nil fallback DB")
	}
}

func TestAuditLogAppendOnly(t *testing.T) {
	db := newTestDB(t)
	ctx := WithWorkspaceID(context.Background(), 1)

	// Insert audit log row
	log := &AuditLog{
		At:          time.Now(),
		ActorUserID: 1,
		ActorRole:   "admin",
		Action:      "user.create",
		TargetType:  "user",
		TargetID:    1,
		Changes:     `{"before":{},"after":{"name":"Alice"}}`,
	}
	if err := db.WithContext(ctx).Create(log).Error; err != nil {
		t.Fatalf("failed to create audit log: %v", err)
	}

	// 1. GORM Model hook blocks BeforeUpdate
	err := db.WithContext(ctx).Model(log).Update("action", "user.modified").Error
	if err == nil {
		t.Fatal("expected error updating audit_log via GORM, got nil")
	}
	if !strings.Contains(err.Error(), "audit_log is append-only: updates forbidden") {
		t.Errorf("unexpected error: %v", err)
	}

	// 2. GORM Model hook blocks BeforeDelete
	err = db.WithContext(ctx).Delete(log).Error
	if err == nil {
		t.Fatal("expected error deleting audit_log via GORM, got nil")
	}
	if !strings.Contains(err.Error(), "audit_log is append-only: deletes forbidden") {
		t.Errorf("unexpected error: %v", err)
	}

	// 3. SQLite database trigger blocks raw UPDATE
	err = db.Exec("UPDATE audit_log SET action = 'tampered' WHERE id = ?", log.ID).Error
	if err == nil {
		t.Fatal("expected SQLite trigger to prevent raw UPDATE on audit_log, got nil")
	}
	if !strings.Contains(err.Error(), "audit_log is append-only: updates forbidden") {
		t.Errorf("unexpected error from trigger: %v", err)
	}

	// 4. SQLite database trigger blocks raw DELETE
	err = db.Exec("DELETE FROM audit_log WHERE id = ?", log.ID).Error
	if err == nil {
		t.Fatal("expected SQLite trigger to prevent raw DELETE on audit_log, got nil")
	}
	if !strings.Contains(err.Error(), "audit_log is append-only: deletes forbidden") {
		t.Errorf("unexpected error from trigger: %v", err)
	}
}

func TestEntriesOneRunningIndex(t *testing.T) {
	db := newTestDB(t)
	ctx := WithWorkspaceID(context.Background(), 1)

	now := time.Now()
	e1 := &Entry{
		UserID:    1,
		ProjectID: 10,
		StartAt:   now.Add(-1 * time.Hour),
		EndAt:     nil, // running entry
	}
	if err := db.WithContext(ctx).Create(e1).Error; err != nil {
		t.Fatalf("create first running entry: %v", err)
	}

	// Second running entry for the same user and workspace should fail unique index
	e2 := &Entry{
		UserID:    1,
		ProjectID: 20,
		StartAt:   now,
		EndAt:     nil, // running entry
	}
	err := db.WithContext(ctx).Create(e2).Error
	if err == nil {
		t.Fatal("expected error creating second running entry for same user, got nil")
	}

	// Running entry for a different user should succeed
	eOtherUser := &Entry{
		UserID:    2,
		ProjectID: 10,
		StartAt:   now,
		EndAt:     nil,
	}
	if err := db.WithContext(ctx).Create(eOtherUser).Error; err != nil {
		t.Fatalf("create running entry for other user: %v", err)
	}

	// Running entry for different workspace should succeed
	ctxWs2 := WithWorkspaceID(context.Background(), 2)
	eOtherWs := &Entry{
		UserID:    1,
		ProjectID: 10,
		StartAt:   now,
		EndAt:     nil,
	}
	if err := db.WithContext(ctxWs2).Create(eOtherWs).Error; err != nil {
		t.Fatalf("create running entry for other workspace: %v", err)
	}

	// Once the first entry is completed (EndAt set), a new running entry can be created
	completed := now
	e1.EndAt = &completed
	if err := db.WithContext(ctx).Save(e1).Error; err != nil {
		t.Fatalf("save completed entry: %v", err)
	}

	// Now e2 can be created
	e2New := &Entry{
		UserID:    1,
		ProjectID: 20,
		StartAt:   now,
		EndAt:     nil,
	}
	if err := db.WithContext(ctx).Create(e2New).Error; err != nil {
		t.Fatalf("create new running entry after completing previous: %v", err)
	}
}
