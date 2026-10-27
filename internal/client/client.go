// Package client is the typed HTTP client the CLI and MCP server share.
package client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"time"
)

const apiPrefix = "/api/v1"

// Config says which server to talk to and, when set, which token to send.
type Config struct {
	URL          string `json:"url"`
	Token        string `json:"token,omitempty"`
	RefreshToken string `json:"refresh_token,omitempty"`
}

// Client talks to one Tstamps server. Login keeps its session cookie in the
// jar; a Config.Token is sent as a bearer header on every request.
type Client struct {
	cfg            Config
	http           *http.Client
	onTokenRefresh func(Config)
}

// New returns a client for cfg with a cookie jar, so Login can keep its session.
func New(cfg Config) *Client {
	jar, _ := cookiejar.New(nil)
	return &Client{cfg: cfg, http: &http.Client{Jar: jar, Timeout: 30 * time.Second}}
}

// SetOnTokenRefresh sets a callback to be notified when an OAuth token is refreshed.
func (c *Client) SetOnTokenRefresh(fn func(Config)) {
	c.onTokenRefresh = fn
}

// SetTransport sets the HTTP transport used by the client.
func (c *Client) SetTransport(rt http.RoundTripper) {
	c.http.Transport = rt
}

// Membership is a team role the authenticated user holds.
type Membership struct {
	TeamID uint   `json:"team_id"`
	Team   string `json:"team"`
	Role   string `json:"role"`
}

// Me is the authenticated user.
type Me struct {
	ID                 uint         `json:"id"`
	Email              string       `json:"email"`
	Name               string       `json:"name"`
	Timezone           string       `json:"timezone"`
	IsAdmin            bool         `json:"is_admin"`
	MustChangePassword bool         `json:"must_change_password"`
	Memberships        []Membership `json:"memberships"`
}

