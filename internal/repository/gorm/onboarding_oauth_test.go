package gorm

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/havspect/tstamps/internal/oauth"
	"github.com/havspect/tstamps/internal/onboarding"
)

func TestOnboardingRepository(t *testing.T) {
	db := newTestDB(t)
	repo := NewOnboardingRepository(db)
	ctx := context.Background()

	// 1. CreateWorkspace and GetWorkspaceBySlug
	ws, err := repo.CreateWorkspace(ctx, "acme")
	if err != nil {
		t.Fatalf("CreateWorkspace failed: %v", err)
	}
	if ws.Slug != "acme" || ws.ID == 0 {
		t.Fatalf("unexpected workspace: %+v", ws)
	}

	fetched, err := repo.GetWorkspaceBySlug(ctx, "acme")
	if err != nil {
		t.Fatalf("GetWorkspaceBySlug failed: %v", err)
	}
	if fetched.ID != ws.ID || fetched.Slug != "acme" {
		t.Fatalf("fetched mismatch: %+v", fetched)
	}

	// Unknown workspace
	_, err = repo.GetWorkspaceBySlug(ctx, "nonexistent")
	if !errors.Is(err, onboarding.ErrNotFound) {
		t.Fatalf("expected ErrNotFound for nonexistent workspace, got %v", err)
	}

	// Duplicate workspace slug
	_, err = repo.CreateWorkspace(ctx, "acme")
	if !errors.Is(err, onboarding.ErrSlugTaken) {
		t.Fatalf("expected ErrSlugTaken for duplicate slug, got %v", err)
	}

	// 2. ListWorkspaces
	_, err = repo.CreateWorkspace(ctx, "beta")
	if err != nil {
		t.Fatalf("CreateWorkspace beta failed: %v", err)
	}

	workspaces, err := repo.ListWorkspaces(ctx)
	if err != nil {
		t.Fatalf("ListWorkspaces failed: %v", err)
	}
	if len(workspaces) != 2 {
		t.Fatalf("expected 2 workspaces, got %d", len(workspaces))
	}
	if workspaces[0].Slug != "acme" || workspaces[1].Slug != "beta" {
		t.Fatalf("unexpected workspaces order/content: %+v", workspaces)
	}

	// 3. PendingSignups
	tokenHash := "hash123456789"
	err = repo.CreatePendingSignup(ctx, "user@example.com", tokenHash)
	if err != nil {
		t.Fatalf("CreatePendingSignup failed: %v", err)
	}

	pending, err := repo.GetPendingSignup(ctx, tokenHash)
	if err != nil {
		t.Fatalf("GetPendingSignup failed: %v", err)
	}
	if pending.Email != "user@example.com" || pending.TokenHash != tokenHash {
		t.Fatalf("unexpected pending signup: %+v", pending)
	}

	// Non-existent pending signup
	_, err = repo.GetPendingSignup(ctx, "unknown-hash")
	if !errors.Is(err, onboarding.ErrNotFound) {
		t.Fatalf("expected ErrNotFound for unknown token hash, got %v", err)
	}

	// Delete pending signup
	if err := repo.DeletePendingSignup(ctx, tokenHash); err != nil {
		t.Fatalf("DeletePendingSignup failed: %v", err)
	}
	_, err = repo.GetPendingSignup(ctx, tokenHash)
	if !errors.Is(err, onboarding.ErrNotFound) {
		t.Fatalf("expected ErrNotFound after deletion, got %v", err)
	}

	// 4. CreateInitialAdmin
	adminID, err := repo.CreateInitialAdmin(ctx, ws.ID, "admin@acme.com", "Admin Alice", "phash123", "America/New_York")
	if err != nil {
		t.Fatalf("CreateInitialAdmin failed: %v", err)
	}
	if adminID == 0 {
		t.Fatalf("expected positive admin ID")
	}

	// Query created admin via DB scoped to workspace
	var u User
	if err := db.WithContext(WithWorkspaceID(ctx, ws.ID)).First(&u, adminID).Error; err != nil {
		t.Fatalf("failed to query admin user: %v", err)
	}
	if u.Email != "admin@acme.com" || u.Name != "Admin Alice" || !u.IsAdmin || u.Timezone != "America/New_York" {
		t.Fatalf("unexpected admin user data: %+v", u)
	}
}

