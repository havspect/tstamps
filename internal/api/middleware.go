package api

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"github.com/havspect/tstamps/internal/auth"
	"github.com/havspect/tstamps/internal/store"
)

const (
	sessionCookie = "tstamps_session"
	// The __Host- prefix makes browsers require https, Path=/ and no Domain;
	// plain http cannot set it, so the bare name is used there instead.
	secureSessionCookie = "__Host-" + sessionCookie
	// maxRequestBody bounds JSON bodies; no legitimate request comes close.
	maxRequestBody = 1 << 20
)

// statusRecorder keeps the status and size for the access log without changing
// the handler's view of the response.
type statusRecorder struct {
	http.ResponseWriter
	status int
	bytes  int
}

func (r *statusRecorder) WriteHeader(status int) {
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}

func (r *statusRecorder) Write(b []byte) (int, error) {
	if r.status == 0 {
		r.status = http.StatusOK
	}
	n, err := r.ResponseWriter.Write(b)
	r.bytes += n
	return n, err
}

// Unwrap lets http.ResponseController reach the underlying writer.
func (r *statusRecorder) Unwrap() http.ResponseWriter { return r.ResponseWriter }

type requestInfo struct {
	Request    *http.Request
	User       *store.User     // nil when unauthenticated
	Session    *store.Session  // nil unless credentials came from the cookie
	Token      *store.APIToken // nil unless credentials came from the bearer header
	ClientID   string          // OAuth client_id if authenticated via OAuth token
	GrantID    string          // OAuth grant request_id if authenticated via OAuth token
	Scopes     []string        // OAuth scopes if authenticated via OAuth token
	Cookie     bool            // credentials came from the cookie
	CookieName string          // which session cookie the request carried, for clearing it
}

type requestKey struct{}
type workspaceKey struct{}

// WithWorkspace labels a request with the hosted workspace slug so the access log
// can name it after the router strips the /w/{slug} prefix.
func WithWorkspace(r *http.Request, slug string) *http.Request {
	return r.WithContext(context.WithValue(r.Context(), workspaceKey{}, slug))
}

func workspaceFrom(ctx context.Context) string {
	slug, _ := ctx.Value(workspaceKey{}).(string)
	return slug
}

func requestFrom(ctx context.Context) *requestInfo {
	ri, _ := ctx.Value(requestKey{}).(*requestInfo)
	return ri
}

func principal(ctx context.Context) *store.User {
	if ri := requestFrom(ctx); ri != nil {
		return ri.User
	}
	return nil
}

