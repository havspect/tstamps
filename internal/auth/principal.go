package auth

import "context"

// Principal represents the canonical authenticated identity and its authorization attributes.
type Principal struct {
	WorkspaceID uint
	UserID      uint
	Email       string
	Roles       []string // "admin", "lead", "member"
	Scopes      []string // "tstamps.read", "tstamps.write"
	ClientID    string
	TokenID     string // Identifier only, never the secret value
	Source      string // "http", "mcp", "cli", "local"
	LedTeamIDs  []uint // Team IDs where user is a lead
}

// HasRole reports whether the principal holds the given role.
func (p *Principal) HasRole(role string) bool {
	if p == nil {
		return false
	}
	for _, r := range p.Roles {
		if r == role {
			return true
		}
	}
	return false
}

// HasScope reports whether the principal possesses the requested OAuth scope.
func (p *Principal) HasScope(scope string) bool {
	if p == nil {
		return false
	}
	for _, s := range p.Scopes {
		if s == scope {
			return true
		}
	}
	return false
}

// LeadsTeam reports whether the principal is a lead for the specified team ID.
func (p *Principal) LeadsTeam(teamID uint) bool {
	if p == nil {
		return false
	}
	for _, id := range p.LedTeamIDs {
		if id == teamID {
			return true
		}
	}
	return false
}

// IsAdmin reports whether the principal has the "admin" role.
func (p *Principal) IsAdmin() bool {
	return p.HasRole("admin")
}

type principalContextKey struct{}

// WithPrincipal returns a copy of parent context carrying principal p.
func WithPrincipal(ctx context.Context, p *Principal) context.Context {
	return context.WithValue(ctx, principalContextKey{}, p)
}

// PrincipalFrom extracts the Principal from ctx. If no valid non-nil
// Principal is present, it returns (nil, false).
func PrincipalFrom(ctx context.Context) (*Principal, bool) {
	if ctx == nil {
		return nil, false
	}
	p, ok := ctx.Value(principalContextKey{}).(*Principal)
	if !ok || p == nil {
		return nil, false
	}
	return p, true
}
