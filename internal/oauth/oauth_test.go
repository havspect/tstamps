package oauth

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/ory/fosite"
)

type memoryOAuthStore struct {
	clients       map[string]*Client
	authCodes     map[string]fosite.Requester
	accessTokens  map[string]fosite.Requester
	refreshTokens map[string]fosite.Requester
	pkceCodes     map[string]fosite.Requester
	deviceCodes   map[string]*DeviceCode
	grants        map[string][]Grant
}

func newMemoryOAuthStore() *memoryOAuthStore {
	return &memoryOAuthStore{
		clients:       make(map[string]*Client),
		authCodes:     make(map[string]fosite.Requester),
		accessTokens:  make(map[string]fosite.Requester),
		refreshTokens: make(map[string]fosite.Requester),
		pkceCodes:     make(map[string]fosite.Requester),
		deviceCodes:   make(map[string]*DeviceCode),
		grants:        make(map[string][]Grant),
	}
}

func (m *memoryOAuthStore) BeginTX(ctx context.Context) (context.Context, error) { return ctx, nil }
func (m *memoryOAuthStore) BeginTx(ctx context.Context) (context.Context, error) { return ctx, nil }
func (m *memoryOAuthStore) Commit(ctx context.Context) error                     { return nil }
func (m *memoryOAuthStore) Rollback(ctx context.Context) error                   { return nil }

func (m *memoryOAuthStore) GetClient(_ context.Context, id string) (fosite.Client, error) {
	if id == BuiltInCLIClientID {
		return &fosite.DefaultClient{
			ID:            BuiltInCLIClientID,
			RedirectURIs:  []string{"http://127.0.0.1/callback"},
			GrantTypes:    []string{"authorization_code", "refresh_token"},
			ResponseTypes: []string{"code"},
			Scopes:        []string{"tstamps.read", "tstamps.write"},
			Public:        true,
		}, nil
	}
	c, ok := m.clients[id]
	if !ok {
		return nil, ErrNotFound
	}
	return &fosite.DefaultClient{
		ID:            c.ClientID,
		RedirectURIs:  c.RedirectURIs,
		GrantTypes:    c.GrantTypes,
		ResponseTypes: c.ResponseTypes,
		Scopes:        c.Scopes,
		Public:        c.Public,
	}, nil
}

func (m *memoryOAuthStore) ClientAssertionJWTValid(_ context.Context, _ string) error { return nil }
func (m *memoryOAuthStore) SetClientAssertionJWT(_ context.Context, _ string, _ time.Time) error {
	return nil
}

func (m *memoryOAuthStore) CreateClient(_ context.Context, client *Client) error {
	m.clients[client.ClientID] = client
	return nil
}

func (m *memoryOAuthStore) GetClientByID(_ context.Context, clientID string) (*Client, error) {
	c, ok := m.clients[clientID]
	if !ok {
		return nil, ErrNotFound
	}
	return c, nil
}

func (m *memoryOAuthStore) CreateAuthorizeCodeSession(_ context.Context, code string, req fosite.Requester) error {
	m.authCodes[code] = req
	return nil
}

func (m *memoryOAuthStore) GetAuthorizeCodeSession(_ context.Context, code string, _ fosite.Session) (fosite.Requester, error) {
	req, ok := m.authCodes[code]
	if !ok {
		return nil, ErrNotFound
	}
	return req, nil
}

func (m *memoryOAuthStore) InvalidateAuthorizeCodeSession(_ context.Context, code string) error {
	delete(m.authCodes, code)
	return nil
}

func (m *memoryOAuthStore) CreatePKCERequestSession(ctx context.Context, code string, req fosite.Requester) error {
	m.pkceCodes[code] = req
	return nil
}

func (m *memoryOAuthStore) GetPKCERequestSession(ctx context.Context, code string, _ fosite.Session) (fosite.Requester, error) {
	req, ok := m.pkceCodes[code]
	if !ok {
		return nil, ErrNotFound
	}
	return req, nil
}

func (m *memoryOAuthStore) DeletePKCERequestSession(ctx context.Context, code string) error {
	delete(m.pkceCodes, code)
	return nil
}

func (m *memoryOAuthStore) CreateAccessTokenSession(_ context.Context, sig string, req fosite.Requester) error {
	m.accessTokens[sig] = req
	return nil
}

func (m *memoryOAuthStore) GetAccessTokenSession(_ context.Context, sig string, _ fosite.Session) (fosite.Requester, error) {
	req, ok := m.accessTokens[sig]
	if !ok {
		return nil, ErrNotFound
	}
	return req, nil
}

func (m *memoryOAuthStore) DeleteAccessTokenSession(_ context.Context, sig string) error {
	delete(m.accessTokens, sig)
	return nil
}

