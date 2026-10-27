package httpapi

import (
	"context"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/havspect/tstamps/internal/auth"
	"github.com/havspect/tstamps/internal/oauth"
	gormrepo "github.com/havspect/tstamps/internal/repository/gorm"
	"gorm.io/gorm"
)

const (
	sessionCookie       = "tstamps_session"
	secureSessionCookie = "__Host-" + sessionCookie
	maxRequestBody      = 1 << 20
)

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

func (r *statusRecorder) Unwrap() http.ResponseWriter {
	return r.ResponseWriter
}

// SecurityHeaders sets standard web security headers.
func SecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		csp := "default-src 'self'; connect-src 'self' data:; style-src 'self' 'unsafe-inline'; img-src 'self' data:; object-src 'none'; base-uri 'self'; frame-ancestors 'none'"
		if r.URL.Path == "/docs" || strings.HasPrefix(r.URL.Path, "/docs/") || strings.HasSuffix(r.URL.Path, "/api/v1/docs") {
			csp = "default-src 'self'; script-src 'self' 'unsafe-inline'; connect-src 'self' data:; style-src 'self' 'unsafe-inline'; img-src 'self' data:; object-src 'none'; base-uri 'self'; frame-ancestors 'none'"
		}
		h.Set("Content-Security-Policy", csp)
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "same-origin")
		next.ServeHTTP(w, r)
	})
}

func clientIP(r *http.Request) string {
	if fwd := r.Header.Get("X-Forwarded-For"); fwd != "" {
		if i := strings.Index(fwd, ","); i != -1 {
			return strings.TrimSpace(fwd[:i])
		}
		return strings.TrimSpace(fwd)
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func isHTTPS(r *http.Request) bool {
	if r.TLS != nil {
		return true
	}
	return strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
}

func origin(r *http.Request) string {
	if isHTTPS(r) {
		return "https://" + r.Host
	}
	return "http://" + r.Host
}

func sameOrigin(r *http.Request) bool {
	target := origin(r)
	if orig := r.Header.Get("Origin"); orig != "" {
		return strings.EqualFold(orig, target)
	}
	if ref := r.Header.Get("Referer"); ref != "" {
		return strings.HasPrefix(ref, target+"/") || strings.EqualFold(ref, target)
	}
	return false
}

func safeMethod(m string) bool {
	return m == http.MethodGet || m == http.MethodHead || m == http.MethodOptions
}

func bearerToken(r *http.Request) (string, bool) {
	scheme, token, ok := strings.Cut(r.Header.Get("Authorization"), " ")
	if !ok || !strings.EqualFold(scheme, "Bearer") || token == "" {
		return "", false
	}
	return token, true
}

func sessionToken(r *http.Request) (string, string, bool) {
	if c, err := r.Cookie(secureSessionCookie); err == nil && c.Value != "" {
		return secureSessionCookie, c.Value, true
	}
	if c, err := r.Cookie(sessionCookie); err == nil && c.Value != "" {
		return sessionCookie, c.Value, true
	}
	return "", "", false
}

// withAccessLog records request duration, status, and actor details.
func (s *Server) withAccessLog(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
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

		if p, ok := auth.PrincipalFrom(r.Context()); ok && p != nil {
			attrs = append(attrs, "user_id", p.UserID, "workspace_id", p.WorkspaceID)
		}
		slog.Info("request", attrs...)
	})
}

// withAuth extracts credentials, resolves user, and sets auth.Principal and workspace ID in ctx.
func (s *Server) withAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, maxRequestBody)
		ctx := r.Context()
		var principal *auth.Principal
		var wsID uint
		fromCookie := false

		if rawToken, ok := bearerToken(r); ok {
			p, wID := s.authenticateBearer(ctx, rawToken)
			if p != nil {
				principal = p
				wsID = wID
			}
		} else if _, cookieVal, ok := sessionToken(r); ok {
			p, wID := s.authenticateSession(ctx, cookieVal)
			if p != nil {
				principal = p
				wsID = wID
				fromCookie = true
			}
		}

		if principal != nil {
			ctx = auth.WithPrincipal(ctx, principal)
			ctx = gormrepo.WithWorkspaceID(ctx, wsID)
			r = r.WithContext(ctx)
		}

		// CSRF check for cookie-authenticated mutations
		if fromCookie && !safeMethod(r.Method) && !sameOrigin(r) {
			writeProblem(w, problem(http.StatusForbidden, urnInvalidOrigin, "cross-origin request rejected"))
			return
		}

		// OAuth scope verification
		if principal != nil && principal.Source == "oauth" {
			hasRead := principal.HasScope("tstamps.read")
			hasWrite := principal.HasScope("tstamps.write")
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

		// Enforce password change restriction
		if principal != nil && principalHasPasswordChangeRequired(principal, s.deps.DB) && !isPasswordChangeAllowedPath(r) {
			writeProblem(w, problem(http.StatusForbidden, urnPasswordChangeRequired, "change your password first"))
			return
		}

		next.ServeHTTP(w, r)
	})
}

