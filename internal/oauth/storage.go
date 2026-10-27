package oauth

import (
	"context"
	"time"

	"github.com/ory/fosite"
	"github.com/ory/fosite/handler/oauth2"
	"github.com/ory/fosite/handler/pkce"
	fosstorage "github.com/ory/fosite/storage"
)

// Transactional defines transaction boundaries for storage operations.
type Transactional interface {
	fosstorage.Transactional
	BeginTx(ctx context.Context) (context.Context, error)
}

// DeviceCodeStorage handles persistence of RFC 8628 device authorization requests.
type DeviceCodeStorage interface {
	CreateDeviceCode(ctx context.Context, dc *DeviceCode) error
	GetDeviceCode(ctx context.Context, deviceCodeHash string) (*DeviceCode, error)
	ApproveDeviceCode(ctx context.Context, userCodeHash string, userID uint, status int) error
	ConsumeDeviceCode(ctx context.Context, id uint) error
}

// Storage represents the comprehensive persistence layer required by the OAuth provider.
type Storage interface {
	fosite.Storage
	oauth2.CoreStorage
	pkce.PKCERequestStorage
	oauth2.TokenRevocationStorage
	Transactional
	DeviceCodeStorage

	// Direct client persistence
	CreateClient(ctx context.Context, client *Client) error
	GetClientByID(ctx context.Context, clientID string) (*Client, error)

	// User grant queries and revocation
	GetGrants(ctx context.Context, userID uint) ([]Grant, error)
	RevokeGrant(ctx context.Context, userID uint, requestID string, at time.Time) error
}
