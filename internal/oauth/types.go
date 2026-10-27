package oauth

import (
	"time"

	"github.com/ory/fosite"
)

// Built-in OAuth clients
const (
	BuiltInCLIClientID    = "tstamps-cli"
	BuiltInMobileClientID = "tstamps-mobile"
)

// Device code statuses (RFC 8628)
const (
	DeviceStatusPending  = 0
	DeviceStatusApproved = 1
	DeviceStatusDenied   = 2
	DeviceStatusConsumed = 3
)

// Fosite TokenType constants
const (
	AuthorizeCode = fosite.AuthorizeCode
	AccessToken   = fosite.AccessToken
	RefreshToken  = fosite.RefreshToken
)

// Fosite type aliases to prevent Ory Fosite types from leaking outside internal/oauth
type (
	TokenType          = fosite.TokenType
	Session            = fosite.Session
	Requester          = fosite.Requester
	AccessRequester    = fosite.AccessRequester
	AuthorizeRequester = fosite.AuthorizeRequester
	AccessResponder    = fosite.AccessResponder
	AuthorizeResponder = fosite.AuthorizeResponder
	ClientManager      = fosite.ClientManager
	FositeClient       = fosite.Client
	DefaultClient      = fosite.DefaultClient
	Request            = fosite.Request
	Arguments          = fosite.Arguments
)

// Fosite common error aliases
var (
	ErrNotFound                 = fosite.ErrNotFound
	ErrInvalidatedAuthorizeCode = fosite.ErrInvalidatedAuthorizeCode
	ErrInactiveToken            = fosite.ErrInactiveToken
	ErrInvalidRequest           = fosite.ErrInvalidRequest
	ErrInvalidClient            = fosite.ErrInvalidClient
	ErrInvalidGrant             = fosite.ErrInvalidGrant
	ErrUnauthorizedClient       = fosite.ErrUnauthorizedClient
	ErrUnsupportedGrantType     = fosite.ErrUnsupportedGrantType
	ErrUnsupportedResponseType  = fosite.ErrUnsupportedResponseType
	ErrInvalidScope             = fosite.ErrInvalidScope
	ErrServerError              = fosite.ErrServerError
)

// ErrorToRFC6749Error converts an error into a Fosite RFC6749 error representation.
func ErrorToRFC6749Error(err error) *fosite.RFC6749Error {
	return fosite.ErrorToRFC6749Error(err)
}

// NewRequest returns a new Fosite request instance.
func NewRequest() *fosite.Request {
	return fosite.NewRequest()
}

// OAuthSession implements fosite.Session for user sessions.
type OAuthSession struct {
	UserID    uint                           `json:"user_id"`
	Username  string                         `json:"username"`
	Subject   string                         `json:"subject"`
	ExpiresAt map[fosite.TokenType]time.Time `json:"expires_at"`
	Extra     map[string]interface{}         `json:"extra"`
}

// NewSession creates an OAuthSession with standard default lifespans.
func NewSession(userID uint, username, subject string) *OAuthSession {
	now := time.Now()
	return &OAuthSession{
		UserID:   userID,
		Username: username,
		Subject:  subject,
		ExpiresAt: map[fosite.TokenType]time.Time{
			fosite.AuthorizeCode: now.Add(time.Minute),
			fosite.AccessToken:   now.Add(time.Hour),
			fosite.RefreshToken:  now.Add(30 * 24 * time.Hour),
		},
	}
}

// GetExpiresAt returns the expiration time for the given token type.
func (s *OAuthSession) GetExpiresAt(key fosite.TokenType) time.Time {
	if s.ExpiresAt == nil {
		return time.Time{}
	}
	return s.ExpiresAt[key]
}

// SetExpiresAt sets the expiration time for the given token type.
func (s *OAuthSession) SetExpiresAt(key fosite.TokenType, val time.Time) {
	if s.ExpiresAt == nil {
		s.ExpiresAt = make(map[fosite.TokenType]time.Time)
	}
	s.ExpiresAt[key] = val
}

// GetUsername returns the username.
func (s *OAuthSession) GetUsername() string {
	return s.Username
}

// SetUsername sets the username.
func (s *OAuthSession) SetUsername(username string) {
	s.Username = username
}

// GetSubject returns the subject.
func (s *OAuthSession) GetSubject() string {
	return s.Subject
}

// Clone creates a deep copy of the session.
func (s *OAuthSession) Clone() fosite.Session {
	if s == nil {
		return nil
	}
	exp := make(map[fosite.TokenType]time.Time, len(s.ExpiresAt))
	for k, v := range s.ExpiresAt {
		exp[k] = v
	}
	extra := make(map[string]interface{}, len(s.Extra))
	for k, v := range s.Extra {
		extra[k] = v
	}
	return &OAuthSession{
		UserID:    s.UserID,
		Username:  s.Username,
		Subject:   s.Subject,
		ExpiresAt: exp,
		Extra:     extra,
	}
}

// Client represents a domain OAuth client application.
type Client struct {
	ID               uint      `json:"id"`
	WorkspaceID      uint      `json:"workspace_id"`
	ClientID         string    `json:"client_id"`
	ClientName       string    `json:"client_name"`
	ClientSecretHash string    `json:"client_secret_hash,omitempty"`
	RedirectURIs     []string  `json:"redirect_uris"`
	GrantTypes       []string  `json:"grant_types"`
	ResponseTypes    []string  `json:"response_types"`
	Scopes           []string  `json:"scopes"`
	Public           bool      `json:"public"`
	CreatedAt        time.Time `json:"created_at"`
}

// DeviceCode represents a device authorization request row.
type DeviceCode struct {
	ID             uint       `json:"id"`
	WorkspaceID    uint       `json:"workspace_id"`
	DeviceCodeHash string     `json:"device_code_hash"`
	UserCodeHash   string     `json:"user_code_hash"`
	RequestID      string     `json:"request_id"`
	ClientID       string     `json:"client_id"`
	UserID         uint       `json:"user_id"`
	Status         int        `json:"status"`
	Scopes         []string   `json:"scopes"`
	SessionData    string     `json:"session_data"`
	ExpiresAt      time.Time  `json:"expires_at"`
	LastCheckedAt  *time.Time `json:"last_checked_at,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
}

// Grant represents an active or historic user-authorized OAuth grant.
type Grant struct {
	RequestID  string     `json:"id"`
	ClientID   string     `json:"client_id"`
	ClientName string     `json:"client_name"`
	Scopes     []string   `json:"scopes"`
	CreatedAt  time.Time  `json:"created_at"`
	LastUsedAt *time.Time `json:"last_used_at,omitempty"`
	RevokedAt  *time.Time `json:"revoked_at,omitempty"`
}

// ClientRegistrationRequest is the payload for registering dynamic OAuth clients.
type ClientRegistrationRequest struct {
	ClientName    string   `json:"client_name"`
	RedirectURIs  []string `json:"redirect_uris"`
	GrantTypes    []string `json:"grant_types"`
	ResponseTypes []string `json:"response_types"`
	Scope         string   `json:"scope,omitempty"`
}

// ClientRegistrationResponse describes the registered client response.
type ClientRegistrationResponse struct {
	ClientID                string   `json:"client_id"`
	ClientName              string   `json:"client_name"`
	RedirectURIs            []string `json:"redirect_uris"`
	GrantTypes              []string `json:"grant_types"`
	ResponseTypes           []string `json:"response_types"`
	TokenEndpointAuthMethod string   `json:"token_endpoint_auth_method"`
}