func (m *memoryOAuthStore) CreateRefreshTokenSession(_ context.Context, sig, _ string, req fosite.Requester) error {
	m.refreshTokens[sig] = req
	return nil
}

func (m *memoryOAuthStore) GetRefreshTokenSession(_ context.Context, sig string, _ fosite.Session) (fosite.Requester, error) {
	req, ok := m.refreshTokens[sig]
	if !ok {
		return nil, ErrNotFound
	}
	return req, nil
}

func (m *memoryOAuthStore) DeleteRefreshTokenSession(_ context.Context, sig string) error {
	delete(m.refreshTokens, sig)
	return nil
}

func (m *memoryOAuthStore) RotateRefreshToken(_ context.Context, _ string, sig string) error {
	delete(m.refreshTokens, sig)
	return nil
}

func (m *memoryOAuthStore) RevokeRefreshToken(_ context.Context, reqID string) error {
	for k, v := range m.refreshTokens {
		if v.GetID() == reqID {
			delete(m.refreshTokens, k)
		}
	}
	return nil
}

func (m *memoryOAuthStore) RevokeAccessToken(_ context.Context, reqID string) error {
	for k, v := range m.accessTokens {
		if v.GetID() == reqID {
			delete(m.accessTokens, k)
		}
	}
	return nil
}

func (m *memoryOAuthStore) CreateDeviceCode(_ context.Context, dc *DeviceCode) error {
	m.deviceCodes[dc.DeviceCodeHash] = dc
	return nil
}

func (m *memoryOAuthStore) GetDeviceCode(_ context.Context, hash string) (*DeviceCode, error) {
	dc, ok := m.deviceCodes[hash]
	if !ok {
		return nil, ErrNotFound
	}
	return dc, nil
}

func (m *memoryOAuthStore) ApproveDeviceCode(_ context.Context, userCodeHash string, userID uint, status int) error {
	for _, dc := range m.deviceCodes {
		if dc.UserCodeHash == userCodeHash {
			dc.UserID = userID
			dc.Status = status
			return nil
		}
	}
	return ErrNotFound
}

func (m *memoryOAuthStore) ConsumeDeviceCode(_ context.Context, id uint) error {
	for _, dc := range m.deviceCodes {
		if dc.ID == id {
			dc.Status = DeviceStatusConsumed
			return nil
		}
	}
	return nil
}

func (m *memoryOAuthStore) GetGrants(_ context.Context, _ uint) ([]Grant, error) {
	return []Grant{}, nil
}

func (m *memoryOAuthStore) RevokeGrant(_ context.Context, _ uint, _ string, _ time.Time) error {
	return nil
}

func TestOAuthSession(t *testing.T) {
	sess := NewSession(42, "alice@example.com", "42")
	if sess.UserID != 42 {
		t.Errorf("expected UserID=42, got %d", sess.UserID)
	}
	if sess.GetUsername() != "alice@example.com" {
		t.Errorf("expected username alice@example.com, got %s", sess.GetUsername())
	}
	sess.SetUsername("bob@example.com")
	if sess.GetUsername() != "bob@example.com" {
		t.Errorf("expected username bob@example.com, got %s", sess.GetUsername())
	}
	if sess.GetSubject() != "42" {
		t.Errorf("expected subject 42, got %s", sess.GetSubject())
	}

	now := time.Now()
	sess.SetExpiresAt(AccessToken, now)
	if !sess.GetExpiresAt(AccessToken).Equal(now) {
		t.Errorf("expected expiration %v, got %v", now, sess.GetExpiresAt(AccessToken))
	}

	cloned := sess.Clone()
	if cloned == nil {
		t.Fatal("expected non-nil clone")
	}
	if clonedSess, ok := cloned.(*OAuthSession); !ok || clonedSess.UserID != 42 {
		t.Errorf("clone failed or mismatch")
	}
}

func TestIsValidRedirectURI(t *testing.T) {
	valid := []string{
		"http://127.0.0.1/callback",
		"http://127.0.0.1:8080/cb",
		"http://localhost/cb",
		"http://localhost:3000/cb",
		"http://[::1]/cb",
		"https://myapp.example.com/oauth/callback",
	}
	for _, uri := range valid {
		if !IsValidRedirectURI(uri) {
			t.Errorf("expected valid redirect uri: %s", uri)
		}
	}

	invalid := []string{
		"",
		"not-a-url",
		"http://evil.com/callback", // http non-loopback
		"ftp://localhost/cb",
	}
	for _, uri := range invalid {
		if IsValidRedirectURI(uri) {
			t.Errorf("expected invalid redirect uri: %s", uri)
		}
	}
}