// withRequest resolves the session and enforces the CSRF rule for cookie-authenticated
// mutations; it never rejects an unauthenticated request, so unknown paths still 404.
// A database error is not an anonymous request, so it answers 500 instead.
func (s *Server) withRequest(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, maxRequestBody)
		// withAccessLog made the requestInfo; a direct call still needs one.
		ri := requestFrom(r.Context())
		if ri == nil {
			ri = &requestInfo{}
			r = r.WithContext(context.WithValue(r.Context(), requestKey{}, ri))
		}
		ri.Request = r
		// A bearer header wins over a cookie: a bad token must not fall back to
		// whatever session the browser happens to hold.
		if token, ok := bearerToken(r); ok {
			if err := s.applyToken(ri, token); err != nil {
				slog.Error("token auth", "error", err)
				writeProblem(w, internal(err))
				return
			}
		} else if name, value, ok := sessionToken(r); ok {
			ri.CookieName = name
			sess, err := s.Store.SessionByTokenHash(auth.HashToken(value))
			switch {
			case errors.Is(err, store.ErrNotFound):
				// Stale cookie: continue as anonymous.
			case err != nil:
				slog.Error("session lookup", "error", err)
				writeProblem(w, internal(err))
				return
			case !s.Clock.Now().Before(sess.ExpiresAt):
				s.expireSession(w, name, sess.ID, r)
			default:
				u, err := s.Store.UserByID(sess.UserID)
				switch {
				case errors.Is(err, store.ErrNotFound):
					// The user is gone: continue as anonymous.
				case err != nil:
					slog.Error("session user lookup", "error", err)
					writeProblem(w, internal(err))
					return
				case u.Active:
					ri.User, ri.Session, ri.Cookie = u, sess, true
				}
			}
		}
		if ri.Cookie && !safeMethod(r.Method) && !sameOrigin(r) {
			writeProblem(w, problem(http.StatusForbidden, urnInvalidOrigin, "cross-origin request rejected"))
			return
		}
		if ri.Token == nil && ri.Session == nil && ri.User != nil {
			// Authenticated via OAuth token, check scopes
			hasRead := false
			hasWrite := false
			for _, sc := range ri.Scopes {
				if sc == "tstamps.read" {
					hasRead = true
				}
				if sc == "tstamps.write" {
					hasWrite = true
				}
			}
			isMCP := r.URL.Path == "/mcp" || r.URL.Path == apiPrefix+"/mcp"
			if safeMethod(r.Method) || isMCP {
				if !hasRead && !hasWrite {
					writeProblem(w, problem(http.StatusForbidden, urnForbidden, "missing tstamps.read scope"))
					return
				}
			} else {
				if !hasWrite {
					writeProblem(w, problem(http.StatusForbidden, urnForbidden, "missing tstamps.write scope"))
					return
				}
			}
		}
		// A user created or reset by an admin reaches only the endpoints the spec
		// allows until they replace the initial password.
		if ri.User != nil && ri.User.MustChangePassword && !passwordChangePath(r) {
			writeProblem(w, problem(http.StatusForbidden, urnPasswordChangeRequired, "change your password first"))
			return
		}
		next.ServeHTTP(w, r)
	})
}

// withAccessLog records one line per API request after the handler replies,
// including the early 401/403 replies, so no request disappears from the log.
// The path is logged without its query string.
func (s *Server) withAccessLog(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		ri := &requestInfo{}
		r = r.WithContext(context.WithValue(r.Context(), requestKey{}, ri))
		rec := &statusRecorder{ResponseWriter: w}
		next.ServeHTTP(rec, r)

		status := rec.status
		if status == 0 {
			status = http.StatusOK
		}
		attrs := []any{
			"method", r.Method,
			"path", r.URL.Path,
			"status", status,
			"duration_ms", time.Since(start).Milliseconds(),
			"bytes", rec.bytes,
			"ip", clientIP(r),
		}
		if slug := workspaceFrom(r.Context()); slug != "" {
			attrs = append(attrs, "workspace", slug)
		}
		if ri.User != nil {
			attrs = append(attrs, "user_id", ri.User.ID)
		}
		slog.Info("request", attrs...)
	})
}

// bearerToken returns the personal API token or OAuth token presented in the Authorization header.
func bearerToken(r *http.Request) (string, bool) {
	scheme, token, ok := strings.Cut(r.Header.Get("Authorization"), " ")
	if !ok || !strings.EqualFold(scheme, "Bearer") || token == "" {
		return "", false
	}
	return token, true
}

// applyToken authenticates a personal token or OAuth access token as its owning user.
func (s *Server) applyToken(ri *requestInfo, token string) error {
	t, err := s.Store.APITokenByTokenHash(auth.HashToken(token))
	switch {
	case err != nil && !errors.Is(err, store.ErrNotFound):
		return err
	case err == nil:
		if t.RevokedAt != nil {
			return nil
		}
		u, err := s.Store.UserByID(t.UserID)
		switch {
		case errors.Is(err, store.ErrNotFound):
			return nil
		case err != nil:
			return err
		case !u.Active:
			return nil
		}
		ri.User, ri.Token = u, t
		ri.Scopes = []string{"tstamps.read", "tstamps.write"}
		s.touchToken(t)
		return nil
	}

	// Try Fosite OAuth Access Token
	sig := token
	if idx := strings.LastIndex(token, "."); idx != -1 {
		sig = token[idx+1:]
	}
	at, err := s.Store.OAuthAccessTokenBySignatureHash(sig)
	if err != nil || at == nil {
		return nil
	}
	if at.RevokedAt != nil || !at.ExpiresAt.After(s.Clock.Now()) {
		return nil
	}

	userID := at.UserID
	if userID == 0 && len(at.SessionData) > 0 {
		var sess store.OAuthSession
		_ = json.Unmarshal([]byte(at.SessionData), &sess)
		userID = sess.UserID
	}
	u, err := s.Store.UserByID(userID)
	if err != nil || u == nil || !u.Active {
		return nil
	}

	var scopes []string
	if err := json.Unmarshal([]byte(at.Scopes), &scopes); err != nil {
		scopes = strings.Fields(at.Scopes)
	}
	ri.User = u
	ri.ClientID = at.ClientID
	ri.GrantID = at.RequestID
	ri.Scopes = scopes
	_ = s.Store.TouchOAuthAccessToken(at.ID, s.Clock.Now())
	return nil
}

