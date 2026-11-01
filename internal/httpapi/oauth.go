package httpapi

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/havspect/tstamps/internal/auth"
	"github.com/havspect/tstamps/internal/oauth"
	gormrepo "github.com/havspect/tstamps/internal/repository/gorm"
)

func randomHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func randomUserCode() string {
	const alphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	out := make([]byte, 9)
	for i := 0; i < 8; i++ {
		ch := alphabet[int(b[i])%len(alphabet)]
		if i < 4 {
			out[i] = ch
		} else {
			out[i+1] = ch
		}
	}
	out[4] = '-'
	return string(out)
}

func issuerURL(r *http.Request) string {
	return origin(r)
}

// writeOAuthError replies with an RFC 6749 error body.
func writeOAuthError(w http.ResponseWriter, status int, code, description string) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"error":             code,
		"error_description": description,
	})
}

// deviceWorkspace resolves the workspace for an unauthenticated device request:
// the one bound to the request context, else the only workspace on the server.
func (s *Server) deviceWorkspace(ctx context.Context) (*gormrepo.Workspace, bool) {
	if s.deps.DB == nil {
		return nil, false
	}
	var ws gormrepo.Workspace
	if id, ok := gormrepo.WorkspaceIDFrom(ctx); ok {
		if err := s.deps.DB.WithContext(ctx).First(&ws, id).Error; err != nil {
			return nil, false
		}
		return &ws, true
	}
	var all []gormrepo.Workspace
	if err := s.deps.DB.WithContext(ctx).Order("id ASC").Limit(2).Find(&all).Error; err != nil || len(all) != 1 {
		return nil, false
	}
	return &all[0], true
}