func TestOAuthStoreLifecycleAndTenancy(t *testing.T) {
	db := newTestDB(t)

	// Set up two workspaces
	onb := NewOnboardingRepository(db)
	ctx := context.Background()

	ws1, err := onb.CreateWorkspace(ctx, "ws1")
	if err != nil {
		t.Fatalf("CreateWorkspace ws1: %v", err)
	}
	ws2, err := onb.CreateWorkspace(ctx, "ws2")
	if err != nil {
		t.Fatalf("CreateWorkspace ws2: %v", err)
	}

	store1 := NewOAuthStore(db, ws1.ID)
	store2 := NewOAuthStore(db, ws2.ID)

	// 1. Built-in clients
	cliClient, err := store1.GetClient(ctx, oauth.BuiltInCLIClientID)
	if err != nil {
		t.Fatalf("GetClient cli: %v", err)
	}
	if !cliClient.IsPublic() || cliClient.GetID() != oauth.BuiltInCLIClientID {
		t.Fatalf("unexpected cli client: %+v", cliClient)
	}

	mobileClient, err := store1.GetClient(ctx, oauth.BuiltInMobileClientID)
	if err != nil {
		t.Fatalf("GetClient mobile: %v", err)
	}
	if !mobileClient.IsPublic() || mobileClient.GetID() != oauth.BuiltInMobileClientID {
		t.Fatalf("unexpected mobile client: %+v", mobileClient)
	}

	// 2. Custom client registration in ws1
	custom := &oauth.Client{
		ClientID:      "custom-client-1",
		ClientName:    "Custom Client",
		RedirectURIs:  []string{"http://127.0.0.1/cb"},
		GrantTypes:    []string{"authorization_code"},
		ResponseTypes: []string{"code"},
		Scopes:        []string{"tstamps.read"},
		Public:        false,
	}
	if err := store1.CreateClient(ctx, custom); err != nil {
		t.Fatalf("CreateClient failed: %v", err)
	}

	c1, err := store1.GetClient(ctx, "custom-client-1")
	if err != nil {
		t.Fatalf("GetClient custom in ws1: %v", err)
	}
	if c1.GetID() != "custom-client-1" {
		t.Fatalf("expected custom-client-1, got %s", c1.GetID())
	}

	cDirect, err := store1.GetClientByID(ctx, "custom-client-1")
	if err != nil {
		t.Fatalf("GetClientByID custom in ws1: %v", err)
	}
	if cDirect.ClientName != "Custom Client" {
		t.Fatalf("expected Custom Client, got %s", cDirect.ClientName)
	}

	// Client in ws1 MUST NOT be found in ws2
	_, err = store2.GetClient(ctx, "custom-client-1")
	if !errors.Is(err, oauth.ErrNotFound) {
		t.Fatalf("expected ErrNotFound for custom client in ws2, got %v", err)
	}
	_, err = store2.GetClientByID(ctx, "custom-client-1")
	if !errors.Is(err, oauth.ErrNotFound) {
		t.Fatalf("expected ErrNotFound for GetClientByID in ws2, got %v", err)
	}

	// 3. Authorize code sessions and replay detection
	sess1 := oauth.NewSession(1, "alice@example.com", "1")
	req := oauth.NewRequest()
	req.ID = "req-auth-1"
	req.Client = cliClient
	req.Session = sess1

	codeSig := "auth-code-sig-alpha"
	if err := store1.CreateAuthorizeCodeSession(ctx, codeSig, req); err != nil {
		t.Fatalf("CreateAuthorizeCodeSession failed: %v", err)
	}

	fetchedReq, err := store1.GetAuthorizeCodeSession(ctx, codeSig, sess1)
	if err != nil {
		t.Fatalf("GetAuthorizeCodeSession failed: %v", err)
	}
	if fetchedReq.GetID() != "req-auth-1" {
		t.Fatalf("expected req-auth-1, got %s", fetchedReq.GetID())
	}

	// Auth code in ws1 MUST NOT be visible in ws2
	_, err = store2.GetAuthorizeCodeSession(ctx, codeSig, sess1)
	if !errors.Is(err, oauth.ErrNotFound) {
		t.Fatalf("expected ErrNotFound for auth code in ws2, got %v", err)
	}

	// Invalidate and verify replay error
	if err := store1.InvalidateAuthorizeCodeSession(ctx, codeSig); err != nil {
		t.Fatalf("InvalidateAuthorizeCodeSession failed: %v", err)
	}
	replayedReq, err := store1.GetAuthorizeCodeSession(ctx, codeSig, sess1)
	if !errors.Is(err, oauth.ErrInvalidatedAuthorizeCode) {
		t.Fatalf("expected ErrInvalidatedAuthorizeCode on replay, got %v", err)
	}
	if replayedReq == nil {
		t.Fatalf("expected non-nil requester alongside ErrInvalidatedAuthorizeCode")
	}

	// 4. PKCE Request session
	pkceSig := "pkce-sig-alpha"
	if err := store1.CreatePKCERequestSession(ctx, pkceSig, req); err != nil {
		t.Fatalf("CreatePKCERequestSession: %v", err)
	}
	if _, err := store1.GetPKCERequestSession(ctx, pkceSig, sess1); err != nil {
		t.Fatalf("GetPKCERequestSession: %v", err)
	}
	if err := store1.DeletePKCERequestSession(ctx, pkceSig); err != nil {
		t.Fatalf("DeletePKCERequestSession: %v", err)
	}
	if _, err := store1.GetPKCERequestSession(ctx, pkceSig, sess1); !errors.Is(err, oauth.ErrNotFound) {
		t.Fatalf("expected ErrNotFound after deleting PKCE session, got %v", err)
	}

	// 5. Access Token session
	atSig := "access-token-sig-alpha"
	req.RequestedScope = oauth.Arguments{"tstamps.read"}
	req.GrantedScope = oauth.Arguments{"tstamps.read"}
	if err := store1.CreateAccessTokenSession(ctx, atSig, req); err != nil {
		t.Fatalf("CreateAccessTokenSession failed: %v", err)
	}

	fetchedAT, err := store1.GetAccessTokenSession(ctx, atSig, sess1)
	if err != nil {
		t.Fatalf("GetAccessTokenSession failed: %v", err)
	}
	if fetchedAT.GetID() != "req-auth-1" {
		t.Fatalf("expected req-auth-1, got %s", fetchedAT.GetID())
	}

	// Access token in ws1 invisible to ws2
	if _, err := store2.GetAccessTokenSession(ctx, atSig, sess1); !errors.Is(err, oauth.ErrNotFound) {
		t.Fatalf("expected ErrNotFound for ws1 access token in ws2, got %v", err)
	}

	// Touch access token
	storedAT, err := store1.OAuthAccessTokenBySignatureHash(ctx, atSig)
	if err != nil {
		t.Fatalf("OAuthAccessTokenBySignatureHash: %v", err)
	}
	touchTime := time.Now()
	if err := store1.TouchOAuthAccessToken(ctx, storedAT.ID, touchTime); err != nil {
		t.Fatalf("TouchOAuthAccessToken failed: %v", err)
	}

	// Soft delete / revoke access token
	if err := store1.DeleteAccessTokenSession(ctx, atSig); err != nil {
		t.Fatalf("DeleteAccessTokenSession failed: %v", err)
	}
	if _, err := store1.GetAccessTokenSession(ctx, atSig, sess1); !errors.Is(err, oauth.ErrInactiveToken) {
		t.Fatalf("expected ErrInactiveToken after deletion, got %v", err)
	}

	// 6. Refresh Token rotation
	rtSig1 := "refresh-sig-1"
	rtSig2 := "refresh-sig-2"
	if err := store1.CreateRefreshTokenSession(ctx, rtSig1, atSig, req); err != nil {
		t.Fatalf("CreateRefreshTokenSession 1 failed: %v", err)
	}
	if err := store1.CreateRefreshTokenSession(ctx, rtSig2, atSig, req); err != nil {
		t.Fatalf("CreateRefreshTokenSession 2 failed: %v", err)
	}

	// Rotate rtSig1
	if err := store1.RotateRefreshToken(ctx, "req-auth-1", rtSig1); err != nil {
		t.Fatalf("RotateRefreshToken failed: %v", err)
	}
	// Getting rotated token must return ErrInactiveToken
	if _, err := store1.GetRefreshTokenSession(ctx, rtSig1, sess1); !errors.Is(err, oauth.ErrInactiveToken) {
		t.Fatalf("expected ErrInactiveToken for rotated refresh token, got %v", err)
	}
	// rtSig2 should still be active
	if _, err := store1.GetRefreshTokenSession(ctx, rtSig2, sess1); err != nil {
		t.Fatalf("expected active rtSig2, got %v", err)
	}

	// Revoke refresh token
	if err := store1.RevokeRefreshToken(ctx, "req-auth-1"); err != nil {
		t.Fatalf("RevokeRefreshToken failed: %v", err)
	}
	if _, err := store1.GetRefreshTokenSession(ctx, rtSig2, sess1); !errors.Is(err, oauth.ErrInactiveToken) {
		t.Fatalf("expected ErrInactiveToken after RevokeRefreshToken, got %v", err)
	}

	// 7. Device authorization code flow
	dc := &oauth.DeviceCode{
		DeviceCodeHash: "dev-code-hash-1",
		UserCodeHash:   "user-code-hash-1",
		RequestID:      "req-dev-1",
		ClientID:       oauth.BuiltInCLIClientID,
		Status:         oauth.DeviceStatusPending,
		Scopes:         []string{"tstamps.read"},
		SessionData:    "{}",
		ExpiresAt:      time.Now().Add(10 * time.Minute),
	}
	if err := store1.CreateDeviceCode(ctx, dc); err != nil {
		t.Fatalf("CreateDeviceCode failed: %v", err)
	}
	if dc.ID == 0 {
		t.Fatalf("expected assigned ID for device code")
	}

	fetchedDC, err := store1.GetDeviceCode(ctx, "dev-code-hash-1")
	if err != nil {
		t.Fatalf("GetDeviceCode failed: %v", err)
	}
	if fetchedDC.Status != oauth.DeviceStatusPending {
		t.Fatalf("expected status pending, got %d", fetchedDC.Status)
	}

	// Approve device code
	if err := store1.ApproveDeviceCode(ctx, "user-code-hash-1", 1, oauth.DeviceStatusApproved); err != nil {
		t.Fatalf("ApproveDeviceCode failed: %v", err)
	}
	approvedDC, err := store1.GetDeviceCode(ctx, "dev-code-hash-1")
	if err != nil {
		t.Fatalf("GetDeviceCode after approval failed: %v", err)
	}
	if approvedDC.Status != oauth.DeviceStatusApproved || approvedDC.UserID != 1 {
		t.Fatalf("unexpected approved device code: %+v", approvedDC)
	}

	// Consume device code
	if err := store1.ConsumeDeviceCode(ctx, dc.ID); err != nil {
		t.Fatalf("ConsumeDeviceCode failed: %v", err)
	}
	consumedDC, err := store1.GetDeviceCode(ctx, "dev-code-hash-1")
	if err != nil {
		t.Fatalf("GetDeviceCode after consume failed: %v", err)
	}
	if consumedDC.Status != oauth.DeviceStatusConsumed {
		t.Fatalf("expected status consumed, got %d", consumedDC.Status)
	}

	// 8. Grants query and revocation
	// Create another active access token for user 1
	atSigGrant := "at-sig-grant-1"
	reqGrant := oauth.NewRequest()
	reqGrant.ID = "req-grant-1"
	reqGrant.Client = cliClient
	reqGrant.Session = sess1
	reqGrant.GrantedScope = oauth.Arguments{"tstamps.read", "tstamps.write"}
	if err := store1.CreateAccessTokenSession(ctx, atSigGrant, reqGrant); err != nil {
		t.Fatalf("CreateAccessTokenSession for grant: %v", err)
	}

	grants, err := store1.GetGrants(ctx, 1)
	if err != nil {
		t.Fatalf("GetGrants failed: %v", err)
	}
	if len(grants) == 0 {
		t.Fatalf("expected at least 1 grant for user 1")
	}

	if err := store1.RevokeGrant(ctx, 1, "req-grant-1", time.Now()); err != nil {
		t.Fatalf("RevokeGrant failed: %v", err)
	}

	// Non-existent grant revocation returns ErrNotFound
	if err := store1.RevokeGrant(ctx, 1, "nonexistent-req", time.Now()); !errors.Is(err, oauth.ErrNotFound) {
		t.Fatalf("expected ErrNotFound for nonexistent grant, got %v", err)
	}

	// 9. Transactional interface
	txCtx, err := store1.BeginTx(ctx)
	if err != nil {
		t.Fatalf("BeginTx failed: %v", err)
	}
	txStore := store1.ForWorkspace(ws1.ID)
	// Create a client within transaction
	txClient := &oauth.Client{
		ClientID:     "tx-client",
		ClientName:   "TX Client",
		RedirectURIs: []string{"http://127.0.0.1/cb"},
	}
	if err := txStore.CreateClient(txCtx, txClient); err != nil {
		t.Fatalf("CreateClient in tx failed: %v", err)
	}
	if err := store1.Rollback(txCtx); err != nil {
		t.Fatalf("Rollback failed: %v", err)
	}
	// After rollback, tx-client should not exist
	if _, err := store1.GetClientByID(ctx, "tx-client"); !errors.Is(err, oauth.ErrNotFound) {
		t.Fatalf("expected ErrNotFound after rollback, got %v", err)
	}
}
