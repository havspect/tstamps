package store

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"time"

	"github.com/ory/fosite"
	"gorm.io/gorm"
)

// Built-in OAuth clients
const (
	BuiltInCLIClientID    = "tstamps-cli"
	BuiltInMobileClientID = "tstamps-mobile"
)

// Device code statuses
const (
	DeviceStatusPending  = 0
	DeviceStatusApproved = 1
	DeviceStatusDenied   = 2
	DeviceStatusConsumed = 3
)

// OAuthSession implements fosite.Session
type OAuthSession struct {
	UserID    uint                           `json:"user_id"`
	Username  string                         `json:"username"`
	Subject   string                         `json:"subject"`
	ExpiresAt map[fosite.TokenType]time.Time `json:"expires_at"`
	Extra     map[string]interface{}         `json:"extra"`
}

func (s *OAuthSession) GetExpiresAt(key fosite.TokenType) time.Time {
	if s.ExpiresAt == nil {
		return time.Time{}
	}
	return s.ExpiresAt[key]
}

func (s *OAuthSession) SetExpiresAt(key fosite.TokenType, val time.Time) {
	if s.ExpiresAt == nil {
		s.ExpiresAt = make(map[fosite.TokenType]time.Time)
	}
	s.ExpiresAt[key] = val
}

func (s *OAuthSession) GetUsername() string {
	return s.Username
}

func (s *OAuthSession) SetUsername(username string) {
	s.Username = username
}

