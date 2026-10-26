package api

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/ory/fosite"
	"github.com/ory/fosite/compose"
	"github.com/ory/fosite/handler/oauth2"

	"github.com/havspect/tstamps/internal/auth"
	"github.com/havspect/tstamps/internal/store"
)

func (s *Server) configureOAuth() (fosite.OAuth2Provider, *oauth2.HMACSHAStrategy) {
	var secret []byte
	if s.Store != nil {
		secret, _ = s.Store.OAuthSecret()
	}
	if len(secret) < 32 {
		secret = []byte("tstamps-default-oauth-secret-key-32bytes")
	}

	config := &fosite.Config{
		GlobalSecret:                   secret,
		AccessTokenLifespan:            time.Hour,
		AuthorizeCodeLifespan:          time.Minute,
		RefreshTokenLifespan:           30 * 24 * time.Hour,
		EnforcePKCE:                    true,
		EnforcePKCEForPublicClients:    true,
		EnablePKCEPlainChallengeMethod: false,
		RefreshTokenScopes:             []string{"tstamps.read", "tstamps.write"},
	}

	var fs *store.FositeStore
	if s.Store != nil {
		fs = s.Store.Fosite()
	}
	hmacStrategy := compose.NewOAuth2HMACStrategy(config)
	provider := compose.Compose(
		config,
		fs,
		&compose.CommonStrategy{
			CoreStrategy: hmacStrategy,
		},
		compose.OAuth2AuthorizeExplicitFactory,
		compose.OAuth2PKCEFactory,
		compose.OAuth2RefreshTokenGrantFactory,
		compose.OAuth2TokenIntrospectionFactory,
		compose.OAuth2TokenRevocationFactory,
	)
	return provider, hmacStrategy
}

func issuerURL(r *http.Request) string {
	orig := origin(r)
	slug := workspaceFrom(r.Context())
	if slug != "" {
		return orig + "/w/" + slug
	}
	return orig
}

