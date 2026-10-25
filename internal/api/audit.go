package api

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"github.com/havspect/tstamps/internal/store"
)

// actorOf is the audit actor: the principal, the token it used, and the role its
// authority came from.
func (s *Server) actorOf(ctx context.Context, u *store.User) store.AuditActor {
	a := store.AuditActor{UserID: u.ID, Role: "lead"}
	if u.IsAdmin {
		a.Role = "admin"
	}
	if ri := requestFrom(ctx); ri != nil && ri.Token != nil {
		a.TokenID = &ri.Token.ID
	}
	return a
}

// auditRoutes registers the admin-only audit log read.
func (s *Server) auditRoutes(api huma.API) {
	huma.Register(api, huma.Operation{
		OperationID: "audit-list",
		Method:      http.MethodGet,
		Path:        apiPrefix + "/audit",
		Tags:        []string{"audit"},
		Errors:      []int{http.StatusUnauthorized, http.StatusForbidden, http.StatusUnprocessableEntity, http.StatusInternalServerError},
	}, s.listAudit)
}

type auditEventResponse struct {
	ID          uint            `json:"id"`
	At          time.Time       `json:"at"`
	ActorUserID uint            `json:"actor_user_id"`
	ActorEmail  string          `json:"actor_email"`
	ActorName   string          `json:"actor_name"`
	ActorRole   string          `json:"actor_role"`
	Action      string          `json:"action"`
	TargetType  string          `json:"target_type"`
	TargetID    uint            `json:"target_id"`
	Changes     json.RawMessage `json:"changes"`
}

type auditListInput struct {
	From        string `query:"from" doc:"Inclusive start date, YYYY-MM-DD. Given with to."`
	To          string `query:"to" doc:"Exclusive end date, YYYY-MM-DD. Given with from."`
	ActorUserID uint   `query:"actor_user_id" doc:"Only events by this user."`
	Action      string `query:"action" doc:"Only events with this action, e.g. user.update."`
	TargetType  string `query:"target_type" doc:"Only events about this target type, e.g. project."`
	TargetID    uint   `query:"target_id" doc:"Only events about this target id."`
	Limit       int    `query:"limit" doc:"Maximum events to return; default 200, at most 1000."`
}

type auditListOutput struct {
	Body struct {
		Events []auditEventResponse `json:"events"`
	}
}

func (s *Server) listAudit(ctx context.Context, in *auditListInput) (*auditListOutput, error) {
	if _, err := requireAdmin(ctx); err != nil {
		return nil, err
	}
	f := store.AuditFilter{
		ActorUserID: in.ActorUserID,
		Action:      in.Action,
		TargetType:  in.TargetType,
		TargetID:    in.TargetID,
		Limit:       in.Limit,
	}
	if in.From != "" || in.To != "" {
		// The limit already bounds the reply, so the log needs no span cap.
		rng, err := parseDates(in.From, in.To)
		if err != nil {
			return nil, err
		}
		f.From, f.To = rng.From, rng.To
	}
	events, err := s.Store.AuditEvents(f)
	if err != nil {
		return nil, internal(err)
	}

	ids := make([]uint, 0, len(events))
	seen := make(map[uint]bool, len(events))
	for _, e := range events {
		if !seen[e.ActorUserID] {
			seen[e.ActorUserID] = true
			ids = append(ids, e.ActorUserID)
		}
	}
	users, err := s.Store.UsersByIDs(ids)
	if err != nil {
		return nil, internal(err)
	}
	byID := make(map[uint]store.User, len(users))
	for _, u := range users {
		byID[u.ID] = u
	}

	out := &auditListOutput{}
	out.Body.Events = make([]auditEventResponse, 0, len(events))
	for _, e := range events {
		actor := byID[e.ActorUserID]
		out.Body.Events = append(out.Body.Events, auditEventResponse{
			ID: e.ID, At: e.At,
			ActorUserID: e.ActorUserID, ActorEmail: actor.Email, ActorName: actor.Name, ActorRole: e.ActorRole,
			Action: e.Action, TargetType: e.TargetType, TargetID: e.TargetID,
			Changes: json.RawMessage(e.Changes),
		})
	}
	return out, nil
}