func (s *OAuthSession) GetSubject() string {
	return s.Subject
}

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
	if sess, ok := r.GetSession().(*OAuthSession); ok && sess != nil {
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

func (fs *FositeStore) deserializeRequester(ctx context.Context, raw string, fallbackClientID string, fallbackReqID string, userID uint, session fosite.Session) (*fosite.Request, error) {
	var sr storedRequest
	if err := json.Unmarshal([]byte(raw), &sr); err != nil || sr.ClientID == "" {
		// Fallback if raw was just OAuthSession JSON
		var sess OAuthSession
		_ = json.Unmarshal([]byte(raw), &sess)
		sess.UserID = userID
		if oas, ok := session.(*OAuthSession); ok && oas != nil {
			*oas = sess
		}
		client, err := fs.GetClient(ctx, fallbackClientID)
		if err != nil {
			return nil, err
		}
		req := fosite.NewRequest()
		req.ID = fallbackReqID
		req.Client = client
		req.Session = &sess
		return req, nil
	}

	var sess OAuthSession
	if len(sr.SessionJSON) > 0 {
		_ = json.Unmarshal(sr.SessionJSON, &sess)
	}
	if sess.UserID == 0 {
		sess.UserID = userID
	}
	if oas, ok := session.(*OAuthSession); ok && oas != nil {
		*oas = sess
	}

	client, err := fs.GetClient(ctx, sr.ClientID)
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

// FositeStore wraps *Store to implement fosite storage interfaces.
type FositeStore struct {
	store *Store
}

func (s *Store) Fosite() *FositeStore {
	return &FositeStore{store: s}
}

func (fs *FositeStore) dbFromContext(ctx context.Context) *gorm.DB {
	if tx, ok := ctx.Value(txKey{}).(*gorm.DB); ok {
		return tx.Session(&gorm.Session{})
	}
	return fs.store.scoped()
}

func (fs *FositeStore) rawDBFromContext(ctx context.Context) *gorm.DB {
	if tx, ok := ctx.Value(txKey{}).(*gorm.DB); ok {
		return tx.Session(&gorm.Session{})
	}
	return fs.store.db
}

type txKey struct{}

// Transactional interface implementation
func (fs *FositeStore) BeginTx(ctx context.Context) (context.Context, error) {
	if _, ok := ctx.Value(txKey{}).(*gorm.DB); ok {
		return ctx, nil
	}
	tx := fs.store.db.Begin()
	if tx.Error != nil {
		return ctx, tx.Error
	}
	return context.WithValue(ctx, txKey{}, tx), nil
}

func (fs *FositeStore) Commit(ctx context.Context) error {
	if tx, ok := ctx.Value(txKey{}).(*gorm.DB); ok {
		return tx.Commit().Error
	}
	return nil
}

func (fs *FositeStore) Rollback(ctx context.Context) error {
	if tx, ok := ctx.Value(txKey{}).(*gorm.DB); ok {
		return tx.Rollback().Error
	}
	return nil
}

// ClientManager
func (fs *FositeStore) GetClient(ctx context.Context, id string) (fosite.Client, error) {
	if id == BuiltInCLIClientID {
		return &fosite.DefaultClient{
			ID:            BuiltInCLIClientID,
			Secret:        []byte(""),
			RedirectURIs:  []string{"http://127.0.0.1/callback", "http://localhost/callback"},
			GrantTypes:    []string{"urn:ietf:params:oauth:grant-type:device_code", "refresh_token", "authorization_code"},
			ResponseTypes: []string{"code"},
			Scopes:        []string{"tstamps.read", "tstamps.write"},
			Public:        true,
		}, nil
	}
	if id == BuiltInMobileClientID {
		return &fosite.DefaultClient{
			ID:            BuiltInMobileClientID,
			Secret:        []byte(""),
			RedirectURIs:  []string{"tstamps://oauth/callback"},
			GrantTypes:    []string{"authorization_code", "refresh_token"},
			ResponseTypes: []string{"code"},
			Scopes:        []string{"tstamps.read", "tstamps.write"},
			Public:        true,
		}, nil
	}

	var client OAuthClient
	db := fs.dbFromContext(ctx)
	if err := db.Where("client_id = ?", id).First(&client).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fosite.ErrNotFound
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

func (fs *FositeStore) ClientAssertionJWTValid(ctx context.Context, jti string) error {
	return nil
}

func (fs *FositeStore) SetClientAssertionJWT(ctx context.Context, jti string, exp time.Time) error {
	return nil
}

// AuthorizeCodeStorage
func (fs *FositeStore) CreateAuthorizeCodeSession(ctx context.Context, codeSignature string, requester fosite.Requester) error {
	sessionData, userID, err := serializeRequester(requester)
	if err != nil {
		return err
	}

	pkceActive := false
	if form := requester.GetRequestForm(); form.Get("code_challenge") != "" {
		pkceActive = true
	}

	exp := requester.GetSession().GetExpiresAt(fosite.AuthorizeCode)
	if exp.IsZero() {
		exp = time.Now().Add(10 * time.Minute)
	}

	var existing OAuthCode
	if err := fs.dbFromContext(ctx).Where("signature_hash = ?", codeSignature).First(&existing).Error; err == nil {
		return fs.dbFromContext(ctx).Model(&OAuthCode{}).Where("id = ?", existing.ID).Updates(map[string]any{
			"session_data": sessionData,
			"pkce_active":  pkceActive || existing.PKCEActive,
			"expires_at":   exp,
		}).Error
	}

	code := OAuthCode{
		WorkspaceID:   fs.store.workspaceID,
		SignatureHash: codeSignature,
		RequestID:     requester.GetID(),
		ClientID:      requester.GetClient().GetID(),
		UserID:        userID,
		SessionData:   sessionData,
		PKCEActive:    pkceActive,
		Used:          false,
		ExpiresAt:     exp,
	}
	return fs.rawDBFromContext(ctx).Create(&code).Error
}

func (fs *FositeStore) GetAuthorizeCodeSession(ctx context.Context, codeSignature string, session fosite.Session) (fosite.Requester, error) {
	var code OAuthCode
	db := fs.dbFromContext(ctx)
	if err := db.Where("signature_hash = ?", codeSignature).First(&code).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fosite.ErrNotFound
		}
		return nil, err
	}

	req, err := fs.deserializeRequester(ctx, code.SessionData, code.ClientID, code.RequestID, code.UserID, session)
	if err != nil {
		return nil, err
	}

	if code.Used {
		return req, fosite.ErrInvalidatedAuthorizeCode
	}

	return req, nil
}

func (fs *FositeStore) InvalidateAuthorizeCodeSession(ctx context.Context, codeSignature string) error {
	db := fs.dbFromContext(ctx)
	return db.Model(&OAuthCode{}).Where("signature_hash = ?", codeSignature).Update("used", true).Error
}

// PKCERequestStorage
func (fs *FositeStore) CreatePKCERequestSession(ctx context.Context, codeSignature string, requester fosite.Requester) error {
	return fs.CreateAuthorizeCodeSession(ctx, "pkce:"+codeSignature, requester)
}

func (fs *FositeStore) GetPKCERequestSession(ctx context.Context, codeSignature string, session fosite.Session) (fosite.Requester, error) {
	return fs.GetAuthorizeCodeSession(ctx, "pkce:"+codeSignature, session)
}

func (fs *FositeStore) DeletePKCERequestSession(ctx context.Context, codeSignature string) error {
	return fs.dbFromContext(ctx).Where("signature_hash = ?", "pkce:"+codeSignature).Delete(&OAuthCode{}).Error
}

// AccessTokenStorage
func (fs *FositeStore) CreateAccessTokenSession(ctx context.Context, signature string, requester fosite.Requester) error {
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

	token := OAuthAccessToken{
		WorkspaceID:   fs.store.workspaceID,
		SignatureHash: signature,
		RequestID:     requester.GetID(),
		ClientID:      requester.GetClient().GetID(),
		UserID:        userID,
		Scopes:        string(scopesJSON),
		Audience:      string(audJSON),
		SessionData:   sessionData,
		ExpiresAt:     exp,
	}
	return fs.rawDBFromContext(ctx).Create(&token).Error
}

func (fs *FositeStore) GetAccessTokenSession(ctx context.Context, signature string, session fosite.Session) (fosite.Requester, error) {
	var token OAuthAccessToken
	db := fs.dbFromContext(ctx)
	if err := db.Where("signature_hash = ?", signature).First(&token).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fosite.ErrNotFound
		}
		return nil, err
	}

	req, err := fs.deserializeRequester(ctx, token.SessionData, token.ClientID, token.RequestID, token.UserID, session)
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
		return req, fosite.ErrInactiveToken
	}

	return req, nil
}

