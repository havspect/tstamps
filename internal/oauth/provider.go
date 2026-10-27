package oauth

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/ory/fosite"
	"github.com/ory/fosite/compose"
	"github.com/ory/fosite/handler/oauth2"
)

// Provider coordinates OAuth 2.1 authorization flows, token issuance, and introspection.
type Provider struct {
	store      Storage
	fosite     fosite.OAuth2Provider
	strategy   *oauth2.HMACSHAStrategy
	hmacSecret []byte
}

// NewProvider initializes Ory Fosite with PKCE, explicit code flow, refresh tokens, and introspection.
func NewProvider(store Storage, hmacSecret []byte) *Provider {
	if len(hmacSecret) < 32 {
		hmacSecret = []byte("tstamps-default-oauth-secret-key-32bytes")
	}

	config := &fosite.Config{
		GlobalSecret:                   hmacSecret,
		AccessTokenLifespan:            time.Hour,
		AuthorizeCodeLifespan:          time.Minute,
		RefreshTokenLifespan:           30 * 24 * time.Hour,
		EnforcePKCE:                    true,
		EnforcePKCEForPublicClients:    true,
		EnablePKCEPlainChallengeMethod: false,
		RefreshTokenScopes:             []string{"tstamps.read", "tstamps.write"},
	}

	hmacStrategy := compose.NewOAuth2HMACStrategy(config)
	fositeProvider := compose.Compose(
		config,
		store,
		&compose.CommonStrategy{
			CoreStrategy: hmacStrategy,
		},
		compose.OAuth2AuthorizeExplicitFactory,
		compose.OAuth2PKCEFactory,
		compose.OAuth2RefreshTokenGrantFactory,
		compose.OAuth2TokenIntrospectionFactory,
		compose.OAuth2TokenRevocationFactory,
	)

	return &Provider{
		store:      store,
		fosite:     fositeProvider,
		strategy:   hmacStrategy,
		hmacSecret: hmacSecret,
	}
}

// Store returns the underlying Storage implementation.
func (p *Provider) Store() Storage {
	return p.store
}

// Fosite returns the underlying fosite.OAuth2Provider instance.
func (p *Provider) Fosite() fosite.OAuth2Provider {
	return p.fosite
}

// Strategy returns the HMACSHAStrategy.
func (p *Provider) Strategy() *oauth2.HMACSHAStrategy {
	return p.strategy
}

// NewAuthorizeRequest parses and validates an incoming OAuth authorization request.
func (p *Provider) NewAuthorizeRequest(ctx context.Context, req *http.Request) (fosite.AuthorizeRequester, error) {
	return p.fosite.NewAuthorizeRequest(ctx, req)
}

// NewAuthorizeResponse creates a response for an approved authorization request.
func (p *Provider) NewAuthorizeResponse(ctx context.Context, ar fosite.AuthorizeRequester, session fosite.Session) (fosite.AuthorizeResponder, error) {
	return p.fosite.NewAuthorizeResponse(ctx, ar, session)
}

// WriteAuthorizeResponse writes the authorization response headers or redirect.
func (p *Provider) WriteAuthorizeResponse(ctx context.Context, rw http.ResponseWriter, ar fosite.AuthorizeRequester, resp fosite.AuthorizeResponder) {
	p.fosite.WriteAuthorizeResponse(ctx, rw, ar, resp)
}

// WriteAuthorizeError writes an authorization error to the response writer.
func (p *Provider) WriteAuthorizeError(ctx context.Context, rw http.ResponseWriter, ar fosite.AuthorizeRequester, err error) {
	p.fosite.WriteAuthorizeError(ctx, rw, ar, err)
}

// NewAccessRequest parses an incoming token exchange or refresh request.
func (p *Provider) NewAccessRequest(ctx context.Context, req *http.Request, session fosite.Session) (fosite.AccessRequester, error) {
	return p.fosite.NewAccessRequest(ctx, req, session)
}

// NewAccessResponse creates a response for an approved access token request.
func (p *Provider) NewAccessResponse(ctx context.Context, requester fosite.AccessRequester) (fosite.AccessResponder, error) {
	return p.fosite.NewAccessResponse(ctx, requester)
}

