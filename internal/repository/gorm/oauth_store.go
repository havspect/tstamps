package gorm

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"time"

	"github.com/ory/fosite"
	"gorm.io/gorm"

	"github.com/havspect/tstamps/internal/oauth"
)

var _ oauth.Storage = (*OAuthStore)(nil)

// OAuthStore implements oauth.Storage over GORM without raw SQL.
type OAuthStore struct {
	db          *gorm.DB
	workspaceID uint
}

// NewOAuthStore constructs an OAuthStore scoped to the given workspace.
func NewOAuthStore(db *gorm.DB, workspaceID uint) *OAuthStore {
	return &OAuthStore{
		db:          db,
		workspaceID: workspaceID,
	}
}

// ForWorkspace returns a copy of OAuthStore scoped to the specified workspace ID.
func (s *OAuthStore) ForWorkspace(workspaceID uint) *OAuthStore {
	return &OAuthStore{
		db:          s.db,
		workspaceID: workspaceID,
	}
}

func (s *OAuthStore) getWorkspaceID(ctx context.Context) (uint, error) {
	if s.workspaceID > 0 {
		return s.workspaceID, nil
	}
	if id, ok := WorkspaceIDFrom(ctx); ok && id > 0 {
		return id, nil
	}
	return 0, errors.New("workspace ID required for OAuth operation")
}

func (s *OAuthStore) dbFrom(ctx context.Context) *gorm.DB {
	wsID := s.workspaceID
	if ctxWsID, ok := WorkspaceIDFrom(ctx); ok && ctxWsID > 0 {
		wsID = ctxWsID
	}
	base := DBFrom(ctx, s.db)
	if wsID > 0 {
		return base.WithContext(WithWorkspaceID(ctx, wsID))
	}
	return base.WithContext(ctx)
}

type storedRequest struct {
	ID                string          `json:"id"`
	RequestedAt       time.Time       `json:"requested_at"`
	ClientID          string          `json:"client_id"`
	RequestedScopes   []string        `json:"requested_scopes"`
	GrantedScopes     []string        `json:"granted_scopes"`
	RequestedAudience []string        `json:"requested_audience"`
	GrantedAudience   []string        `json:"granted_audience"`
	Form              url.Values      `json:"form"`
	SessionJSON       json.RawMessage `json:"session"`
}

func serializeRequester(r fosite.Requester) (string, uint, error) {
	var userID uint
	if sess, ok := r.GetSession().(*oauth.OAuthSession); ok && sess != nil {
		userID = sess.UserID
	}
	sessBytes, err := json.Marshal(r.GetSession())
	if err != nil {
		return "", 0, err
	}
	clientID := ""
	if r.GetClient() != nil {
		clientID = r.GetClient().GetID()
	}
	sr := storedRequest{
		ID:                r.GetID(),
		RequestedAt:       r.GetRequestedAt(),
		ClientID:          clientID,
		RequestedScopes:   r.GetRequestedScopes(),
		GrantedScopes:     r.GetGrantedScopes(),
		RequestedAudience: r.GetRequestedAudience(),
		GrantedAudience:   r.GetGrantedAudience(),
		Form:              r.GetRequestForm(),
		SessionJSON:       sessBytes,
	}
	b, err := json.Marshal(sr)
	if err != nil {
		return "", 0, err
	}
	return string(b), userID, nil
}

func (s *OAuthStore) deserializeRequester(ctx context.Context, raw string, fallbackClientID string, fallbackReqID string, userID uint, session fosite.Session) (*fosite.Request, error) {
	var sr storedRequest
	if err := json.Unmarshal([]byte(raw), &sr); err != nil || sr.ClientID == "" {
		// Fallback if raw was only session JSON
		var sess oauth.OAuthSession
		_ = json.Unmarshal([]byte(raw), &sess)
		sess.UserID = userID
		if oas, ok := session.(*oauth.OAuthSession); ok && oas != nil {
			*oas = sess
		}
		client, err := s.GetClient(ctx, fallbackClientID)
		if err != nil {
			return nil, err
		}
		req := fosite.NewRequest()
		req.ID = fallbackReqID
		req.Client = client
		req.Session = &sess
		return req, nil
	}

	var sess oauth.OAuthSession
	if len(sr.SessionJSON) > 0 {
		_ = json.Unmarshal(sr.SessionJSON, &sess)
	}
	if sess.UserID == 0 {
		sess.UserID = userID
	}
	if oas, ok := session.(*oauth.OAuthSession); ok && oas != nil {
		*oas = sess
	}

	client, err := s.GetClient(ctx, sr.ClientID)
	if err != nil {
		return nil, err
	}

	req := fosite.NewRequest()
	req.ID = sr.ID
	if !sr.RequestedAt.IsZero() {
		req.RequestedAt = sr.RequestedAt
	}
	req.Client = client
	req.RequestedScope = fosite.Arguments(sr.RequestedScopes)
	req.GrantedScope = fosite.Arguments(sr.GrantedScopes)
	req.RequestedAudience = fosite.Arguments(sr.RequestedAudience)
	req.GrantedAudience = fosite.Arguments(sr.GrantedAudience)
	if sr.Form != nil {
		req.Form = sr.Form
	}
	if session != nil {
		req.Session = session
	} else {
		req.Session = &sess
	}
	return req, nil
}

