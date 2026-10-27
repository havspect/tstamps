package gorm

import (
	"crypto/rand"
	"embed"
	"errors"
	"fmt"
	"sort"
	"strings"

	"gorm.io/gorm"
)

//go:embed migrations/*.sql
var migrationFS embed.FS

var allModels = []any{
	&Workspace{},
	&PendingSignup{},
	&ServerSecret{},
	&User{},
	&Session{},
	&APIToken{},
	&Team{},
	&TeamMembership{},
	&Project{},
	&Entry{},
	&AuditLog{},
	&OAuthClient{},
	&OAuthCode{},
	&OAuthAccessToken{},
	&OAuthRefreshToken{},
	&OAuthDeviceCode{},
}

// AllModels returns a slice of all GORM persistence models.
func AllModels() []any {
	return allModels
}

// Migrate runs AutoMigrate for all models, executes all embedded migration SQL scripts,
// and ensures the server-wide HMAC secret exists.
func Migrate(db *gorm.DB) error {
	if err := db.AutoMigrate(allModels...); err != nil {
		return fmt.Errorf("automigrate: %w", err)
	}

	entries, err := migrationFS.ReadDir("migrations")
	if err != nil {
		return fmt.Errorf("read migrations directory: %w", err)
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Name() < entries[j].Name()
	})

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}
		content, err := migrationFS.ReadFile("migrations/" + entry.Name())
		if err != nil {
			return fmt.Errorf("read migration %s: %w", entry.Name(), err)
		}
		sqlStr := strings.TrimSpace(string(content))
		if sqlStr == "" {
			continue
		}
		if err := db.Exec(sqlStr).Error; err != nil {
			return fmt.Errorf("exec migration %s: %w", entry.Name(), err)
		}
	}

	// Ensure server-wide HMAC secret exists (ID = 1)
	var secret ServerSecret
	if err := db.First(&secret, 1).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			buf := make([]byte, 32)
			if _, err := rand.Read(buf); err != nil {
				return fmt.Errorf("generate hmac secret: %w", err)
			}
			if err := db.Create(&ServerSecret{ID: 1, Secret: buf}).Error; err != nil {
				return fmt.Errorf("create server secret: %w", err)
			}
		} else {
			return fmt.Errorf("find server secret: %w", err)
		}
	}

	return nil
}
