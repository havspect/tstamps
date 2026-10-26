package store

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/ory/fosite"
)

func TestOAuthStoreRoundTrips(t *testing.T) {
	reg, err := OpenRegistry(":memory:")
	if err != nil {
		t.Fatalf("OpenRegistry: %v", err)
	}
	defer reg.Close()

	if err := reg.Migrate(); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	// Test server secret generation
	secret1, err := reg.OAuthSecret()
	if err != nil {
		t.Fatalf("OAuthSecret: %v", err)
	}
	if len(secret1) != 32 {
		t.Fatalf("expected 32-byte secret, got %d", len(secret1))
	}

	ws1 := reg.ForWorkspace(1)
	if _, err := ws1.OAuthSecret(); err != nil {
		t.Fatalf("Store.OAuthSecret: %v", err)
	}

	// Create users in workspace 1 and workspace 2
	u1 := &User{Email: "user1@example.com", Name: "User 1", PasswordHash: "hash"}
	if err := ws1.CreateUser(u1); err != nil {
		t.Fatalf("CreateUser ws1: %v", err)
	}

	// Create workspace 2
	w2 := &Workspace{ID: 2, Slug: "ws2"}
	if err := reg.CreateWorkspace(w2); err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}
	ws2 := reg.ForWorkspace(2)
	u2 := &User{Email: "user2@example.com", Name: "User 2", PasswordHash: "hash"}
	if err := ws2.CreateUser(u2); err != nil {
		t.Fatalf("CreateUser ws2: %v", err)
	}

	fositeStore1 := ws1.Fosite()
	ctx := context.Background()

	// 1. Client registration & built-in clients
	cliClient, err := fositeStore1.GetClient(ctx, BuiltInCLIClientID)
	if err != nil {
		t.Fatalf("GetClient cli: %v", err)
	}
	if !cliClient.IsPublic() {
		t.Fatalf("expected cli client to be public")
	}

	mobileClient, err := fositeStore1.GetClient(ctx, BuiltInMobileClientID)
	if err != nil {
		t.Fatalf("GetClient mobile: %v", err)
	}
	if !mobileClient.IsPublic() {
		t.Fatalf("expected mobile client to be public")
	}

	customClient := &OAuthClient{
		ClientID:      "custom-client",
		ClientName:    "Custom Client",
		RedirectURIs:  `["http://localhost/cb"]`,
		GrantTypes:    `["authorization_code"]`,
		ResponseTypes: `["code"]`,
		Scopes:        `["tstamps.read"]`,
		Public:        false,
	}
	if err := ws1.CreateOAuthClient(customClient); err != nil {
		t.Fatalf("CreateOAuthClient: %v", err)
	}
	fetchedCustom, err := fositeStore1.GetClient(ctx, "custom-client")
	if err != nil {
		t.Fatalf("GetClient custom: %v", err)
	}
	if fetchedCustom.GetID() != "custom-client" {
		t.Fatalf("expected custom-client")
	}

	// 2. Auth code + replay returning ErrInvalidatedAuthorizeCode
	codeSig := "auth-code-sig-1"
	req := fosite.NewRequest()
	req.ID = "req-1"
	req.Client = cliClient
	req.Session = &OAuthSession{UserID: u1.ID, Username: u1.Email, Subject: "1"}

	if err := fositeStore1.CreateAuthorizeCodeSession(ctx, codeSig, req); err != nil {
		t.Fatalf("CreateAuthorizeCodeSession: %v", err)
	}

	fetchedReq, err := fositeStore1.GetAuthorizeCodeSession(ctx, codeSig, req.Session)
	if err != nil {
		t.Fatalf("GetAuthorizeCodeSession: %v", err)
	}
	if fetchedReq.GetID() != "req-1" {
		t.Fatalf("expected req-1")
	}

	if err := fositeStore1.InvalidateAuthorizeCodeSession(ctx, codeSig); err != nil {
		t.Fatalf("InvalidateAuthorizeCodeSession: %v", err)
	}

	replayedReq, err := fositeStore1.GetAuthorizeCodeSession(ctx, codeSig, req.Session)
	if !errors.Is(err, fosite.ErrInvalidatedAuthorizeCode) {
		t.Fatalf("expected ErrInvalidatedAuthorizeCode on replayed auth code, got %v", err)
	}
	if replayedReq == nil {
		t.Fatalf("expected non-nil requester with ErrInvalidatedAuthorizeCode")
	}

	// 3. PKCE
	pkceSig := "pkce-sig-1"
	if err := fositeStore1.CreatePKCERequestSession(ctx, pkceSig, req); err != nil {
		t.Fatalf("CreatePKCERequestSession: %v", err)
	}
	if _, err := fositeStore1.GetPKCERequestSession(ctx, pkceSig, req.Session); err != nil {
		t.Fatalf("GetPKCERequestSession: %v", err)
	}
	if err := fositeStore1.DeletePKCERequestSession(ctx, pkceSig); err != nil {
		t.Fatalf("DeletePKCERequestSession: %v", err)
	}

	// 4. Access Token
	atSig := "access-token-sig-1"
	if err := fositeStore1.CreateAccessTokenSession(ctx, atSig, req); err != nil {
		t.Fatalf("CreateAccessTokenSession: %v", err)
	}
	fetchedAT, err := fositeStore1.GetAccessTokenSession(ctx, atSig, req.Session)
	if err != nil {
		t.Fatalf("GetAccessTokenSession: %v", err)
	}
	if fetchedAT.GetID() != "req-1" {
		t.Fatalf("expected access token request id")
	}

	// Test Touch and SignatureHash helper
	storedAT, err := ws1.OAuthAccessTokenBySignatureHash(atSig)
	if err != nil {
		t.Fatalf("OAuthAccessTokenBySignatureHash: %v", err)
	}
	now := time.Now()
	if err := ws1.TouchOAuthAccessToken(storedAT.ID, now); err != nil {
		t.Fatalf("TouchOAuthAccessToken: %v", err)
	}

	// 5. Refresh Token rotation + reuse returning ErrInactiveToken and revoking family
	rtSig := "refresh-token-sig-1"
	accessSig := "access-sig-for-rt"
	if err := fositeStore1.CreateRefreshTokenSession(ctx, rtSig, accessSig, req); err != nil {
		t.Fatalf("CreateRefreshTokenSession: %v", err)
	}

	rtSig2 := "refresh-token-sig-2"
	if err := fositeStore1.CreateRefreshTokenSession(ctx, rtSig2, "access-sig-2", req); err != nil {
		t.Fatalf("CreateRefreshTokenSession 2: %v", err)
	}

	// Rotate refresh token (revokes the used token rtSig)
	if err := fositeStore1.RotateRefreshToken(ctx, "req-1", rtSig); err != nil {
		t.Fatalf("RotateRefreshToken: %v", err)
	}

	// Getting rotated-out rtSig should return ErrInactiveToken
	_, err = fositeStore1.GetRefreshTokenSession(ctx, rtSig, req.Session)
	if !errors.Is(err, fosite.ErrInactiveToken) {
		t.Fatalf("expected ErrInactiveToken for rotated refresh token, got %v", err)
	}

	// 7. Workspace isolation (workspace 1 tokens/clients invisible to workspace 2)
	fositeStore2 := ws2.Fosite()
	_, err = fositeStore2.GetClient(ctx, "custom-client")
	if !errors.Is(err, fosite.ErrNotFound) {
		t.Fatalf("expected ErrNotFound for custom-client in workspace 2, got %v", err)
	}
	_, err = fositeStore2.GetAccessTokenSession(ctx, atSig, req.Session)
	if !errors.Is(err, fosite.ErrNotFound) {
		t.Fatalf("expected ErrNotFound for ws1 access token in workspace 2, got %v", err)
	}

	// 8. Grants of and Revoke OAuth Grant
	grants, err := ws1.OAuthGrantsOf(u1.ID)
	if err != nil {
		t.Fatalf("OAuthGrantsOf: %v", err)
	}
	if len(grants) == 0 {
		t.Fatalf("expected at least one OAuth grant")
	}

	if err := ws1.RevokeOAuthGrant(u1.ID, "req-1", time.Now()); err != nil {
		t.Fatalf("RevokeOAuthGrant: %v", err)
	}

	// 9. Password change revoking OAuth tokens
	if err := ws1.SetUserPassword(u1.ID, "new-hash", false, 0, time.Now()); err != nil {
		t.Fatalf("SetUserPassword: %v", err)
	}
	_, err = fositeStore1.GetAccessTokenSession(ctx, atSig, req.Session)
	if !errors.Is(err, fosite.ErrInactiveToken) {
		t.Fatalf("expected ErrInactiveToken after password change, got %v", err)
	}
}