// Transactional interface implementation
func (s *OAuthStore) BeginTX(ctx context.Context) (context.Context, error) {
	return s.BeginTx(ctx)
}

func (s *OAuthStore) BeginTx(ctx context.Context) (context.Context, error) {
	if ctx != nil {
		if _, ok := ctx.Value(txKey{}).(*gorm.DB); ok {
			return ctx, nil
		}
	}
	tx := s.db.Begin()
	if tx.Error != nil {
		return ctx, tx.Error
	}
	return context.WithValue(ctx, txKey{}, tx), nil
}

func (s *OAuthStore) Commit(ctx context.Context) error {
	if ctx != nil {
		if tx, ok := ctx.Value(txKey{}).(*gorm.DB); ok && tx != nil {
			return tx.Commit().Error
		}
	}
	return nil
}

func (s *OAuthStore) Rollback(ctx context.Context) error {
	if ctx != nil {
		if tx, ok := ctx.Value(txKey{}).(*gorm.DB); ok && tx != nil {
			return tx.Rollback().Error
		}
	}
	return nil
}

// ClientManager
func (s *OAuthStore) GetClient(ctx context.Context, id string) (fosite.Client, error) {
	if id == oauth.BuiltInCLIClientID {
		return &fosite.DefaultClient{
			ID:            oauth.BuiltInCLIClientID,
			Secret:        []byte(""),
			RedirectURIs:  []string{"http://127.0.0.1/callback", "http://localhost/callback"},
			GrantTypes:    []string{"urn:ietf:params:oauth:grant-type:device_code", "refresh_token", "authorization_code"},
			ResponseTypes: []string{"code"},
			Scopes:        []string{"tstamps.read", "tstamps.write"},
			Public:        true,
		}, nil
	}
	if id == oauth.BuiltInMobileClientID {
		return &fosite.DefaultClient{
			ID:            oauth.BuiltInMobileClientID,
			Secret:        []byte(""),
			RedirectURIs:  []string{"tstamps://oauth/callback"},
			GrantTypes:    []string{"authorization_code", "refresh_token"},
			ResponseTypes: []string{"code"},
			Scopes:        []string{"tstamps.read", "tstamps.write"},
			Public:        true,
		}, nil
	}

	var client OAuthClient
	if err := s.dbFrom(ctx).Where("client_id = ?", id).First(&client).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, oauth.ErrNotFound
		}
		return nil, err
	}

	var redirectURIs, grantTypes, responseTypes, scopes []string
	_ = json.Unmarshal([]byte(client.RedirectURIs), &redirectURIs)
	_ = json.Unmarshal([]byte(client.GrantTypes), &grantTypes)
	_ = json.Unmarshal([]byte(client.ResponseTypes), &responseTypes)
	_ = json.Unmarshal([]byte(client.Scopes), &scopes)

	var secret []byte
	if client.ClientSecretHash != "" {
		secret = []byte(client.ClientSecretHash)
	}

	return &fosite.DefaultClient{
		ID:            client.ClientID,
		Secret:        secret,
		RedirectURIs:  redirectURIs,
		GrantTypes:    grantTypes,
		ResponseTypes: responseTypes,
		Scopes:        scopes,
		Public:        client.Public,
	}, nil
}

func (s *OAuthStore) ClientAssertionJWTValid(_ context.Context, _ string) error {
	return nil
}

func (s *OAuthStore) SetClientAssertionJWT(_ context.Context, _ string, _ time.Time) error {
	return nil
}

