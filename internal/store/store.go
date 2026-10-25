// Package store persists users and sessions in SQLite through GORM.
package store

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// SchemaVersion is the model set this binary understands. It is stored in the
// database's PRAGMA user_version so /healthz reports what is on disk, not what
// the binary hopes is there, and an older binary refuses a newer database.
const SchemaVersion = 5

// ErrNotFound is returned when a lookup matches no row.
var ErrNotFound = errors.New("not found")

// ErrDuplicateEmail is returned by CreateUser when the email is already taken.
var ErrDuplicateEmail = errors.New("duplicate email")

// Store is a handle to the SQLite database.
type Store struct {
	db *gorm.DB
}

// Open opens the SQLite database at path, creating the file 0600 when it does not exist.
// File databases get WAL and a 10s busy timeout, so a second writer waits instead of
// failing with "database is locked" immediately.
func Open(path string) (*Store, error) {
	dsn := path
	// A DSN with options is not a plain file, and :memory: has none to protect.
	if path != ":memory:" && !strings.Contains(path, "?") {
		// The file holds password and session hashes, so a new one must not be world-readable.
		f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY, 0o600)
		if err != nil {
			return nil, err
		}
		if err := f.Close(); err != nil {
			return nil, err
		}
		dsn = path + "?_pragma=busy_timeout(10000)&_pragma=journal_mode(WAL)"
	}
	// The default logger prints every not-found lookup, which every bad login and
	// stale cookie triggers; a missing row is a normal reply here, not an error.
	// TranslateError turns driver errors into gorm's sentinels, so a unique-constraint
	// violation is recognizable without matching the driver's prose.
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{
		Logger:         logger.Default.LogMode(logger.Silent),
		TranslateError: true,
	})
	if err != nil {
		return nil, err
	}
	return &Store{db: db}, nil
}

// Close closes the underlying SQL connection pool.
func (s *Store) Close() error {
	sqlDB, err := s.db.DB()
	if err != nil {
		return err
	}
	return sqlDB.Close()
}

// InTx runs fn with a Store bound to one transaction, so a mutation and the
// audit event describing it commit together. A method that opens its own
// transaction inside fn becomes a savepoint.
func (s *Store) InTx(fn func(tx *Store) error) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		return fn(&Store{db: tx})
	})
}

// Migrate creates or updates the tables; running it on every boot is safe. A
// database written by a newer binary is refused rather than silently mistreated.
func (s *Store) Migrate() error {
	version, err := s.SchemaVersion()
	if err != nil {
		return err
	}
	if version > SchemaVersion {
		return fmt.Errorf("database schema %d is newer than this binary (%d)", version, SchemaVersion)
	}
	if err := s.db.AutoMigrate(&User{}, &Session{}, &Team{}, &TeamMembership{}, &Project{}, &APIToken{}, &Entry{}, &AuditEvent{}); err != nil {
		return err
	}
	// Entries logged before the column existed get their owner's zone. That is a
	// guess: the zone they were logged in is not recoverable.
	if err := s.db.Exec(`UPDATE entries SET timezone = COALESCE(
		(SELECT timezone FROM users WHERE users.id = entries.user_id), 'UTC')
		WHERE timezone = '' OR timezone IS NULL`).Error; err != nil {
		return err
	}
	// One running entry per user; struct tags cannot express a partial index.
	if err := s.db.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS idx_entries_one_running
		ON entries(user_id) WHERE end_at IS NULL AND deleted_at IS NULL`).Error; err != nil {
		return err
	}
	return s.db.Exec(fmt.Sprintf("PRAGMA user_version = %d", SchemaVersion)).Error
}

// SchemaVersion returns the schema version recorded in the database file.
func (s *Store) SchemaVersion() (int, error) {
	var version int
	if err := s.db.Raw("PRAGMA user_version").Row().Scan(&version); err != nil {
		return 0, err
	}
	return version, nil
}

// Ping reports whether the database can answer.
func (s *Store) Ping() error {
	sqlDB, err := s.db.DB()
	if err != nil {
		return err
	}
	return sqlDB.Ping()
}

// BackupTo writes a consistent snapshot to dest. VACUUM INTO is safe while the
// server is writing and refuses to overwrite an existing file.
func (s *Store) BackupTo(dest string) error {
	return s.db.Exec("VACUUM INTO ?", dest).Error
}

// CreateUser inserts u, lowercasing and trimming u.Email in place. A taken email
// returns ErrDuplicateEmail.
func (s *Store) CreateUser(u *User) error {
	u.Email = normalizeEmail(u.Email)
	err := s.db.Create(u).Error
	if errors.Is(err, gorm.ErrDuplicatedKey) {
		return ErrDuplicateEmail
	}
	return err
}

// UserByID returns the user with id, or ErrNotFound.
func (s *Store) UserByID(id uint) (*User, error) {
	var u User
	if err := s.db.First(&u, id).Error; err != nil {
		return nil, translate(err)
	}
	return &u, nil
}

// UserByEmail matches email case-insensitively and returns ErrNotFound when nobody has it.
func (s *Store) UserByEmail(email string) (*User, error) {
	var u User
	if err := s.db.Where("email = ?", normalizeEmail(email)).First(&u).Error; err != nil {
		return nil, translate(err)
	}
	return &u, nil
}

// CreateSession stores a session; TokenHash must already be hashed by auth.HashToken.
func (s *Store) CreateSession(sess *Session) error {
	return s.db.Create(sess).Error
}

// SessionByTokenHash looks up a session by auth.HashToken(token).
// It does not check ExpiresAt; callers do.
func (s *Store) SessionByTokenHash(hash string) (*Session, error) {
	var sess Session
	if err := s.db.Where("token_hash = ?", hash).First(&sess).Error; err != nil {
		return nil, translate(err)
	}
	return &sess, nil
}

// DeleteSession removes a session; deleting one that is already gone is not an error.
func (s *Store) DeleteSession(id uint) error {
	return s.db.Delete(&Session{}, id).Error
}

// SetUserActive enables or disables a user's ability to log in and use tokens.
// An unknown id returns ErrNotFound.
func (s *Store) SetUserActive(id uint, active bool) error {
	res := s.db.Model(&User{}).Where("id = ?", id).Update("active", active)
	if res.Error == nil && res.RowsAffected == 0 {
		return ErrNotFound
	}
	return res.Error
}

func normalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

func translate(err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ErrNotFound
	}
	return err
}