func (fs *FositeStore) DeleteAccessTokenSession(ctx context.Context, signature string) error {
	db := fs.dbFromContext(ctx)
	now := time.Now()
	return db.Model(&OAuthAccessToken{}).Where("signature_hash = ?", signature).Update("revoked_at", &now).Error
}

// RefreshTokenStorage
func (fs *FositeStore) CreateRefreshTokenSession(ctx context.Context, signature string, accessSignature string, requester fosite.Requester) error {
	sessionData, userID, err := serializeRequester(requester)
	if err != nil {
		return err
	}
	scopesJSON, _ := json.Marshal([]string(requester.GetGrantedScopes()))

	exp := requester.GetSession().GetExpiresAt(fosite.RefreshToken)
	if exp.IsZero() {
		exp = time.Now().Add(30 * 24 * time.Hour)
	}

	token := OAuthRefreshToken{
		WorkspaceID:         fs.store.workspaceID,
		SignatureHash:       signature,
		AccessSignatureHash: accessSignature,
		RequestID:           requester.GetID(),
		ClientID:            requester.GetClient().GetID(),
		UserID:              userID,
		Scopes:              string(scopesJSON),
		SessionData:         sessionData,
		Active:              true,
		ExpiresAt:           exp,
	}
	return fs.rawDBFromContext(ctx).Create(&token).Error
}

func (fs *FositeStore) GetRefreshTokenSession(ctx context.Context, signature string, session fosite.Session) (fosite.Requester, error) {
	var token OAuthRefreshToken
	db := fs.dbFromContext(ctx)
	if err := db.Where("signature_hash = ?", signature).First(&token).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fosite.ErrNotFound
		}
		return nil, err
	}

	req, err := fs.deserializeRequester(ctx, token.SessionData, token.ClientID, token.RequestID, token.UserID, session)
	if err != nil {
		return nil, err
	}
	var scopes []string
	if err := json.Unmarshal([]byte(token.Scopes), &scopes); err == nil && len(scopes) > 0 {
		req.GrantedScope = scopes
	}

	if !token.Active || token.RevokedAt != nil || time.Now().After(token.ExpiresAt) {
		return req, fosite.ErrInactiveToken
	}

	return req, nil
}

func (fs *FositeStore) DeleteRefreshTokenSession(ctx context.Context, signature string) error {
	db := fs.dbFromContext(ctx)
	now := time.Now()
	return db.Model(&OAuthRefreshToken{}).Where("signature_hash = ?", signature).Updates(map[string]any{
		"active":     false,
		"revoked_at": &now,
	}).Error
}

func (fs *FositeStore) RotateRefreshToken(ctx context.Context, requestID string, refreshTokenSignature string) error {
	now := time.Now()
	var rt OAuthRefreshToken
	if err := fs.dbFromContext(ctx).Where("signature_hash = ?", refreshTokenSignature).First(&rt).Error; err == nil && rt.AccessSignatureHash != "" {
		_ = fs.dbFromContext(ctx).Model(&OAuthAccessToken{}).Where("signature_hash = ?", rt.AccessSignatureHash).Update("revoked_at", &now).Error
	}
	return fs.dbFromContext(ctx).Model(&OAuthRefreshToken{}).Where("request_id = ? AND signature_hash = ?", requestID, refreshTokenSignature).Updates(map[string]any{
		"active":     false,
		"revoked_at": &now,
	}).Error
}

// TokenRevocationStorage
func (fs *FositeStore) RevokeRefreshToken(ctx context.Context, requestID string) error {
	db := fs.dbFromContext(ctx)
	now := time.Now()
	return db.Model(&OAuthRefreshToken{}).Where("request_id = ?", requestID).Updates(map[string]any{
		"active":     false,
		"revoked_at": &now,
	}).Error
}