// AuthorizeCodeStorage
func (s *OAuthStore) CreateAuthorizeCodeSession(ctx context.Context, codeSignature string, requester fosite.Requester) error {
	wsID, err := s.getWorkspaceID(ctx)
	if err != nil {
		return err
	}
	sessionData, userID, err := serializeRequester(requester)
	if err != nil {
		return err
	}

	pkceActive := false
	if form := requester.GetRequestForm(); form != nil && form.Get("code_challenge") != "" {
		pkceActive = true
	}

	exp := requester.GetSession().GetExpiresAt(fosite.AuthorizeCode)
	if exp.IsZero() {
		exp = time.Now().Add(10 * time.Minute)
	}

	db := s.dbFrom(ctx)
	var existing OAuthCode
	if err := db.Where("signature_hash = ?", codeSignature).First(&existing).Error; err == nil {
		return db.Model(&OAuthCode{}).Where("id = ?", existing.ID).Updates(map[string]any{
			"session_data": sessionData,
			"pkce_active":  pkceActive || existing.PKCEActive,
			"expires_at":   exp,
		}).Error
	}

	clientID := ""
	if requester.GetClient() != nil {
		clientID = requester.GetClient().GetID()
	}

	code := OAuthCode{
		WorkspaceID:   wsID,
		SignatureHash: codeSignature,
		RequestID:     requester.GetID(),
		ClientID:      clientID,
		UserID:        userID,
		SessionData:   sessionData,
		PKCEActive:    pkceActive,
		Used:          false,
		ExpiresAt:     exp,
	}
	return db.Create(&code).Error
}

func (s *OAuthStore) GetAuthorizeCodeSession(ctx context.Context, codeSignature string, session fosite.Session) (fosite.Requester, error) {
	var code OAuthCode
	db := s.dbFrom(ctx)
	if err := db.Where("signature_hash = ?", codeSignature).First(&code).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, oauth.ErrNotFound
		}
		return nil, err
	}

	req, err := s.deserializeRequester(ctx, code.SessionData, code.ClientID, code.RequestID, code.UserID, session)
	if err != nil {
		return nil, err
	}

	if code.Used {
		return req, oauth.ErrInvalidatedAuthorizeCode
	}

	return req, nil
}

func (s *OAuthStore) InvalidateAuthorizeCodeSession(ctx context.Context, codeSignature string) error {
	db := s.dbFrom(ctx)
	return db.Model(&OAuthCode{}).Where("signature_hash = ?", codeSignature).Update("used", true).Error
}

// PKCERequestStorage
func (s *OAuthStore) CreatePKCERequestSession(ctx context.Context, codeSignature string, requester fosite.Requester) error {
	return s.CreateAuthorizeCodeSession(ctx, "pkce:"+codeSignature, requester)
}

func (s *OAuthStore) GetPKCERequestSession(ctx context.Context, codeSignature string, session fosite.Session) (fosite.Requester, error) {
	return s.GetAuthorizeCodeSession(ctx, "pkce:"+codeSignature, session)
}

func (s *OAuthStore) DeletePKCERequestSession(ctx context.Context, codeSignature string) error {
	return s.dbFrom(ctx).Where("signature_hash = ?", "pkce:"+codeSignature).Delete(&OAuthCode{}).Error
}

// AccessTokenStorage
func (s *OAuthStore) CreateAccessTokenSession(ctx context.Context, signature string, requester fosite.Requester) error {
	wsID, err := s.getWorkspaceID(ctx)
	if err != nil {
		return err
	}
	sessionData, userID, err := serializeRequester(requester)
	if err != nil {
		return err
	}
	scopesJSON, _ := json.Marshal([]string(requester.GetGrantedScopes()))
	audJSON, _ := json.Marshal([]string(requester.GetGrantedAudience()))

	exp := requester.GetSession().GetExpiresAt(fosite.AccessToken)
	if exp.IsZero() {
		exp = time.Now().Add(1 * time.Hour)
	}

	clientID := ""
	if requester.GetClient() != nil {
		clientID = requester.GetClient().GetID()
	}

	token := OAuthAccessToken{
		WorkspaceID:   wsID,
		SignatureHash: signature,
		RequestID:     requester.GetID(),
		ClientID:      clientID,
		UserID:        userID,
		Scopes:        string(scopesJSON),
		Audience:      string(audJSON),
		SessionData:   sessionData,
		ExpiresAt:     exp,
	}
	return s.dbFrom(ctx).Create(&token).Error
}