func (s *Server) authenticateBearer(ctx context.Context, rawToken string) (*auth.Principal, uint) {
	if s.deps.DB == nil {
		return nil, 0
	}
	hash := auth.HashToken(rawToken)
	bypassCtx := gormrepo.WithBypassTenancy(ctx)

	// 1. Try personal API token
	var tok gormrepo.APIToken
	err := s.deps.DB.WithContext(bypassCtx).Where("token_hash = ? AND revoked_at IS NULL", hash).First(&tok).Error
	if err == nil {
		var u gormrepo.User
		if err := s.deps.DB.WithContext(bypassCtx).First(&u, tok.UserID).Error; err == nil && u.Active {
			_ = s.deps.DB.WithContext(bypassCtx).Model(&tok).Update("last_used_at", time.Now())
			p := s.buildPrincipal(&u, "bearer", strconv.FormatUint(uint64(tok.ID), 10), []string{"tstamps.read", "tstamps.write"}, "")
			return p, tok.WorkspaceID
		}
	}

	// 2. Try OAuth access token via oauthProvider.IntrospectToken
	if s.deps.OAuthProvider != nil {
		sess := oauth.NewSession(0, "", "")
		tokenType, accessReq, err := s.deps.OAuthProvider.IntrospectToken(ctx, rawToken, oauth.AccessToken, sess)
		if err == nil && tokenType == oauth.AccessToken && sess.UserID > 0 {
			var u gormrepo.User
			if err := s.deps.DB.WithContext(bypassCtx).First(&u, sess.UserID).Error; err == nil && u.Active {
				scopes := accessReq.GetGrantedScopes()
				clientID := ""
				if client := accessReq.GetClient(); client != nil {
					clientID = client.GetID()
				}
				p := s.buildPrincipal(&u, "oauth", accessReq.GetID(), scopes, clientID)
				return p, u.WorkspaceID
			}
		}
	}

	return nil, 0
}

func (s *Server) authenticateSession(ctx context.Context, cookieVal string) (*auth.Principal, uint) {
	if s.deps.DB == nil {
		return nil, 0
	}
	hash := auth.HashToken(cookieVal)
	bypassCtx := gormrepo.WithBypassTenancy(ctx)
	var sess gormrepo.Session
	err := s.deps.DB.WithContext(bypassCtx).Where("token_hash = ?", hash).First(&sess).Error
	if err != nil {
		return nil, 0
	}

	if !time.Now().Before(sess.ExpiresAt) {
		return nil, 0
	}

	var u gormrepo.User
	if err := s.deps.DB.WithContext(bypassCtx).First(&u, sess.UserID).Error; err != nil || !u.Active {
		return nil, 0
	}

	p := s.buildPrincipal(&u, "session", sess.TokenHash, []string{"tstamps.read", "tstamps.write"}, "")
	return p, sess.WorkspaceID
}

func (s *Server) buildPrincipal(u *gormrepo.User, source, tokenID string, scopes []string, clientID string) *auth.Principal {
	roles := make([]string, 0, 2)
	if u.IsAdmin {
		roles = append(roles, "admin")
	} else {
		roles = append(roles, "member")
	}

	var ledTeamIDs []uint
	if s.deps.DB != nil {
		var teamIDs []uint
		_ = s.deps.DB.WithContext(gormrepo.WithWorkspaceID(context.Background(), u.WorkspaceID)).
			Model(&gormrepo.TeamMembership{}).
			Where("user_id = ? AND role = 'lead'", u.ID).
			Pluck("team_id", &teamIDs)
		if len(teamIDs) > 0 {
			roles = append(roles, "lead")
			ledTeamIDs = teamIDs
		}
	}

	return &auth.Principal{
		WorkspaceID: u.WorkspaceID,
		UserID:      u.ID,
		Email:       u.Email,
		Roles:       roles,
		Scopes:      scopes,
		ClientID:    clientID,
		TokenID:     tokenID,
		Source:      source,
		LedTeamIDs:  ledTeamIDs,
	}
}

func principalHasPasswordChangeRequired(p *auth.Principal, db *gorm.DB) bool {
	if p == nil || db == nil {
		return false
	}
	var u gormrepo.User
	if err := db.Select("must_change_password").First(&u, p.UserID).Error; err == nil {
		return u.MustChangePassword
	}
	return false
}

func isPasswordChangeAllowedPath(r *http.Request) bool {
	switch r.Method + " " + r.URL.Path {
	case "GET " + apiPrefix + "/me",
		"POST " + apiPrefix + "/me/password",
		"POST " + apiPrefix + "/auth/logout",
		"POST " + apiPrefix + "/auth/login":
		return true
	}
	return false
}