// WriteAccessResponse writes token response payload to the response writer.
func (p *Provider) WriteAccessResponse(ctx context.Context, rw http.ResponseWriter, requester fosite.AccessRequester, resp fosite.AccessResponder) {
	p.fosite.WriteAccessResponse(ctx, rw, requester, resp)
}

// WriteAccessError writes a token error to the response writer.
func (p *Provider) WriteAccessError(ctx context.Context, rw http.ResponseWriter, requester fosite.AccessRequester, err error) {
	p.fosite.WriteAccessError(ctx, rw, requester, err)
}

// NewRevocationRequest revokes an access or refresh token as specified in RFC 7009.
func (p *Provider) NewRevocationRequest(ctx context.Context, req *http.Request) error {
	return p.fosite.NewRevocationRequest(ctx, req)
}

// WriteRevocationResponse writes a revocation response.
func (p *Provider) WriteRevocationResponse(ctx context.Context, rw http.ResponseWriter, err error) {
	p.fosite.WriteRevocationResponse(ctx, rw, err)
}

// IntrospectToken checks token validity and claims as specified in RFC 7662.
func (p *Provider) IntrospectToken(ctx context.Context, token string, tokenType fosite.TokenType, session fosite.Session, scopes ...string) (fosite.TokenType, fosite.AccessRequester, error) {
	return p.fosite.IntrospectToken(ctx, token, tokenType, session, scopes...)
}

// RegisterClient validates and creates a new dynamic OAuth client.
func (p *Provider) RegisterClient(ctx context.Context, req ClientRegistrationRequest) (*ClientRegistrationResponse, error) {
	if len(req.RedirectURIs) == 0 {
		return nil, errors.New("redirect_uris required")
	}
	for _, uri := range req.RedirectURIs {
		if !IsValidRedirectURI(uri) {
			return nil, errors.New("invalid redirect uri: must be https or loopback")
		}
	}

	clientID := "cli_" + randomHex(16)
	grantTypes := req.GrantTypes
	if len(grantTypes) == 0 {
		grantTypes = []string{"authorization_code", "refresh_token"}
	}
	responseTypes := req.ResponseTypes
	if len(responseTypes) == 0 {
		responseTypes = []string{"code"}
	}
	clientName := req.ClientName
	if clientName == "" {
		clientName = clientID
	}
	scopes := []string{"tstamps.read", "tstamps.write"}
	if req.Scope != "" {
		scopes = strings.Fields(req.Scope)
	}

	client := &Client{
		ClientID:      clientID,
		ClientName:    clientName,
		RedirectURIs:  req.RedirectURIs,
		GrantTypes:    grantTypes,
		ResponseTypes: responseTypes,
		Scopes:        scopes,
		Public:        true,
		CreatedAt:     time.Now(),
	}

	if p.store != nil {
		if err := p.store.CreateClient(ctx, client); err != nil {
			return nil, err
		}
	}

	return &ClientRegistrationResponse{
		ClientID:                clientID,
		ClientName:              clientName,
		RedirectURIs:            req.RedirectURIs,
		GrantTypes:              grantTypes,
		ResponseTypes:           responseTypes,
		TokenEndpointAuthMethod: "none",
	}, nil
}

// GenerateAccessToken generates an access token and signature via the HMAC strategy.
func (p *Provider) GenerateAccessToken(ctx context.Context, requester fosite.Requester) (string, string, error) {
	return p.strategy.GenerateAccessToken(ctx, requester)
}

// GenerateRefreshToken generates a refresh token and signature via the HMAC strategy.
func (p *Provider) GenerateRefreshToken(ctx context.Context, requester fosite.Requester) (string, string, error) {
	return p.strategy.GenerateRefreshToken(ctx, requester)
}

// IsValidRedirectURI checks if a redirect URI is HTTPS or loopback HTTP.
func IsValidRedirectURI(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme == "" {
		return false
	}
	if u.Scheme == "https" {
		return u.Host != ""
	}
	if u.Scheme == "http" {
		host := u.Hostname()
		if host == "127.0.0.1" || host == "localhost" || host == "::1" || strings.HasPrefix(host, "127.") {
			return true
		}
	}
	return false
}

func randomHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