func (s *OAuthStore) GetAccessTokenSession(ctx context.Context, signature string, session fosite.Session) (fosite.Requester, error) {
	var token OAuthAccessToken
	db := s.dbFrom(ctx)
	if err := db.Where("signature_hash = ?", signature).First(&token).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, oauth.ErrNotFound
		}
		return nil, err
	}

	req, err := s.deserializeRequester(ctx, token.SessionData, token.ClientID, token.RequestID, token.UserID, session)
	if err != nil {
		return nil, err
	}
	var scopes, aud []string
	if err := json.Unmarshal([]byte(token.Scopes), &scopes); err == nil && len(scopes) > 0 {
		req.GrantedScope = scopes
	}
	if err := json.Unmarshal([]byte(token.Audience), &aud); err == nil && len(aud) > 0 {
		req.GrantedAudience = aud
	}

	if token.RevokedAt != nil || time.Now().After(token.ExpiresAt) {
		return req, oauth.ErrInactiveToken
	}

	return req, nil
}

func (s *OAuthStore) DeleteAccessTokenSession(ctx context.Context, signature string) error {
	db := s.dbFrom(ctx)
	now := time.Now()
	return db.Model(&OAuthAccessToken{}).Where("signature_hash = ?", signature).Update("revoked_at", &now).Error
}

// RefreshTokenStorage
func (s *OAuthStore) CreateRefreshTokenSession(ctx context.Context, signature string, accessSignature string, requester fosite.Requester) error {
	wsID, err := s.getWorkspaceID(ctx)
	if err != nil {
		return err
	}
	sessionData, userID, err := serializeRequester(requester)
	if err != nil {
		return err
	}
	scopesJSON, _ := json.Marshal([]string(requester.GetGrantedScopes()))

	exp := requester.GetSession().GetExpiresAt(fosite.RefreshToken)
	if exp.IsZero() {
		exp = time.Now().Add(30 * 24 * time.Hour)
	}

	clientID := ""
	if requester.GetClient() != nil {
		clientID = requester.GetClient().GetID()
	}

	token := OAuthRefreshToken{
		WorkspaceID:         wsID,
		SignatureHash:       signature,
		AccessSignatureHash: accessSignature,
		RequestID:           requester.GetID(),
		ClientID:            clientID,
		UserID:              userID,
		Scopes:              string(scopesJSON),
		SessionData:         sessionData,
		Active:              true,
		ExpiresAt:           exp,
	}
	return s.dbFrom(ctx).Create(&token).Error
}

func (s *OAuthStore) GetRefreshTokenSession(ctx context.Context, signature string, session fosite.Session) (fosite.Requester, error) {
	var token OAuthRefreshToken
	db := s.dbFrom(ctx)
	if err := db.Where("signature_hash = ?", signature).First(&token).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, oauth.ErrNotFound
		}
		return nil, err
	}

	req, err := s.deserializeRequester(ctx, token.SessionData, token.ClientID, token.RequestID, token.UserID, session)
	if err != nil {
		return nil, err
	}
	var scopes []string
	if err := json.Unmarshal([]byte(token.Scopes), &scopes); err == nil && len(scopes) > 0 {
		req.GrantedScope = scopes
	}

	if !token.Active || token.RevokedAt != nil || time.Now().After(token.ExpiresAt) {
		return req, oauth.ErrInactiveToken
	}

	return req, nil
}

func (s *OAuthStore) DeleteRefreshTokenSession(ctx context.Context, signature string) error {
	db := s.dbFrom(ctx)
	now := time.Now()
	return db.Model(&OAuthRefreshToken{}).Where("signature_hash = ?", signature).Updates(map[string]any{
		"active":     false,
		"revoked_at": &now,
	}).Error
}

func (s *OAuthStore) RotateRefreshToken(ctx context.Context, requestID string, refreshTokenSignature string) error {
	now := time.Now()
	db := s.dbFrom(ctx)
	var rt OAuthRefreshToken
	if err := db.Where("signature_hash = ?", refreshTokenSignature).First(&rt).Error; err == nil && rt.AccessSignatureHash != "" {
		_ = db.Model(&OAuthAccessToken{}).Where("signature_hash = ?", rt.AccessSignatureHash).Update("revoked_at", &now).Error
	}
	return db.Model(&OAuthRefreshToken{}).Where("request_id = ? AND signature_hash = ?", requestID, refreshTokenSignature).Updates(map[string]any{
		"active":     false,
		"revoked_at": &now,
	}).Error
}

// TokenRevocationStorage
func (s *OAuthStore) RevokeRefreshToken(ctx context.Context, requestID string) error {
	db := s.dbFrom(ctx)
	now := time.Now()
	return db.Model(&OAuthRefreshToken{}).Where("request_id = ?", requestID).Updates(map[string]any{
		"active":     false,
		"revoked_at": &now,
	}).Error
}

