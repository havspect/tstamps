package gorm

import (
	"context"
	"testing"
	"time"

	"github.com/havspect/tstamps/internal/plan"
)

func TestPlanAvailabilityRepositoryLifecycle(t *testing.T) {
	db := newTestDB(t)
	repo := NewPlanRepository(db)
	ctx := context.Background()

	ws := &Workspace{Slug: "acme"}
	if err := db.WithContext(ctx).Create(ws).Error; err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	wsCtx := WithWorkspaceID(ctx, ws.ID)
	user := &User{Email: "ana@example.com", Name: "Ana", PasswordHash: "x"}
	if err := db.WithContext(wsCtx).Create(user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}

	week := time.Date(2026, 11, 2, 0, 0, 0, 0, time.UTC)
	now := time.Now()
	a := &plan.Availability{
		WorkspaceID: ws.ID, UserID: user.ID, WeekStart: week, Hours: 20,
		CreatedAt: now, UpdatedAt: now,
	}
	if err := repo.UpsertAvailability(wsCtx, a); err != nil {
		t.Fatalf("create override: %v", err)
	}
	if a.ID == 0 {
		t.Fatal("expected a row ID")
	}

	got, err := repo.GetAvailability(wsCtx, user.ID, week)
	if err != nil || got == nil || got.Hours != 20 {
		t.Fatalf("GetAvailability = %+v, %v", got, err)
	}

	// Zero is stored, not treated as a missing row.
	a.Hours = 0
	a.UpdatedAt = now.Add(time.Minute)
	if err := repo.UpsertAvailability(wsCtx, a); err != nil {
		t.Fatalf("update override: %v", err)
	}
	got, err = repo.GetAvailability(wsCtx, user.ID, week)
	if err != nil || got == nil || got.Hours != 0 || got.ID != a.ID {
		t.Fatalf("updated = %+v, %v", got, err)
	}

	list, err := repo.Availability(wsCtx, plan.Filter{
		From: week, To: week.AddDate(0, 0, 7), UserIDs: []uint{user.ID},
	})
	if err != nil || len(list) != 1 || list[0].Hours != 0 {
		t.Fatalf("Availability = %+v, %v", list, err)
	}

	// A row is invisible from another workspace.
	other := &Workspace{Slug: "other"}
	if err := db.WithContext(ctx).Create(other).Error; err != nil {
		t.Fatalf("create other workspace: %v", err)
	}
	if got, err := repo.GetAvailability(WithWorkspaceID(ctx, other.ID), user.ID, week); err != nil || got != nil {
		t.Fatalf("cross-workspace read = %+v, %v", got, err)
	}

	if err := repo.DeleteAvailability(wsCtx, user.ID, week); err != nil {
		t.Fatalf("delete override: %v", err)
	}
	if got, err := repo.GetAvailability(wsCtx, user.ID, week); err != nil || got != nil {
		t.Fatalf("deleted = %+v, %v", got, err)
	}
}