// touchToken records last_used_at, at most once a minute so a busy client does
// not turn every read into a write.
func (s *Server) touchToken(t *store.APIToken) {
	now := s.Clock.Now()
	if t.LastUsedAt != nil && now.Sub(*t.LastUsedAt) < time.Minute {
		return
	}
	if err := s.Store.TouchAPIToken(t.ID, now); err != nil {
		slog.Error("touch api token", "error", err)
	}
}

// passwordChangePath lists what a user with must_change_password may reach.
func passwordChangePath(r *http.Request) bool {
	switch r.Method + " " + r.URL.Path {
	case "GET " + apiPrefix + "/me",
		"POST " + apiPrefix + "/me/password",
		"POST " + apiPrefix + "/auth/logout",
		"POST " + apiPrefix + "/auth/login":
		return true
	}
	return false
}

// expireSession drops a session the clock has passed and asks the browser to forget
// the cookie; both are best effort so the request still proceeds as anonymous.
func (s *Server) expireSession(w http.ResponseWriter, name string, id uint, r *http.Request) {
	if err := s.Store.DeleteSession(id); err != nil {
		slog.Error("delete expired session", "error", err)
	}
	http.SetCookie(w, clearCookie(name, r))
}

// sessionToken returns the session cookie presented with r, preferring the secure name.
func sessionToken(r *http.Request) (name, value string, ok bool) {
	for _, n := range []string{secureSessionCookie, sessionCookie} {
		if c, err := r.Cookie(n); err == nil {
			return c.Name, c.Value, true
		}
	}
	return "", "", false
}

// sessionCookieName is the name a response should set: browsers reject __Host- over http.
func sessionCookieName(r *http.Request) string {
	if isHTTPS(r) {
		return secureSessionCookie
	}
	return sessionCookie
}

func clearCookie(name string, r *http.Request) *http.Cookie {
	return &http.Cookie{
		Name: name, Path: "/", MaxAge: -1, Expires: time.Unix(0, 0),
		HttpOnly: true, SameSite: http.SameSiteLaxMode, Secure: isHTTPS(r),
	}
}

func safeMethod(method string) bool {
	return method == http.MethodGet || method == http.MethodHead || method == http.MethodOptions
}

func sameOrigin(r *http.Request) bool {
	scheme := "http"
	if isHTTPS(r) {
		scheme = "https"
	}
	return r.Header.Get("Origin") == scheme+"://"+r.Host
}

// proxyPeer reports whether the request came from the reverse proxy on this host.
// Only a loopback peer may set X-Forwarded-*; a direct client cannot forge them.
func proxyPeer(r *http.Request) bool {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func isHTTPS(r *http.Request) bool {
	if r.TLS != nil {
		return true
	}
	if !proxyPeer(r) {
		return false
	}
	// Proxies append their own scheme, so only the first value is the client's.
	proto, _, _ := strings.Cut(r.Header.Get("X-Forwarded-Proto"), ",")
	return strings.TrimSpace(proto) == "https"
}

// writeProblem skips the encode error: the status and content type are already
// sent, and a failed write has nothing left to report to.
func writeProblem(w http.ResponseWriter, em *huma.ErrorModel) {
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(em.Status)
	_ = json.NewEncoder(w).Encode(em)
}