func (s *OAuthStore) RevokeRefreshTokenMaybeGracePeriod(ctx context.Context, requestID string, _ string) error {
	return s.RevokeRefreshToken(ctx, requestID)
}

func (s *OAuthStore) RevokeAccessToken(ctx context.Context, requestID string) error {
	db := s.dbFrom(ctx)
	now := time.Now()
	return db.Model(&OAuthAccessToken{}).Where("request_id = ?", requestID).Update("revoked_at", &now).Error
}

// Direct client persistence
func (s *OAuthStore) CreateClient(ctx context.Context, client *oauth.Client) error {
	wsID, err := s.getWorkspaceID(ctx)
	if err != nil {
		return err
	}
	redirJSON, _ := json.Marshal(client.RedirectURIs)
	grantJSON, _ := json.Marshal(client.GrantTypes)
	respJSON, _ := json.Marshal(client.ResponseTypes)
	scopesJSON, _ := json.Marshal(client.Scopes)

	model := OAuthClient{
		WorkspaceID:      wsID,
		ClientID:         client.ClientID,
		ClientName:       client.ClientName,
		ClientSecretHash: client.ClientSecretHash,
		RedirectURIs:     string(redirJSON),
		GrantTypes:       string(grantJSON),
		ResponseTypes:    string(respJSON),
		Scopes:           string(scopesJSON),
		Public:           client.Public,
	}
	if err := s.dbFrom(ctx).Create(&model).Error; err != nil {
		return err
	}
	client.ID = model.ID
	client.WorkspaceID = model.WorkspaceID
	client.CreatedAt = model.CreatedAt
	return nil
}

func (s *OAuthStore) GetClientByID(ctx context.Context, clientID string) (*oauth.Client, error) {
	var model OAuthClient
	if err := s.dbFrom(ctx).Where("client_id = ?", clientID).First(&model).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, oauth.ErrNotFound
		}
		return nil, err
	}
	var redirectURIs, grantTypes, responseTypes, scopes []string
	_ = json.Unmarshal([]byte(model.RedirectURIs), &redirectURIs)
	_ = json.Unmarshal([]byte(model.GrantTypes), &grantTypes)
	_ = json.Unmarshal([]byte(model.ResponseTypes), &responseTypes)
	_ = json.Unmarshal([]byte(model.Scopes), &scopes)

	return &oauth.Client{
		ID:               model.ID,
		WorkspaceID:      model.WorkspaceID,
		ClientID:         model.ClientID,
		ClientName:       model.ClientName,
		ClientSecretHash: model.ClientSecretHash,
		RedirectURIs:     redirectURIs,
		GrantTypes:       grantTypes,
		ResponseTypes:    responseTypes,
		Scopes:           scopes,
		Public:           model.Public,
		CreatedAt:        model.CreatedAt,
	}, nil
}

// DeviceCodeStorage
func (s *OAuthStore) CreateDeviceCode(ctx context.Context, dc *oauth.DeviceCode) error {
	wsID, err := s.getWorkspaceID(ctx)
	if err != nil {
		return err
	}
	scopesJSON, _ := json.Marshal(dc.Scopes)
	model := OAuthDeviceCode{
		WorkspaceID:    wsID,
		DeviceCodeHash: dc.DeviceCodeHash,
		UserCodeHash:   dc.UserCodeHash,
		RequestID:      dc.RequestID,
		ClientID:       dc.ClientID,
		UserID:         dc.UserID,
		Status:         dc.Status,
		Scopes:         string(scopesJSON),
		SessionData:    dc.SessionData,
		ExpiresAt:      dc.ExpiresAt,
		LastCheckedAt:  dc.LastCheckedAt,
	}
	if err := s.dbFrom(ctx).Create(&model).Error; err != nil {
		return err
	}
	dc.ID = model.ID
	dc.CreatedAt = model.CreatedAt
	return nil
}