// Token is a personal API token; Token holds the raw value only in the reply to
// CreateToken.
type Token struct {
	ID         uint       `json:"id"`
	Name       string     `json:"name"`
	Token      string     `json:"token,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
	LastUsedAt *time.Time `json:"last_used_at"`
	RevokedAt  *time.Time `json:"revoked_at"`
}

// DeviceAuthResponse is returned by StartDeviceAuth.
type DeviceAuthResponse struct {
	DeviceCode              string `json:"device_code"`
	UserCode                string `json:"user_code"`
	VerificationURI         string `json:"verification_uri"`
	VerificationURIComplete string `json:"verification_uri_complete"`
	ExpiresIn               int    `json:"expires_in"`
	Interval                int    `json:"interval"`
}

// TokenSet holds access and refresh tokens returned by device token polling or token refresh.
type TokenSet struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
}
// Error is a non-2xx reply from the server. Type is the stable
// urn:tstamps:error:* code from the problem document.
type Error struct {
	StatusCode int
	Type       string
	Detail     string
	// RunningEntry is set on the 409 a second timer start returns.
	RunningEntry *Entry
}

func (e *Error) Error() string {
	if e.Detail == "" {
		return http.StatusText(e.StatusCode)
	}
	return http.StatusText(e.StatusCode) + ": " + e.Detail
}

// StartDeviceAuth initiates device authorization.
func (c *Client) StartDeviceAuth() (*DeviceAuthResponse, error) {
	var out DeviceAuthResponse
	data := url.Values{
		"client_id": {"tstamps-cli"},
		"scope":     {"tstamps.read tstamps.write"},
	}
	req, err := http.NewRequest(http.MethodPost, strings.TrimSuffix(c.cfg.URL, "/")+apiPrefix+"/auth/device", strings.NewReader(data.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, serverError(resp)
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	return &out, nil
}

// PollDeviceToken polls for the device token using RFC 8628 rules.
func (c *Client) PollDeviceToken(ctx context.Context, deviceCode string, interval time.Duration) (*TokenSet, error) {
	if interval <= 0 {
		interval = 5 * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	endpoint := strings.TrimSuffix(c.cfg.URL, "/") + apiPrefix + "/auth/device/token"

	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-ticker.C:
			data := url.Values{
				"grant_type":  {"urn:ietf:params:oauth:grant-type:device_code"},
				"device_code": {deviceCode},
				"client_id":   {"tstamps-cli"},
			}
			req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(data.Encode()))
			if err != nil {
				return nil, err
			}
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			resp, err := c.http.Do(req)
			if err != nil {
				return nil, err
			}

			bodyBytes, _ := io.ReadAll(resp.Body)
			resp.Body.Close()

			if resp.StatusCode >= 200 && resp.StatusCode <= 299 {
				var tok TokenSet
				if err := json.Unmarshal(bodyBytes, &tok); err != nil {
					return nil, err
				}
				return &tok, nil
			}

			var errBody struct {
				Error string `json:"error"`
			}
			_ = json.Unmarshal(bodyBytes, &errBody)
			oauthErr := errBody.Error
			if oauthErr == "" {
				oauthErr = string(bodyBytes)
			}

			switch oauthErr {
			case "authorization_pending":
				// continue polling
			case "slow_down":
				interval += 5 * time.Second
				ticker.Reset(interval)
			case "access_denied":
				return nil, errors.New("authorization access denied")
			case "expired_token":
				return nil, errors.New("device code expired")
			default:
				return nil, fmt.Errorf("device token poll error: %s", oauthErr)
			}
		}
	}
}

// RefreshOAuthToken refreshes the access token using the refresh token.
func (c *Client) RefreshOAuthToken() (*TokenSet, error) {
	endpoint := strings.TrimSuffix(c.cfg.URL, "/") + apiPrefix + "/oauth/token"
	data := url.Values{
		"grant_type":    {"refresh_token"},
		"refresh_token": {c.cfg.RefreshToken},
		"client_id":     {"tstamps-cli"},
	}
	req, err := http.NewRequest(http.MethodPost, endpoint, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, serverError(resp)
	}

	var tok TokenSet
	if err := json.NewDecoder(resp.Body).Decode(&tok); err != nil {
		return nil, err
	}

	c.cfg.Token = tok.AccessToken
	if tok.RefreshToken != "" {
		c.cfg.RefreshToken = tok.RefreshToken
	}
	if c.onTokenRefresh != nil {
		c.onTokenRefresh(c.cfg)
	}
	return &tok, nil
}

// Login authenticates with a password and keeps the session cookie for later calls.
func (c *Client) Login(email, password string) (*Me, error) {
	var me Me
	body := map[string]string{"email": email, "password": password}
	if err := c.do(http.MethodPost, "/auth/login", body, &me); err != nil {
		return nil, err
	}
	return &me, nil
}

// Me returns the user the credentials belong to.
func (c *Client) Me() (*Me, error) {
	var me Me
	if err := c.do(http.MethodGet, "/me", nil, &me); err != nil {
		return nil, err
	}
	return &me, nil
}

// CreateToken mints a personal API token; the raw value is in the reply only.
func (c *Client) CreateToken(name string) (*Token, error) {
	var t Token
	if err := c.do(http.MethodPost, "/me/tokens", map[string]string{"name": name}, &t); err != nil {
		return nil, err
	}
	return &t, nil
}

// ListTokens returns the caller's tokens, revoked ones included.
func (c *Client) ListTokens() ([]Token, error) {
	var out []Token
	if err := c.do(http.MethodGet, "/me/tokens", nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// RevokeToken revokes one of the caller's tokens.
func (c *Client) RevokeToken(id uint) error {
	return c.do(http.MethodDelete, "/me/tokens/"+itoa(id), nil, nil)
}

// do sends a JSON request and decodes a JSON reply. Path may carry a query.
func (c *Client) do(method, path string, in, out any) error {
	resp, err := c.send(method, path, in)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if out == nil {
		return nil
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

// bytes sends a request and returns the raw body, for the CSV export.
func (c *Client) bytes(method, path string) ([]byte, error) {
	resp, err := c.send(method, path, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	return io.ReadAll(resp.Body)
}

func (c *Client) send(method, path string, in any) (*http.Response, error) {
	var body io.Reader
	if in != nil {
		b, err := json.Marshal(in)
		if err != nil {
			return nil, err
		}
		body = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, strings.TrimSuffix(c.cfg.URL, "/")+apiPrefix+path, body)
	if err != nil {
		return nil, err
	}
	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.cfg.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.cfg.Token)
	}
	if method != http.MethodGet {
		// Cookie-authenticated mutations need a same-origin Origin; harmless on bearer requests.
		req.Header.Set("Origin", c.origin())
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode == http.StatusUnauthorized && c.cfg.RefreshToken != "" && !strings.Contains(path, "/oauth/token") {
		resp.Body.Close()
		if _, err := c.RefreshOAuthToken(); err == nil {
			// Retry request with new token
			if in != nil {
				b, _ := json.Marshal(in)
				body = bytes.NewReader(b)
			}
			req2, err := http.NewRequest(method, strings.TrimSuffix(c.cfg.URL, "/")+apiPrefix+path, body)
			if err != nil {
				return nil, err
			}
			if in != nil {
				req2.Header.Set("Content-Type", "application/json")
			}
			if c.cfg.Token != "" {
				req2.Header.Set("Authorization", "Bearer "+c.cfg.Token)
			}
			if method != http.MethodGet {
				req2.Header.Set("Origin", c.origin())
			}
			resp, err = c.http.Do(req2)
			if err != nil {
				return nil, err
			}
		}
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		defer resp.Body.Close()
		return nil, serverError(resp)
	}
	return resp, nil
}

// origin is the scheme://host the server compares its Origin header against.
func (c *Client) origin() string {
	u, err := url.Parse(c.cfg.URL)
	if err != nil {
		return ""
	}
	return u.Scheme + "://" + u.Host
}

func serverError(resp *http.Response) error {
	var body struct {
		Type         string `json:"type"`
		Detail       string `json:"detail"`
		RunningEntry *Entry `json:"running_entry"`
	}
	// A non-JSON error body still has a status, which is the useful part.
	_ = json.NewDecoder(resp.Body).Decode(&body)
	return &Error{
		StatusCode: resp.StatusCode, Type: body.Type,
		Detail: body.Detail, RunningEntry: body.RunningEntry,
	}
}