func TestProvider_RegisterClient(t *testing.T) {
	store := newMemoryOAuthStore()
	secret := []byte("12345678901234567890123456789012")
	p := NewProvider(store, secret)

	ctx := context.Background()

	// 1. Valid registration
	resp, err := p.RegisterClient(ctx, ClientRegistrationRequest{
		ClientName:   "My CLI App",
		RedirectURIs: []string{"http://127.0.0.1/callback"},
	})
	if err != nil {
		t.Fatalf("RegisterClient failed: %v", err)
	}
	if !strings.HasPrefix(resp.ClientID, "cli_") {
		t.Errorf("expected prefix cli_, got %s", resp.ClientID)
	}
	if resp.ClientName != "My CLI App" {
		t.Errorf("expected My CLI App, got %s", resp.ClientName)
	}

	// 2. Missing redirect URIs
	_, err = p.RegisterClient(ctx, ClientRegistrationRequest{
		ClientName: "Bad App",
	})
	if err == nil {
		t.Fatalf("expected error for missing redirect_uris")
	}

	// 3. Invalid redirect URI (non-loopback http)
	_, err = p.RegisterClient(ctx, ClientRegistrationRequest{
		ClientName:   "Insecure App",
		RedirectURIs: []string{"http://insecure.com/callback"},
	})
	if err == nil {
		t.Fatalf("expected error for non-loopback http redirect_uri")
	}
}

func TestProvider_TokenStrategyAndIntrospect(t *testing.T) {
	store := newMemoryOAuthStore()
	secret := []byte("12345678901234567890123456789012")
	p := NewProvider(store, secret)

	ctx := context.Background()
	req := NewRequest()
	req.ID = "req-1"
	req.Client = &fosite.DefaultClient{ID: "client-1"}
	req.Session = NewSession(10, "alice@example.com", "10")
	req.RequestedScope = Arguments{"tstamps.read"}
	req.GrantedScope = Arguments{"tstamps.read"}

	// Generate access token
	at, atSig, err := p.GenerateAccessToken(ctx, req)
	if err != nil {
		t.Fatalf("GenerateAccessToken: %v", err)
	}
	if at == "" || atSig == "" {
		t.Fatalf("expected non-empty at and sig")
	}

	// Generate refresh token
	rt, rtSig, err := p.GenerateRefreshToken(ctx, req)
	if err != nil {
		t.Fatalf("GenerateRefreshToken: %v", err)
	}
	if rt == "" || rtSig == "" {
		t.Fatalf("expected non-empty rt and sig")
	}

	// Store access token
	if err := store.CreateAccessTokenSession(ctx, atSig, req); err != nil {
		t.Fatalf("CreateAccessTokenSession: %v", err)
	}

	// Introspect token
	sess := NewSession(0, "", "")
	tt, accessReq, err := p.IntrospectToken(ctx, at, AccessToken, sess, "tstamps.read")
	if err != nil {
		t.Fatalf("IntrospectToken: %v", err)
	}
	if tt != AccessToken {
		t.Errorf("expected token type AccessToken, got %s", tt)
	}
	if accessReq.GetID() != "req-1" {
		t.Errorf("expected req-1, got %s", accessReq.GetID())
	}
}

func TestProvider_AuthorizeAndRevocationRequests(t *testing.T) {
	store := newMemoryOAuthStore()
	secret := []byte("12345678901234567890123456789012")
	p := NewProvider(store, secret)

	ctx := context.Background()

	// Authorize request with PKCE and state
	u, _ := url.Parse("http://localhost/oauth/authorize?response_type=code&client_id=tstamps-cli&redirect_uri=http%3A%2F%2F127.0.0.1%2Fcallback&code_challenge=E9Melhoa2OwvFrGMTJguCH5rtx64ZWqiJ6-gznkgKc8&code_challenge_method=S256&state=randomstate123")
	httpReq := &http.Request{
		Method: http.MethodGet,
		URL:    u,
		Form:   u.Query(),
	}

	ar, err := p.NewAuthorizeRequest(ctx, httpReq)
	if err != nil {
		t.Fatalf("NewAuthorizeRequest: %v", err)
	}
	if ar.GetClient().GetID() != BuiltInCLIClientID {
		t.Errorf("expected client %s, got %s", BuiltInCLIClientID, ar.GetClient().GetID())
	}

	// Revocation request
	revU, _ := url.Parse("http://localhost/oauth/revoke")
	revReq := &http.Request{
		Method:   http.MethodPost,
		URL:      revU,
		PostForm: url.Values{"token": {"dummy_token"}},
	}
	// Revocation for non-existent token is allowed per RFC 7009
	_ = p.NewRevocationRequest(ctx, revReq)
}
