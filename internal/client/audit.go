package client

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// AuditEvent is one recorded action: the actor and the role its authority came
// from, the action, the target, and the audited fields before and after.
type AuditEvent struct {
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

// AuditQuery narrows the log. From and To are YYYY-MM-DD and go together; the
// server defaults Limit to 200 and caps it at 1000.
type AuditQuery struct {
	From, To    string
	ActorUserID uint
	Action      string
	TargetType  string
	TargetID    uint
	Limit       int
}

func (q AuditQuery) values() url.Values {
	v := url.Values{}
	setString(v, "from", q.From)
	setString(v, "to", q.To)
	setUint(v, "actor_user_id", q.ActorUserID)
	setString(v, "action", q.Action)
	setString(v, "target_type", q.TargetType)
	setUint(v, "target_id", q.TargetID)
	if q.Limit != 0 {
		v.Set("limit", strconv.Itoa(q.Limit))
	}
	return v
}

// ListAudit returns audit events, newest first; admin only.
func (c *Client) ListAudit(q AuditQuery) ([]AuditEvent, error) {
	var out struct {
		Events []AuditEvent `json:"events"`
	}
	if err := c.do(http.MethodGet, "/audit?"+q.values().Encode(), nil, &out); err != nil {
		return nil, err
	}
	return out.Events, nil
}
