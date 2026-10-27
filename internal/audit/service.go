package audit

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/havspect/tstamps/internal/auth"
	"github.com/havspect/tstamps/internal/authz"
	"github.com/havspect/tstamps/internal/clock"
)

// Service coordinates audit event creation and access control.
type Service struct {
	repo  Repository
	clock clock.Clock
}

// New creates a new audit Service.
func New(repo Repository, clk clock.Clock) *Service {
	if clk == nil {
		clk = clock.System{}
	}
	return &Service{
		repo:  repo,
		clock: clk,
	}
}

// Record creates and appends an AuditEvent using actor provenance from ctx.
func (s *Service) Record(ctx context.Context, action, targetType string, targetID uint, changes any) error {
	var (
		wsID      uint
		actorUID  uint
		tokenID   *uint
		clientID  string
		grantID   string
		actorRole string
	)

	if p, ok := auth.PrincipalFrom(ctx); ok && p != nil {
		wsID = p.WorkspaceID
		actorUID = p.UserID
		clientID = p.ClientID
		if p.TokenID != "" {
			if tid, err := strconv.ParseUint(p.TokenID, 10, 64); err == nil {
				uTid := uint(tid)
				tokenID = &uTid
			}
		}
		if p.IsAdmin() {
			actorRole = "admin"
		} else if p.HasRole("lead") {
			actorRole = "lead"
		} else if len(p.Roles) > 0 {
			actorRole = p.Roles[0]
		}
	}

	changesStr := "{}"
	if changes != nil {
		switch v := changes.(type) {
		case string:
			if v != "" {
				changesStr = v
			}
		case []byte:
			if len(v) > 0 {
				changesStr = string(v)
			}
		default:
			b, err := json.Marshal(v)
			if err != nil {
				return fmt.Errorf("marshal audit changes: %w", err)
			}
			changesStr = string(b)
		}
	}

	now := s.clock.Now()
	event := &AuditEvent{
		WorkspaceID:   wsID,
		At:            now,
		ActorUserID:   actorUID,
		ActorTokenID:  tokenID,
		ActorClientID: clientID,
		ActorGrantID:  grantID,
		ActorRole:     actorRole,
		Action:        action,
		TargetType:    targetType,
		TargetID:      targetID,
		Changes:       changesStr,
		CreatedAt:     now,
	}

	return s.repo.Record(ctx, event)
}

// Query retrieves audit events matching filter. Only administrators are authorized.
func (s *Service) Query(ctx context.Context, filter QueryFilter) ([]AuditEvent, error) {
	p, ok := auth.PrincipalFrom(ctx)
	if !ok || p == nil {
		return nil, authz.ErrUnauthorized
	}
	if !p.IsAdmin() {
		return nil, authz.ErrForbidden
	}

	if filter.Limit <= 0 {
		filter.Limit = 200
	} else if filter.Limit > 1000 {
		filter.Limit = 1000
	}

	return s.repo.Query(ctx, filter)
}
