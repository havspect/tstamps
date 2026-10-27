package audit_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/havspect/tstamps/internal/audit"
	"github.com/havspect/tstamps/internal/auth"
	"github.com/havspect/tstamps/internal/authz"
)

type mockClock struct {
	now time.Time
}

func (m mockClock) Now() time.Time {
	return m.now
}

type mockRepo struct {
	recorded []*audit.AuditEvent
	queryFn  func(ctx context.Context, filter audit.QueryFilter) ([]audit.AuditEvent, error)
}

func (m *mockRepo) Record(ctx context.Context, e *audit.AuditEvent) error {
	m.recorded = append(m.recorded, e)
	return nil
}

func (m *mockRepo) Query(ctx context.Context, filter audit.QueryFilter) ([]audit.AuditEvent, error) {
	if m.queryFn != nil {
		return m.queryFn(ctx, filter)
	}
	return nil, nil
}

func TestRecord(t *testing.T) {
	now := time.Date(2026, 10, 27, 10, 0, 0, 0, time.UTC)
	clk := mockClock{now: now}
	repo := &mockRepo{}
	svc := audit.New(repo, clk)

	t.Run("with authenticated principal", func(t *testing.T) {
		p := &auth.Principal{
			WorkspaceID: 10,
			UserID:      42,
			Roles:       []string{"admin"},
			ClientID:    "cli-client",
			TokenID:     "99",
		}
		ctx := auth.WithPrincipal(context.Background(), p)

		changes := map[string]string{"name": "Project Alpha"}
		err := svc.Record(ctx, "project.create", "project", 101, changes)
		if err != nil {
			t.Fatalf("Record failed: %v", err)
		}

		if len(repo.recorded) != 1 {
			t.Fatalf("expected 1 recorded event, got %d", len(repo.recorded))
		}
		evt := repo.recorded[0]
		if evt.WorkspaceID != 10 {
			t.Errorf("expected WorkspaceID=10, got %d", evt.WorkspaceID)
		}
		if evt.ActorUserID != 42 {
			t.Errorf("expected ActorUserID=42, got %d", evt.ActorUserID)
		}
		if evt.ActorRole != "admin" {
			t.Errorf("expected ActorRole='admin', got %q", evt.ActorRole)
		}
		if evt.ActorClientID != "cli-client" {
			t.Errorf("expected ActorClientID='cli-client', got %q", evt.ActorClientID)
		}
		if evt.ActorTokenID == nil || *evt.ActorTokenID != 99 {
			t.Errorf("expected ActorTokenID=99, got %v", evt.ActorTokenID)
		}
		if evt.Action != "project.create" || evt.TargetType != "project" || evt.TargetID != 101 {
			t.Errorf("unexpected event details: %+v", evt)
		}
		if evt.Changes != `{"name":"Project Alpha"}` {
			t.Errorf("unexpected Changes JSON: %q", evt.Changes)
		}
		if !evt.At.Equal(now) {
			t.Errorf("expected At=%v, got %v", now, evt.At)
		}
	})

	t.Run("without principal", func(t *testing.T) {
		repo.recorded = nil
		err := svc.Record(context.Background(), "system.cleanup", "system", 0, nil)
		if err != nil {
			t.Fatalf("Record failed: %v", err)
		}
		if len(repo.recorded) != 1 {
			t.Fatalf("expected 1 recorded event, got %d", len(repo.recorded))
		}
		evt := repo.recorded[0]
		if evt.ActorUserID != 0 || evt.ActorRole != "" {
			t.Errorf("expected zero actor details, got UID=%d Role=%q", evt.ActorUserID, evt.ActorRole)
		}
		if evt.Changes != "{}" {
			t.Errorf("expected '{}' changes, got %q", evt.Changes)
		}
	})
}

func TestQuery(t *testing.T) {
	now := time.Now()
	repo := &mockRepo{}
	svc := audit.New(repo, mockClock{now: now})

	t.Run("unauthorized when no principal", func(t *testing.T) {
		_, err := svc.Query(context.Background(), audit.QueryFilter{})
		if !errors.Is(err, authz.ErrUnauthorized) {
			t.Errorf("expected ErrUnauthorized, got %v", err)
		}
	})

	t.Run("forbidden when non-admin", func(t *testing.T) {
		ctx := auth.WithPrincipal(context.Background(), &auth.Principal{
			UserID: 5,
			Roles:  []string{"member"},
		})
		_, err := svc.Query(ctx, audit.QueryFilter{})
		if !errors.Is(err, authz.ErrForbidden) {
			t.Errorf("expected ErrForbidden, got %v", err)
		}
	})

	t.Run("success when admin with limit normalization", func(t *testing.T) {
		var receivedFilter audit.QueryFilter
		repo.queryFn = func(ctx context.Context, filter audit.QueryFilter) ([]audit.AuditEvent, error) {
			receivedFilter = filter
			return []audit.AuditEvent{{ID: 1, Action: "user.login"}}, nil
		}

		ctx := auth.WithPrincipal(context.Background(), &auth.Principal{
			UserID: 1,
			Roles:  []string{"admin"},
		})

		// Limit <= 0 should default to 200
		res, err := svc.Query(ctx, audit.QueryFilter{Limit: 0})
		if err != nil {
			t.Fatalf("Query failed: %v", err)
		}
		if len(res) != 1 || res[0].ID != 1 {
			t.Errorf("unexpected query result: %+v", res)
		}
		if receivedFilter.Limit != 200 {
			t.Errorf("expected limit=200, got %d", receivedFilter.Limit)
		}

		// Limit > 1000 should cap at 1000
		_, err = svc.Query(ctx, audit.QueryFilter{Limit: 5000})
		if err != nil {
			t.Fatalf("Query failed: %v", err)
		}
		if receivedFilter.Limit != 1000 {
			t.Errorf("expected capped limit=1000, got %d", receivedFilter.Limit)
		}
	})
}
