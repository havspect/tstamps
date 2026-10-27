package auth_test

import (
	"context"
	"testing"

	"github.com/havspect/tstamps/internal/auth"
)

func TestPrincipal_HasRole(t *testing.T) {
	p := &auth.Principal{
		Roles: []string{"lead", "member"},
	}

	if !p.HasRole("lead") {
		t.Errorf("p.HasRole(%q) = false, want true", "lead")
	}
	if !p.HasRole("member") {
		t.Errorf("p.HasRole(%q) = false, want true", "member")
	}
	if p.HasRole("admin") {
		t.Errorf("p.HasRole(%q) = true, want false", "admin")
	}

	var nilP *auth.Principal
	if nilP.HasRole("admin") {
		t.Error("nil.HasRole(\"admin\") = true, want false")
	}
}

func TestPrincipal_HasScope(t *testing.T) {
	p := &auth.Principal{
		Scopes: []string{"tstamps.read", "tstamps.write"},
	}

	if !p.HasScope("tstamps.read") {
		t.Errorf("p.HasScope(%q) = false, want true", "tstamps.read")
	}
	if !p.HasScope("tstamps.write") {
		t.Errorf("p.HasScope(%q) = false, want true", "tstamps.write")
	}
	if p.HasScope("tstamps.admin") {
		t.Errorf("p.HasScope(%q) = true, want false", "tstamps.admin")
	}

	var nilP *auth.Principal
	if nilP.HasScope("tstamps.read") {
		t.Error("nil.HasScope(\"tstamps.read\") = true, want false")
	}
}

func TestPrincipal_LeadsTeam(t *testing.T) {
	p := &auth.Principal{
		LedTeamIDs: []uint{1, 5, 42},
	}

	if !p.LeadsTeam(1) {
		t.Errorf("p.LeadsTeam(1) = false, want true")
	}
	if !p.LeadsTeam(5) {
		t.Errorf("p.LeadsTeam(5) = false, want true")
	}
	if !p.LeadsTeam(42) {
		t.Errorf("p.LeadsTeam(42) = false, want true")
	}
	if p.LeadsTeam(2) {
		t.Errorf("p.LeadsTeam(2) = true, want false")
	}

	var nilP *auth.Principal
	if nilP.LeadsTeam(1) {
		t.Error("nil.LeadsTeam(1) = true, want false")
	}
}

func TestPrincipal_IsAdmin(t *testing.T) {
	admin := &auth.Principal{
		Roles: []string{"admin"},
	}
	if !admin.IsAdmin() {
		t.Error("admin.IsAdmin() = false, want true")
	}

	member := &auth.Principal{
		Roles: []string{"member"},
	}
	if member.IsAdmin() {
		t.Error("member.IsAdmin() = true, want false")
	}

	lead := &auth.Principal{
		Roles: []string{"lead"},
	}
	if lead.IsAdmin() {
		t.Error("lead.IsAdmin() = true, want false")
	}

	var nilP *auth.Principal
	if nilP.IsAdmin() {
		t.Error("nil.IsAdmin() = true, want false")
	}
}

func TestContextInjectionAndExtraction(t *testing.T) {
	t.Run("successful extraction", func(t *testing.T) {
		p := &auth.Principal{
			WorkspaceID: 1,
			UserID:      10,
			Email:       "alice@example.com",
			Roles:       []string{"lead"},
			Scopes:      []string{"tstamps.read", "tstamps.write"},
			ClientID:    "cli-client",
			TokenID:     "tok-123",
			Source:      "cli",
			LedTeamIDs:  []uint{3},
		}

		ctx := auth.WithPrincipal(context.Background(), p)
		got, ok := auth.PrincipalFrom(ctx)
		if !ok {
			t.Fatal("auth.PrincipalFrom(ctx) ok = false, want true")
		}
		if got != p {
			t.Errorf("auth.PrincipalFrom(ctx) = %v, want %v", got, p)
		}
		if got.UserID != 10 || got.Email != "alice@example.com" {
			t.Errorf("extracted principal fields mismatch: %+v", got)
		}
	})

	t.Run("empty context", func(t *testing.T) {
		got, ok := auth.PrincipalFrom(context.Background())
		if ok || got != nil {
			t.Errorf("auth.PrincipalFrom(context.Background()) = (%v, %v), want (nil, false)", got, ok)
		}
	})

	t.Run("nil principal in context", func(t *testing.T) {
		ctx := auth.WithPrincipal(context.Background(), nil)
		got, ok := auth.PrincipalFrom(ctx)
		if ok || got != nil {
			t.Errorf("auth.PrincipalFrom(ctx with nil) = (%v, %v), want (nil, false)", got, ok)
		}
	})

	t.Run("nil context", func(t *testing.T) {
		got, ok := auth.PrincipalFrom(nil)
		if ok || got != nil {
			t.Errorf("auth.PrincipalFrom(nil) = (%v, %v), want (nil, false)", got, ok)
		}
	})
}