func (s *OAuthStore) GetDeviceCode(ctx context.Context, deviceCodeHash string) (*oauth.DeviceCode, error) {
	var model OAuthDeviceCode
	if err := s.dbFrom(ctx).Where("device_code_hash = ?", deviceCodeHash).First(&model).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, oauth.ErrNotFound
		}
		return nil, err
	}
	var scopes []string
	_ = json.Unmarshal([]byte(model.Scopes), &scopes)
	return &oauth.DeviceCode{
		ID:             model.ID,
		WorkspaceID:    model.WorkspaceID,
		DeviceCodeHash: model.DeviceCodeHash,
		UserCodeHash:   model.UserCodeHash,
		RequestID:      model.RequestID,
		ClientID:       model.ClientID,
		UserID:         model.UserID,
		Status:         model.Status,
		Scopes:         scopes,
		SessionData:    model.SessionData,
		ExpiresAt:      model.ExpiresAt,
		LastCheckedAt:  model.LastCheckedAt,
		CreatedAt:      model.CreatedAt,
	}, nil
}

func (s *OAuthStore) ApproveDeviceCode(ctx context.Context, userCodeHash string, userID uint, status int) error {
	res := s.dbFrom(ctx).Model(&OAuthDeviceCode{}).
		Where("user_code_hash = ? AND status = ? AND expires_at > ?", userCodeHash, oauth.DeviceStatusPending, time.Now()).
		Updates(map[string]any{
			"user_id": userID,
			"status":  status,
		})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return oauth.ErrNotFound
	}
	return nil
}

func (s *OAuthStore) ConsumeDeviceCode(ctx context.Context, id uint) error {
	return s.dbFrom(ctx).Model(&OAuthDeviceCode{}).Where("id = ?", id).Update("status", oauth.DeviceStatusConsumed).Error
}

// User grant queries and revocation
func (s *OAuthStore) GetGrants(ctx context.Context, userID uint) ([]oauth.Grant, error) {
	var tokens []OAuthAccessToken
	db := s.dbFrom(ctx)
	if err := db.Where("user_id = ?", userID).Order("created_at DESC").Find(&tokens).Error; err != nil {
		return nil, err
	}

	seen := make(map[string]bool)
	var grants []oauth.Grant
	for _, t := range tokens {
		if seen[t.RequestID] {
			continue
		}
		seen[t.RequestID] = true

		var scopes []string
		_ = json.Unmarshal([]byte(t.Scopes), &scopes)

		clientName := t.ClientID
		var c OAuthClient
		if err := db.Where("client_id = ?", t.ClientID).First(&c).Error; err == nil && c.ClientName != "" {
			clientName = c.ClientName
		} else if t.ClientID == oauth.BuiltInCLIClientID {
			clientName = "Tstamps CLI"
		} else if t.ClientID == oauth.BuiltInMobileClientID {
			clientName = "Tstamps Mobile"
		}

		grants = append(grants, oauth.Grant{
			RequestID:  t.RequestID,
			ClientID:   t.ClientID,
			ClientName: clientName,
			Scopes:     scopes,
			CreatedAt:  t.CreatedAt,
			LastUsedAt: t.LastUsedAt,
			RevokedAt:  t.RevokedAt,
		})
	}
	return grants, nil
}

func (s *OAuthStore) RevokeGrant(ctx context.Context, userID uint, requestID string, at time.Time) error {
	db := s.dbFrom(ctx)
	return db.Transaction(func(tx *gorm.DB) error {
		res1 := tx.Model(&OAuthAccessToken{}).
			Where("user_id = ? AND request_id = ? AND revoked_at IS NULL", userID, requestID).
			Update("revoked_at", &at)
		if res1.Error != nil {
			return res1.Error
		}
		res2 := tx.Model(&OAuthRefreshToken{}).
			Where("user_id = ? AND request_id = ? AND revoked_at IS NULL", userID, requestID).
			Updates(map[string]any{
				"active":     false,
				"revoked_at": &at,
			})
		if res2.Error != nil {
			return res2.Error
		}
		if res1.RowsAffected == 0 && res2.RowsAffected == 0 {
			return oauth.ErrNotFound
		}
		return nil
	})
}

// OAuthAccessTokenBySignatureHash returns an access token record by its signature hash.
func (s *OAuthStore) OAuthAccessTokenBySignatureHash(ctx context.Context, hash string) (*OAuthAccessToken, error) {
	var at OAuthAccessToken
	if err := s.dbFrom(ctx).Where("signature_hash = ?", hash).First(&at).Error; err != nil {
		return nil, err
	}
	return &at, nil
}

// TouchOAuthAccessToken updates the last_used_at timestamp on an access token.
func (s *OAuthStore) TouchOAuthAccessToken(ctx context.Context, id uint, at time.Time) error {
	return s.dbFrom(ctx).Model(&OAuthAccessToken{}).Where("id = ?", id).Update("last_used_at", &at).Error
}