func discoveryHandler(w http.ResponseWriter, r *http.Request) {
	issuer := issuerURL(r)
	disc := map[string]any{
		"issuer":                                issuer,
		"authorization_endpoint":                issuer + "/oauth2/authorize",
		"token_endpoint":                        issuer + "/api/v1/oauth/token",
		"registration_endpoint":                 issuer + "/api/v1/oauth/register",
		"revocation_endpoint":                   issuer + "/api/v1/oauth/revoke",
		"device_authorization_endpoint":         issuer + "/api/v1/auth/device",
		"scopes_supported":                      []string{"tstamps.read", "tstamps.write"},
		"response_types_supported":              []string{"code"},
		"grant_types_supported":                 []string{"authorization_code", "refresh_token", "urn:ietf:params:oauth:grant-type:device_code"},
		"code_challenge_methods_supported":      []string{"S256"},
		"token_endpoint_auth_methods_supported": []string{"none"},
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(disc)
}

func protectedResourceHandler(w http.ResponseWriter, r *http.Request) {
	issuer := issuerURL(r)
	res := map[string]any{
		"resource":              issuer,
		"authorization_servers": []string{issuer},
		"scopes_supported":      []string{"tstamps.read", "tstamps.write"},
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(res)
}

func (s *Server) registerOAuthRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /.well-known/oauth-authorization-server", discoveryHandler)
	mux.HandleFunc("GET /.well-known/oauth-protected-resource", protectedResourceHandler)

	if s.deps.OAuthProvider == nil {
		return
	}

	// Register client
	mux.HandleFunc("POST "+apiPrefix+"/oauth/register", func(w http.ResponseWriter, r *http.Request) {
		var req oauth.ClientRegistrationRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeProblem(w, problem(http.StatusBadRequest, urnInvalidInput, "invalid request body"))
			return
		}
		resp, err := s.deps.OAuthProvider.RegisterClient(r.Context(), req)
		if err != nil {
			writeProblem(w, problem(http.StatusBadRequest, urnInvalidInput, err.Error()))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(resp)
	})

	handleAuthorizeGet := func(w http.ResponseWriter, r *http.Request) {
		p, ok := auth.PrincipalFrom(r.Context())
		if !ok || p == nil {
			writeProblem(w, problem(http.StatusUnauthorized, urnUnauthorized, "authentication required"))
			return
		}

		ar, err := s.deps.OAuthProvider.NewAuthorizeRequest(r.Context(), r)
		if err != nil {
			writeProblem(w, problem(http.StatusBadRequest, urnInvalidInput, oauth.ErrorToRFC6749Error(err).GetDescription()))
			return
		}

		client := ar.GetClient()
		scopes := []string(ar.GetRequestedScopes())
		if len(scopes) == 0 {
			scopes = []string{"tstamps.read", "tstamps.write"}
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"client_id":    client.GetID(),
			"scopes":       scopes,
			"redirect_uri": ar.GetRedirectURI().String(),
			"state":        ar.GetState(),
		})
	}

	handleAuthorizePost := func(w http.ResponseWriter, r *http.Request) {
		p, ok := auth.PrincipalFrom(r.Context())
		if !ok || p == nil {
			writeProblem(w, problem(http.StatusUnauthorized, urnUnauthorized, "authentication required"))
			return
		}

		var body struct {
			Approve bool `json:"approve"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeProblem(w, problem(http.StatusBadRequest, urnInvalidInput, "invalid request body"))
			return
		}

		rClone := r.Clone(r.Context())
		rClone.Method = http.MethodGet
		rClone.Body = http.NoBody
		rClone.Form = rClone.URL.Query()
		rClone.PostForm = url.Values{}

		ar, err := s.deps.OAuthProvider.NewAuthorizeRequest(r.Context(), rClone)
		if err != nil {
			writeProblem(w, problem(http.StatusBadRequest, urnInvalidInput, oauth.ErrorToRFC6749Error(err).GetDescription()))
			return
		}

		redirectURI := ar.GetRedirectURI().String()
		state := ar.GetState()

		if !body.Approve {
			val := url.Values{}
			val.Set("error", "access_denied")
			if state != "" {
				val.Set("state", state)
			}
			sep := "?"
			if strings.Contains(redirectURI, "?") {
				sep = "&"
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]string{
				"redirect_to": redirectURI + sep + val.Encode(),
			})
			return
		}

		requested := ar.GetRequestedScopes()
		if len(requested) == 0 {
			ar.GrantScope("tstamps.read")
			ar.GrantScope("tstamps.write")
		} else {
			for _, sc := range requested {
				if sc == "tstamps.read" || sc == "tstamps.write" {
					ar.GrantScope(sc)
				}
			}
		}

		now := time.Now()
		if s.deps.Clock != nil {
			now = s.deps.Clock.Now()
		}

		sess := &oauth.OAuthSession{
			UserID:   p.UserID,
			Username: p.Email,
			Subject:  fmt.Sprintf("%d", p.UserID),
			ExpiresAt: map[oauth.TokenType]time.Time{
				oauth.AuthorizeCode: now.Add(time.Minute),
				oauth.AccessToken:   now.Add(time.Hour),
				oauth.RefreshToken:  now.Add(30 * 24 * time.Hour),
			},
		}

		response, err := s.deps.OAuthProvider.NewAuthorizeResponse(r.Context(), ar, sess)
		if err != nil {
			writeProblem(w, problem(http.StatusBadRequest, urnInvalidInput, oauth.ErrorToRFC6749Error(err).GetDescription()))
			return
		}

		val := response.GetParameters()
		if state != "" && val.Get("state") == "" {
			val.Set("state", state)
		}
		sep := "?"
		if strings.Contains(redirectURI, "?") {
			sep = "&"
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{
			"redirect_to": redirectURI + sep + val.Encode(),
		})
	}

	mux.HandleFunc("GET "+apiPrefix+"/oauth/authorize", handleAuthorizeGet)
	mux.HandleFunc("POST "+apiPrefix+"/oauth/authorize", handleAuthorizePost)
	mux.HandleFunc("GET /oauth2/authorize", handleAuthorizeGet)
	mux.HandleFunc("POST /oauth2/authorize", handleAuthorizePost)
	mux.HandleFunc("GET /oauth/authorize", handleAuthorizeGet)
	mux.HandleFunc("POST /oauth/authorize", handleAuthorizePost)

	// Token endpoint
	mux.HandleFunc("POST "+apiPrefix+"/oauth/token", func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()

		ctx := r.Context()
		if _, ok := gormrepo.WorkspaceIDFrom(ctx); !ok && s.deps.DB != nil && r.PostForm.Get("grant_type") == "refresh_token" {
			sig := s.deps.OAuthProvider.Strategy().RefreshTokenSignature(ctx, r.PostForm.Get("refresh_token"))
			var rt gormrepo.OAuthRefreshToken
			if err := s.deps.DB.WithContext(gormrepo.WithBypassTenancy(ctx)).Where("signature_hash = ?", sig).First(&rt).Error; err == nil && rt.WorkspaceID > 0 {
				ctx = gormrepo.WithWorkspaceID(ctx, rt.WorkspaceID)
				r = r.WithContext(ctx)
			}
		}

		sess := oauth.NewSession(0, "", "")
		accessReq, err := s.deps.OAuthProvider.NewAccessRequest(ctx, r, sess)
		if err != nil {
			s.deps.OAuthProvider.WriteAccessError(ctx, w, accessReq, err)
			return
		}
		resp, err := s.deps.OAuthProvider.NewAccessResponse(ctx, accessReq)
		if err != nil {
			s.deps.OAuthProvider.WriteAccessError(ctx, w, accessReq, err)
			return
		}
		s.deps.OAuthProvider.WriteAccessResponse(ctx, w, accessReq, resp)
	})

	// Revoke endpoint
	mux.HandleFunc("POST "+apiPrefix+"/oauth/revoke", func(w http.ResponseWriter, r *http.Request) {
		err := s.deps.OAuthProvider.NewRevocationRequest(r.Context(), r)
		s.deps.OAuthProvider.WriteRevocationResponse(r.Context(), w, err)
	})

	// Device code endpoints
	s.registerDeviceCodeRoutes(mux)
}

func (s *Server) registerDeviceCodeRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST "+apiPrefix+"/auth/device", func(w http.ResponseWriter, r *http.Request) {
		if s.deps.DB == nil {
			writeProblem(w, problem(http.StatusServiceUnavailable, urnUnavailable, "database not configured"))
			return
		}
		if err := r.ParseForm(); err != nil {
			writeProblem(w, problem(http.StatusBadRequest, urnInvalidInput, "invalid form data"))
			return
		}
		clientID := r.Form.Get("client_id")
		if clientID == "" {
			writeProblem(w, problem(http.StatusBadRequest, urnInvalidInput, "client_id is required"))
			return
		}

		ws, ok := s.deviceWorkspace(r.Context())
		if !ok {
			writeProblem(w, problem(http.StatusBadRequest, urnInvalidInput, "workspace required"))
			return
		}

		userCode := randomUserCode()
		deviceCode := "dc_" + randomHex(24)
		devHash := auth.HashToken(deviceCode)
		userCodeHash := auth.HashToken(strings.ReplaceAll(userCode, "-", ""))
		reqID := "dev_" + randomHex(16)

		now := time.Now()
		if s.deps.Clock != nil {
			now = s.deps.Clock.Now()
		}
		expires := now.Add(10 * time.Minute)

		dc := gormrepo.OAuthDeviceCode{
			WorkspaceID:    ws.ID,
			ClientID:       clientID,
			DeviceCodeHash: devHash,
			UserCodeHash:   userCodeHash,
			RequestID:      reqID,
			Scopes:         `["tstamps.read","tstamps.write"]`,
			ExpiresAt:      expires,
			Status:         oauth.DeviceStatusPending,
			CreatedAt:      now,
		}
		if err := s.deps.DB.WithContext(gormrepo.WithWorkspaceID(r.Context(), ws.ID)).Create(&dc).Error; err != nil {
			writeProblem(w, internal(err))
			return
		}

		verificationURI := origin(r) + "/w/" + ws.Slug + "/auth/device"
		complete := url.Values{"user_code": {userCode}}.Encode()

		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"device_code":               deviceCode,
			"user_code":                 userCode,
			"verification_uri":          verificationURI,
			"verification_uri_complete": verificationURI + "?" + complete,
			"expires_in":                600,
			"interval":                  5,
		})
	})

	mux.HandleFunc("POST "+apiPrefix+"/auth/device/token", func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			writeOAuthError(w, http.StatusBadRequest, "invalid_request", "invalid form data")
			return
		}
		deviceCode := r.FormValue("device_code")
		if deviceCode == "" {
			writeOAuthError(w, http.StatusBadRequest, "invalid_request", "device_code is required")
			return
		}

		bypassCtx := gormrepo.WithBypassTenancy(r.Context())
		var dc gormrepo.OAuthDeviceCode
		if err := s.deps.DB.WithContext(bypassCtx).Where("device_code_hash = ?", auth.HashToken(deviceCode)).First(&dc).Error; err != nil {
			writeOAuthError(w, http.StatusBadRequest, "invalid_grant", "unknown device_code")
			return
		}

		now := time.Now()
		if s.deps.Clock != nil {
			now = s.deps.Clock.Now()
		}
		if !dc.ExpiresAt.After(now) {
			writeOAuthError(w, http.StatusBadRequest, "expired_token", "device_code has expired")
			return
		}
		switch dc.Status {
		case oauth.DeviceStatusPending:
			writeOAuthError(w, http.StatusBadRequest, "authorization_pending", "authorization is still pending")
			return
		case oauth.DeviceStatusDenied:
			writeOAuthError(w, http.StatusBadRequest, "access_denied", "user denied the request")
			return
		case oauth.DeviceStatusConsumed:
			writeOAuthError(w, http.StatusBadRequest, "invalid_grant", "device_code already used")
			return
		}

		var u gormrepo.User
		if err := s.deps.DB.WithContext(bypassCtx).First(&u, dc.UserID).Error; err != nil || !u.Active {
			writeOAuthError(w, http.StatusBadRequest, "invalid_grant", "user not active")
			return
		}

		ctx := gormrepo.WithWorkspaceID(r.Context(), dc.WorkspaceID)
		if err := s.deps.DB.WithContext(ctx).Model(&gormrepo.OAuthDeviceCode{}).Where("id = ?", dc.ID).Update("status", oauth.DeviceStatusConsumed).Error; err != nil {
			writeOAuthError(w, http.StatusInternalServerError, "server_error", err.Error())
			return
		}

		var scopes []string
		_ = json.Unmarshal([]byte(dc.Scopes), &scopes)
		if len(scopes) == 0 {
			scopes = []string{"tstamps.read", "tstamps.write"}
		}

		client, err := s.deps.OAuthProvider.Store().GetClient(ctx, dc.ClientID)
		if err != nil {
			writeOAuthError(w, http.StatusBadRequest, "invalid_client", "unknown client")
			return
		}

		req := oauth.NewRequest()
		req.ID = dc.RequestID
		req.RequestedAt = now
		req.Client = client
		req.RequestedScope = oauth.Arguments(scopes)
		req.GrantedScope = oauth.Arguments(scopes)
		req.Session = oauth.NewSession(u.ID, u.Email, strconv.FormatUint(uint64(u.ID), 10))

		accessToken, accessSig, err := s.deps.OAuthProvider.Strategy().GenerateAccessToken(ctx, req)
		if err != nil {
			writeOAuthError(w, http.StatusInternalServerError, "server_error", err.Error())
			return
		}
		if err := s.deps.OAuthProvider.Store().CreateAccessTokenSession(ctx, accessSig, req); err != nil {
			writeOAuthError(w, http.StatusInternalServerError, "server_error", err.Error())
			return
		}

		refreshToken, refreshSig, err := s.deps.OAuthProvider.Strategy().GenerateRefreshToken(ctx, req)
		if err != nil {
			writeOAuthError(w, http.StatusInternalServerError, "server_error", err.Error())
			return
		}
		if err := s.deps.OAuthProvider.Store().CreateRefreshTokenSession(ctx, refreshSig, accessSig, req); err != nil {
			writeOAuthError(w, http.StatusInternalServerError, "server_error", err.Error())
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token":  accessToken,
			"refresh_token": refreshToken,
			"token_type":    "Bearer",
			"expires_in":    3600,
			"scope":         strings.Join(scopes, " "),
		})
	})

	verify := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p, ok := auth.PrincipalFrom(r.Context())
		if !ok || p == nil {
			writeProblem(w, problem(http.StatusUnauthorized, urnUnauthorized, "authentication required"))
			return
		}

		var req struct {
			UserCode string `json:"user_code"`
			Approve  bool   `json:"approve"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeProblem(w, problem(http.StatusBadRequest, urnInvalidInput, "invalid request body"))
			return
		}

		cleanCode := strings.ToUpper(strings.ReplaceAll(req.UserCode, "-", ""))
		hash := auth.HashToken(cleanCode)

		var dc gormrepo.OAuthDeviceCode
		if err := s.deps.DB.WithContext(r.Context()).Where("user_code_hash = ?", hash).First(&dc).Error; err != nil {
			writeProblem(w, problem(http.StatusNotFound, urnNotFound, "invalid user code"))
			return
		}

		status := oauth.DeviceStatusApproved
		if !req.Approve {
			status = oauth.DeviceStatusDenied
		}

		if err := s.deps.DB.WithContext(r.Context()).Model(&dc).Updates(map[string]any{
			"status":  status,
			"user_id": p.UserID,
		}).Error; err != nil {
			writeProblem(w, internal(err))
			return
		}

		w.WriteHeader(http.StatusNoContent)
	})

	// These exact routes shadow "/api/" in the top-level mux, so they carry their own auth.
	mux.Handle("POST "+apiPrefix+"/auth/device/verify", s.withAuth(verify))
}