func discoveryHandler(w http.ResponseWriter, r *http.Request) {
	issuer := issuerURL(r)
	disc := map[string]any{
		"issuer":                                issuer,
		"authorization_endpoint":                issuer + "/oauth/authorize",
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

func isValidRedirectURI(raw string) bool {
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

func (s *Server) clientDisplayName(clientID string) string {
	switch clientID {
	case store.BuiltInCLIClientID:
		return "Tstamps CLI"
	case store.BuiltInMobileClientID:
		return "Tstamps Mobile"
	}
	if s.Store != nil {
		if cl, err := s.Store.OAuthClientByID(clientID); err == nil && cl.ClientName != "" {
			return cl.ClientName
		}
	}
	return clientID
}

func (s *Server) registerOAuthRoutes(apiMux *http.ServeMux) {
	oauthProvider, hmacStrategy := s.configureOAuth()

	apiMux.HandleFunc("POST "+apiPrefix+"/oauth/register", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			ClientName    string   `json:"client_name"`
			RedirectURIs  []string `json:"redirect_uris"`
			GrantTypes    []string `json:"grant_types"`
			ResponseTypes []string `json:"response_types"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeProblem(w, problem(http.StatusBadRequest, urnInvalidInput, "invalid request body"))
			return
		}
		if len(req.RedirectURIs) == 0 {
			writeProblem(w, problem(http.StatusBadRequest, urnInvalidInput, "redirect_uris required"))
			return
		}
		for _, uri := range req.RedirectURIs {
			if !isValidRedirectURI(uri) {
				writeProblem(w, problem(http.StatusBadRequest, urnInvalidInput, "invalid redirect uri: must be https or loopback"))
				return
			}
		}

		clientID := "cli_" + randomHex(16)
		if len(req.GrantTypes) == 0 {
			req.GrantTypes = []string{"authorization_code", "refresh_token"}
		}
		if len(req.ResponseTypes) == 0 {
			req.ResponseTypes = []string{"code"}
		}
		if req.ClientName == "" {
			req.ClientName = clientID
		}

		c := &store.OAuthClient{
			ClientID:      clientID,
			ClientName:    req.ClientName,
			RedirectURIs:  stringMustMarshal(req.RedirectURIs),
			GrantTypes:    stringMustMarshal(req.GrantTypes),
			ResponseTypes: stringMustMarshal(req.ResponseTypes),
			Scopes:        stringMustMarshal([]string{"tstamps.read", "tstamps.write"}),
			Public:        true,
		}
		if err := s.Store.CreateOAuthClient(c); err != nil {
			writeProblem(w, internal(err))
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"client_id":                  c.ClientID,
			"client_name":                c.ClientName,
			"redirect_uris":              req.RedirectURIs,
			"grant_types":                req.GrantTypes,
			"response_types":             req.ResponseTypes,
			"token_endpoint_auth_method": "none",
		})
	})

	handleAuthorizeGet := func(w http.ResponseWriter, r *http.Request) {
		ri := requestFrom(r.Context())
		if ri == nil || ri.User == nil {
			writeProblem(w, problem(http.StatusUnauthorized, urnUnauthorized, "authentication required"))
			return
		}

		ar, err := oauthProvider.NewAuthorizeRequest(r.Context(), r)
		if err != nil {
			writeProblem(w, problem(http.StatusBadRequest, urnInvalidInput, fosite.ErrorToRFC6749Error(err).GetDescription()))
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
			"client_name":  s.clientDisplayName(client.GetID()),
			"scopes":       scopes,
			"redirect_uri": ar.GetRedirectURI().String(),
			"state":        ar.GetState(),
		})
	}

	handleAuthorizePost := func(w http.ResponseWriter, r *http.Request) {
		ri := requestFrom(r.Context())
		if ri == nil || ri.User == nil {
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

		// Parse query parameters only so Fosite's NewAuthorizeRequest does not attempt to re-read r.Body as form data.
		rClone := r.Clone(r.Context())
		rClone.Method = http.MethodGet
		rClone.Body = http.NoBody
		rClone.Form = rClone.URL.Query()
		rClone.PostForm = url.Values{}

		ar, err := oauthProvider.NewAuthorizeRequest(r.Context(), rClone)
		if err != nil {
			writeProblem(w, problem(http.StatusBadRequest, urnInvalidInput, fosite.ErrorToRFC6749Error(err).GetDescription()))
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

		now := s.Clock.Now()
		sess := &store.OAuthSession{
			UserID:   ri.User.ID,
			Username: ri.User.Email,
			Subject:  fmt.Sprintf("%d", ri.User.ID),
			ExpiresAt: map[fosite.TokenType]time.Time{
				fosite.AuthorizeCode: now.Add(time.Minute),
				fosite.AccessToken:   now.Add(time.Hour),
				fosite.RefreshToken:  now.Add(30 * 24 * time.Hour),
			},
		}

		response, err := oauthProvider.NewAuthorizeResponse(r.Context(), ar, sess)
		if err != nil {
			writeProblem(w, problem(http.StatusBadRequest, urnInvalidInput, fosite.ErrorToRFC6749Error(err).GetDescription()))
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

	apiMux.HandleFunc("GET "+apiPrefix+"/oauth/authorize", handleAuthorizeGet)
	apiMux.HandleFunc("POST "+apiPrefix+"/oauth/authorize", handleAuthorizePost)

	handleDeviceTokenExchange := func(w http.ResponseWriter, r *http.Request, deviceCode string) {
		if deviceCode == "" {
			writeOAuthError(w, http.StatusBadRequest, "invalid_request", "device_code is required")
			return
		}
		dc, err := s.Store.DeviceCodeByDeviceCodeHash(auth.HashToken(deviceCode))
		if err != nil {
			writeOAuthError(w, http.StatusBadRequest, "invalid_grant", "unknown device_code")
			return
		}
		now := s.Clock.Now()
		if !dc.ExpiresAt.After(now) {
			writeOAuthError(w, http.StatusBadRequest, "expired_token", "device_code has expired")
			return
		}
		switch dc.Status {
		case store.DeviceStatusPending:
			writeOAuthError(w, http.StatusBadRequest, "authorization_pending", "authorization is still pending")
			return
		case store.DeviceStatusDenied:
			writeOAuthError(w, http.StatusBadRequest, "access_denied", "user denied the request")
			return
		case store.DeviceStatusConsumed:
			writeOAuthError(w, http.StatusBadRequest, "invalid_grant", "device_code already used")
			return
		}

		u, err := s.Store.UserByID(dc.UserID)
		if err != nil || !u.Active {
			writeOAuthError(w, http.StatusBadRequest, "invalid_grant", "user not active")
			return
		}
		if err := s.Store.ConsumeDeviceCode(dc.ID); err != nil {
			writeOAuthError(w, http.StatusInternalServerError, "server_error", err.Error())
			return
		}

		var scopes []string
		_ = json.Unmarshal([]byte(dc.Scopes), &scopes)
		if len(scopes) == 0 {
			scopes = []string{"tstamps.read", "tstamps.write"}
		}

		client, err := s.Store.Fosite().GetClient(r.Context(), dc.ClientID)
		if err != nil {
			writeOAuthError(w, http.StatusBadRequest, "invalid_client", "unknown client")
			return
		}

		sess := &store.OAuthSession{
			UserID:   u.ID,
			Username: u.Email,
			Subject:  fmt.Sprintf("%d", u.ID),
			ExpiresAt: map[fosite.TokenType]time.Time{
				fosite.AccessToken:  now.Add(time.Hour),
				fosite.RefreshToken: now.Add(30 * 24 * time.Hour),
			},
		}
		req := fosite.NewRequest()
		req.ID = dc.RequestID
		req.RequestedAt = now
		req.Client = client
		req.RequestedScope = fosite.Arguments(scopes)
		req.GrantedScope = fosite.Arguments(scopes)
		req.Session = sess

		accessToken, atSig, err := hmacStrategy.GenerateAccessToken(r.Context(), req)
		if err != nil {
			writeOAuthError(w, http.StatusInternalServerError, "server_error", err.Error())
			return
		}
		if err := s.Store.Fosite().CreateAccessTokenSession(r.Context(), atSig, req); err != nil {
			writeOAuthError(w, http.StatusInternalServerError, "server_error", err.Error())
			return
		}

		refreshToken, rtSig, err := hmacStrategy.GenerateRefreshToken(r.Context(), req)
		if err != nil {
			writeOAuthError(w, http.StatusInternalServerError, "server_error", err.Error())
			return
		}
		if err := s.Store.Fosite().CreateRefreshTokenSession(r.Context(), rtSig, atSig, req); err != nil {
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
	}

	apiMux.HandleFunc("POST "+apiPrefix+"/oauth/token", func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			writeOAuthError(w, http.StatusBadRequest, "invalid_request", "invalid form")
			return
		}
		if r.PostForm.Get("grant_type") == "urn:ietf:params:oauth:grant-type:device_code" {
			handleDeviceTokenExchange(w, r, r.PostForm.Get("device_code"))
			return
		}

		ctx := r.Context()
		now := s.Clock.Now()
		sess := &store.OAuthSession{
			ExpiresAt: map[fosite.TokenType]time.Time{
				fosite.AccessToken:  now.Add(time.Hour),
				fosite.RefreshToken: now.Add(30 * 24 * time.Hour),
			},
		}
		accessReq, err := oauthProvider.NewAccessRequest(ctx, r, sess)
		if err != nil {
			oauthProvider.WriteAccessError(ctx, w, accessReq, err)
			return
		}
		if accessReq.GetGrantTypes().ExactOne("authorization_code") {
			for _, sc := range accessReq.GetRequestedScopes() {
				if sc == "tstamps.read" || sc == "tstamps.write" {
					accessReq.GrantScope(sc)
				}
			}
			if len(accessReq.GetGrantedScopes()) == 0 {
				accessReq.GrantScope("tstamps.read")
				accessReq.GrantScope("tstamps.write")
			}
		}
		responder, err := oauthProvider.NewAccessResponse(ctx, accessReq)
		if err != nil {
			oauthProvider.WriteAccessError(ctx, w, accessReq, err)
			return
		}
		oauthProvider.WriteAccessResponse(ctx, w, accessReq, responder)
	})

	apiMux.HandleFunc("POST "+apiPrefix+"/oauth/revoke", func(w http.ResponseWriter, r *http.Request) {
		err := oauthProvider.NewRevocationRequest(r.Context(), r)
		oauthProvider.WriteRevocationResponse(r.Context(), w, err)
	})

	apiMux.HandleFunc("POST "+apiPrefix+"/auth/device", func(w http.ResponseWriter, r *http.Request) {
		var clientID, scopeStr string
		if strings.Contains(r.Header.Get("Content-Type"), "application/json") {
			var body struct {
				ClientID string `json:"client_id"`
				Scope    string `json:"scope"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			clientID = body.ClientID
			scopeStr = body.Scope
		} else {
			_ = r.ParseForm()
			clientID = r.FormValue("client_id")
			scopeStr = r.FormValue("scope")
		}
		if clientID == "" {
			clientID = store.BuiltInCLIClientID
		}
		scopes := strings.Fields(scopeStr)
		if len(scopes) == 0 {
			scopes = []string{"tstamps.read", "tstamps.write"}
		}

		deviceCode := "dc_" + randomHex(24)
		userCode := randomUserCode()
		reqID := "dev_" + randomHex(16)
		expiresIn := 600

		dc := &store.OAuthDeviceCode{
			DeviceCodeHash: auth.HashToken(deviceCode),
			UserCodeHash:   auth.HashToken(strings.ToUpper(strings.TrimSpace(userCode))),
			RequestID:      reqID,
			ClientID:       clientID,
			Status:         store.DeviceStatusPending,
			Scopes:         stringMustMarshal(scopes),
			SessionData:    "{}",
			ExpiresAt:      s.Clock.Now().Add(time.Duration(expiresIn) * time.Second),
		}
		if err := s.Store.CreateOAuthDeviceCode(dc); err != nil {
			writeProblem(w, internal(err))
			return
		}

		issuer := issuerURL(r)
		verURI := issuer + "/auth/device"
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"device_code":               deviceCode,
			"user_code":                 userCode,
			"verification_uri":          verURI,
			"verification_uri_complete": verURI + "?user_code=" + url.QueryEscape(userCode),
			"expires_in":                expiresIn,
			"interval":                  1,
		})
	})

	apiMux.HandleFunc("POST "+apiPrefix+"/auth/device/approve", func(w http.ResponseWriter, r *http.Request) {
		ri := requestFrom(r.Context())
		if ri == nil || ri.User == nil {
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

		code := strings.ToUpper(strings.TrimSpace(req.UserCode))
		if code == "" {
			writeProblem(w, problem(http.StatusBadRequest, urnInvalidInput, "user_code required"))
			return
		}

		status := store.DeviceStatusApproved
		if !req.Approve {
			status = store.DeviceStatusDenied
		}
		err := s.Store.ApproveDeviceCodeByUserCode(auth.HashToken(code), ri.User.ID, status)
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				writeProblem(w, problem(http.StatusNotFound, urnNotFound, "invalid or expired user code"))
				return
			}
			writeProblem(w, internal(err))
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "ok"})
	})

	apiMux.HandleFunc("POST "+apiPrefix+"/auth/device/token", func(w http.ResponseWriter, r *http.Request) {
		var deviceCode string
		if strings.Contains(r.Header.Get("Content-Type"), "application/json") {
			var body struct {
				DeviceCode string `json:"device_code"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			deviceCode = body.DeviceCode
		} else {
			_ = r.ParseForm()
			deviceCode = r.FormValue("device_code")
		}
		handleDeviceTokenExchange(w, r, deviceCode)
	})

	apiMux.HandleFunc("GET "+apiPrefix+"/oauth/grants", func(w http.ResponseWriter, r *http.Request) {
		ri := requestFrom(r.Context())
		if ri == nil || ri.User == nil {
			writeProblem(w, problem(http.StatusUnauthorized, urnUnauthorized, "authentication required"))
			return
		}
		grants, err := s.Store.OAuthGrantsOf(ri.User.ID)
		if err != nil {
			writeProblem(w, internal(err))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(grants)
	})

	apiMux.HandleFunc("DELETE "+apiPrefix+"/oauth/grants/{id}", func(w http.ResponseWriter, r *http.Request) {
		ri := requestFrom(r.Context())
		if ri == nil || ri.User == nil {
			writeProblem(w, problem(http.StatusUnauthorized, urnUnauthorized, "authentication required"))
			return
		}
		requestID := r.PathValue("id")
		if err := s.Store.RevokeOAuthGrant(ri.User.ID, requestID, s.Clock.Now()); err != nil {
			if errors.Is(err, store.ErrNotFound) {
				writeProblem(w, problem(http.StatusNotFound, urnNotFound, "grant not found"))
				return
			}
			writeProblem(w, internal(err))
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
}

func writeOAuthError(w http.ResponseWriter, status int, errCode, desc string) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"error":             errCode,
		"error_description": desc,
	})
}

func stringMustMarshal(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}
