package gorm

import (
	"os"
	"strings"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// Open opens a SQLite database at the specified path with default pragmas,
// secure file permissions, and tenancy enforcement callbacks.
func Open(path string) (*gorm.DB, error) {
	dsn := path
	if !strings.Contains(path, "?") {
		if path != ":memory:" {
			// Ensure file has 0600 permissions upon creation.
			f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY, 0o600)
			if err != nil {
				return nil, err
			}
			if err := f.Close(); err != nil {
				return nil, err
			}
		}
		dsn = path + "?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(ON)&_pragma=synchronous(NORMAL)&_txlock=immediate"
	}

	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{
		Logger:         logger.Default.LogMode(logger.Silent),
		TranslateError: true,
	})
	if err != nil {
		return nil, err
	}

	RegisterTenancyCallbacks(db)
	return db, nil
}