func (fs *FositeStore) RevokeRefreshTokenMaybeGracePeriod(ctx context.Context, requestID string, signature string) error {
	return fs.RevokeRefreshToken(ctx, requestID)
}

func (fs *FositeStore) RevokeAccessToken(ctx context.Context, requestID string) error {
	db := fs.dbFromContext(ctx)
	now := time.Now()
	return db.Model(&OAuthAccessToken{}).Where("request_id = ?", requestID).Update("revoked_at", &now).Error
}

// Direct store helper methods

func (s *Store) CreateOAuthClient(c *OAuthClient) error {
	return s.db.Create(c).Error
}

func (s *Store) OAuthClientByID(clientID string) (*OAuthClient, error) {
	var c OAuthClient
	if err := s.scoped().Where("client_id = ?", clientID).First(&c).Error; err != nil {
		return nil, translate(err)
	}
	return &c, nil
}

func (s *Store) OAuthAccessTokenBySignatureHash(hash string) (*OAuthAccessToken, error) {
	var at OAuthAccessToken
	if err := s.scoped().Where("signature_hash = ?", hash).First(&at).Error; err != nil {
		return nil, translate(err)
	}
	return &at, nil
}

func (s *Store) TouchOAuthAccessToken(id uint, at time.Time) error {
	return s.scoped().Model(&OAuthAccessToken{}).Where("id = ?", id).Update("last_used_at", &at).Error
}

func (s *Store) CreateOAuthDeviceCode(dc *OAuthDeviceCode) error {
	return s.db.Create(dc).Error
}

func (s *Store) DeviceCodeByDeviceCodeHash(hash string) (*OAuthDeviceCode, error) {
	var dc OAuthDeviceCode
	if err := s.scoped().Where("device_code_hash = ?", hash).First(&dc).Error; err != nil {
		return nil, translate(err)
	}
	return &dc, nil
}

func (s *Store) ApproveDeviceCodeByUserCode(userCodeHash string, userID uint, status int) error {
	res := s.scoped().Model(&OAuthDeviceCode{}).
		Where("user_code_hash = ? AND status = ? AND expires_at > ?", userCodeHash, DeviceStatusPending, time.Now()).
		Updates(map[string]any{
			"user_id": userID,
			"status":  status,
		})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) ConsumeDeviceCode(id uint) error {
	return s.scoped().Model(&OAuthDeviceCode{}).Where("id = ?", id).Update("status", DeviceStatusConsumed).Error
}

type OAuthGrant struct {
	RequestID  string     `json:"id"`
	ClientID   string     `json:"client_id"`
	ClientName string     `json:"client_name"`
	Scopes     []string   `json:"scopes"`
	CreatedAt  time.Time  `json:"created_at"`
	LastUsedAt *time.Time `json:"last_used_at,omitempty"`
	RevokedAt  *time.Time `json:"revoked_at,omitempty"`
}

func (s *Store) OAuthGrantsOf(userID uint) ([]OAuthGrant, error) {
	var tokens []OAuthAccessToken
	if err := s.scoped().Where("user_id = ?", userID).Order("created_at DESC").Find(&tokens).Error; err != nil {
		return nil, err
	}

	seen := make(map[string]bool)
	grants := []OAuthGrant{}
	for _, t := range tokens {
		if seen[t.RequestID] {
			continue
		}
		seen[t.RequestID] = true

		var scopes []string
		_ = json.Unmarshal([]byte(t.Scopes), &scopes)

		clientName := t.ClientID
		var c OAuthClient
		if err := s.scoped().Where("client_id = ?", t.ClientID).First(&c).Error; err == nil && c.ClientName != "" {
			clientName = c.ClientName
		} else if t.ClientID == BuiltInCLIClientID {
			clientName = "Tstamps CLI"
		} else if t.ClientID == BuiltInMobileClientID {
			clientName = "Tstamps Mobile"
		}

		grants = append(grants, OAuthGrant{
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

func (s *Store) RevokeOAuthGrant(userID uint, requestID string, at time.Time) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		res1 := tx.Model(&OAuthAccessToken{}).Where("user_id = ? AND request_id = ? AND revoked_at IS NULL", userID, requestID).Update("revoked_at", &at)
		if res1.Error != nil {
			return res1.Error
		}
		res2 := tx.Model(&OAuthRefreshToken{}).Where("user_id = ? AND request_id = ? AND revoked_at IS NULL", userID, requestID).Updates(map[string]any{
			"active":     false,
			"revoked_at": &at,
		})
		if res2.Error != nil {
			return res2.Error
		}
		if res1.RowsAffected == 0 && res2.RowsAffected == 0 {
			return ErrNotFound
		}
		return nil
	})
}
