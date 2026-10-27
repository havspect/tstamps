package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"github.com/havspect/tstamps/internal/audit"
	"github.com/havspect/tstamps/internal/user"
)

type auditEventResponse struct {
	ID            uint            `json:"id"`
	At            time.Time       `json:"at"`
	ActorUserID   uint            `json:"actor_user_id"`
	ActorTokenID  *uint           `json:"actor_token_id,omitempty"`
	ActorClientID string          `json:"actor_client_id,omitempty"`
	ActorGrantID  string          `json:"actor_grant_id,omitempty"`
	ActorEmail    string          `json:"actor_email,omitempty"`
	ActorName     string          `json:"actor_name,omitempty"`
	ActorRole     string          `json:"actor_role,omitempty"`
	Action        string          `json:"action"`
	TargetType    string          `json:"target_type"`
	TargetID      uint            `json:"target_id"`
	Changes       json.RawMessage `json:"changes"`
}

func toAuditEventResponse(e *audit.AuditEvent) auditEventResponse {
	if e == nil {
		return auditEventResponse{}
	}
	changes := json.RawMessage("{}")
	if e.Changes != "" && json.Valid([]byte(e.Changes)) {
		changes = json.RawMessage(e.Changes)
	}
	return auditEventResponse{
		ID:            e.ID,
		At:            e.At,
		ActorUserID:   e.ActorUserID,
		ActorTokenID:  e.ActorTokenID,
		ActorClientID: e.ActorClientID,
		ActorGrantID:  e.ActorGrantID,
		ActorRole:     e.ActorRole,
		Action:        e.Action,
		TargetType:    e.TargetType,
		TargetID:      e.TargetID,
		Changes:       changes,
	}
}

type auditQueryInput struct {
	From        string `query:"from" doc:"Start time, RFC 3339"`
	To          string `query:"to" doc:"End time, RFC 3339"`
	ActorUserID uint   `query:"actor_user_id" doc:"Filter by actor user ID"`
	Action      string `query:"action" doc:"Filter by action"`
	TargetType  string `query:"target_type" doc:"Filter by target type"`
	TargetID    uint   `query:"target_id" doc:"Filter by target ID"`
	Limit       int    `query:"limit" doc:"Maximum records to return (default 200, max 1000)"`
}

type auditQueryOutput struct {
	Body struct {
		Events []auditEventResponse `json:"events"`
	}
}

func (s *Server) auditRoutes(api huma.API) {
	huma.Register(api, huma.Operation{
		OperationID: "audit-list",
		Method:      http.MethodGet,
		Path:        apiPrefix + "/audit",
		Tags:        []string{"audit"},
		Errors:      []int{http.StatusUnauthorized, http.StatusForbidden, http.StatusUnprocessableEntity, http.StatusInternalServerError},
	}, s.listAudit)
}

func (s *Server) listAudit(ctx context.Context, in *auditQueryInput) (*auditQueryOutput, error) {
	if s.deps.AuditService == nil {
		return nil, problem(http.StatusServiceUnavailable, urnUnavailable, "audit service not available")
	}

	filter := audit.QueryFilter{
		ActorUserID: in.ActorUserID,
		Action:      in.Action,
		TargetType:  in.TargetType,
		TargetID:    in.TargetID,
		Limit:       in.Limit,
	}

	if in.From != "" {
		f, err := time.Parse(time.RFC3339, in.From)
		if err != nil {
			return nil, problem(http.StatusUnprocessableEntity, urnInvalidInput, "invalid from format")
		}
		filter.From = f
	}

	if in.To != "" {
		t, err := time.Parse(time.RFC3339, in.To)
		if err != nil {
			return nil, problem(http.StatusUnprocessableEntity, urnInvalidInput, "invalid to format")
		}
		filter.To = t
	}

	events, err := s.deps.AuditService.Query(ctx, filter)
	if err != nil {
		return nil, fail(err)
	}

	usersMap := make(map[uint]user.User)
	if s.deps.UserService != nil {
		if users, err := s.deps.UserService.ListUsers(ctx); err == nil {
			for _, u := range users {
				usersMap[u.ID] = u
			}
		}
	}

	out := make([]auditEventResponse, len(events))
	for i, e := range events {
		resp := toAuditEventResponse(&e)
		if u, ok := usersMap[e.ActorUserID]; ok {
			resp.ActorEmail = u.Email
			resp.ActorName = u.Name
		}
		out[i] = resp
	}
	res := &auditQueryOutput{}
	res.Body.Events = out
	return res, nil
}
