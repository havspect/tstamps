// Package store persists users and sessions in SQLite through GORM.
package store

import (
	"context"
	cryptoRand "crypto/rand"
	"errors"
	"fmt"
	"os"
	"reflect"
	"strings"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/logger"
	"gorm.io/gorm/schema"
)

// SchemaVersion is the model set this binary understands. It is stored in the
// database's PRAGMA user_version so /healthz reports what is on disk, not what
// the binary hopes is there, and an older binary refuses a newer database.
const SchemaVersion = 8

// DefaultWorkspaceID is the workspace Open scopes to. The server always uses
// Registry; Open is for tests and tools that work on a database before a
// workspace row exists.
const DefaultWorkspaceID = 1

var cryptoRandRead = cryptoRand.Read

// ErrNotFound is returned when a lookup matches no row.
var ErrNotFound = errors.New("not found")

// ErrDuplicateEmail is returned by CreateUser when the email is already taken in
// this workspace.
var ErrDuplicateEmail = errors.New("duplicate email")

// ErrDuplicateSlug is returned by CreateWorkspace when the slug is taken.
var ErrDuplicateSlug = errors.New("duplicate workspace slug")

// Store is a handle to one workspace's rows in the SQLite database.
type Store struct {
	db          *gorm.DB
	workspaceID uint
	owned       bool // true only for Open; registry stores share the registry's pool
}

// Open opens the SQLite database at path and scopes it to workspace 1. The
// server uses OpenRegistry; Open is for tests and tools that work on a database
// before a workspace row exists. File databases get WAL and a 10s busy timeout,
// so a second writer waits instead of failing with "database is locked"
// immediately.
func Open(path string) (*Store, error) {
	st, err := open(path)
	if err != nil {
		return nil, err
	}
	ctx := withWorkspaceID(st.db.Statement.Context, DefaultWorkspaceID)
	return &Store{db: st.db.WithContext(ctx), workspaceID: DefaultWorkspaceID, owned: true}, nil
}

// OpenRegistry opens the database that holds several workspaces and their rows.
// Every workspace seen through it shares one connection pool and one schema.
func OpenRegistry(path string) (*Registry, error) {
	st, err := open(path)
	if err != nil {
		return nil, err
	}
	return &Registry{db: st.db}, nil
}

func open(path string) (*Store, error) {
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
		dsn = path + "?_pragma=busy_timeout(10000)&_pragma=journal_mode(WAL)&_txlock=immediate"
	}
	// The default logger prints every not-found lookup, which every bad login and
	// stale cookie triggers; a missing row is a normal reply here, not an error.
	// TranslateError turns driver errors into gorm's sentinels, so a unique-constraint
	// violation is recognizable without matching the driver's prose.
	// _txlock=immediate makes a transaction take the write lock at BEGIN: every
	// transaction here writes, and read-then-write on a deferred transaction gets
	// SQLITE_BUSY_SNAPSHOT (not a wait) when another workspace commits first.
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{
		Logger:         logger.Default.LogMode(logger.Silent),
		TranslateError: true,
	})
	if err != nil {
		return nil, err
	}
	registerTenancyCallbacks(db)
	return &Store{db: db}, nil
}

// Registry owns the shared database. Use ForWorkspace to read or write one
// workspace; the Registry itself only knows the workspace list.
type Registry struct {
	db *gorm.DB
}

// ForWorkspace returns a Store scoped to workspace id. It shares the Registry's
// connection pool, so opening one costs no file handle.
func (r *Registry) ForWorkspace(id uint) *Store {
	ctx := withWorkspaceID(r.db.Statement.Context, id)
	return &Store{db: r.db.WithContext(ctx), workspaceID: id}
}

// InTx runs fn with a Registry bound to one transaction, so creating a
// workspace and its first admin commit together.
func (r *Registry) InTx(fn func(tx *Registry) error) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		return fn(&Registry{db: tx})
	})
}

// Close closes the underlying SQL connection pool.
func (r *Registry) Close() error {
	return closeDB(r.db)
}

// Ping reports whether the database can answer.
func (r *Registry) Ping() error {
	return ping(r.db)
}

// Migrate creates or updates the schema. Running it on every boot is safe, and
// one call covers every workspace in the file.
func (r *Registry) Migrate() error {
	return migrate(r.db)
}

// OAuthSecret returns the server-wide Fosite HMAC secret, generating it if not present.
func (s *Store) OAuthSecret() ([]byte, error) {
	var secret ServerSecret
	if err := s.db.First(&secret, 1).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			buf := make([]byte, 32)
			if _, err := cryptoRandRead(buf); err != nil {
				return nil, err
			}
			if err := s.db.Create(&ServerSecret{ID: 1, Secret: buf}).Error; err != nil {
				return nil, err
			}
			return buf, nil
		}
		return nil, err
	}
	return secret.Secret, nil
}

// BackupTo writes one consistent snapshot of every workspace to dest.
func (r *Registry) BackupTo(dest string) error {
	return backupTo(r.db, dest)
}

// OAuthSecret returns the server-wide Fosite HMAC secret from the registry.
func (r *Registry) OAuthSecret() ([]byte, error) {
	var secret ServerSecret
	if err := r.db.First(&secret, 1).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			buf := make([]byte, 32)
			if _, err := cryptoRandRead(buf); err != nil {
				return nil, err
			}
			if err := r.db.Create(&ServerSecret{ID: 1, Secret: buf}).Error; err != nil {
				return nil, err
			}
			return buf, nil
		}
		return nil, err
	}
	return secret.Secret, nil
}

// WorkspaceBySlug returns the workspace with this slug, or ErrNotFound.
func (r *Registry) WorkspaceBySlug(slug string) (*Workspace, error) {
	var w Workspace
	if err := r.db.Where("slug = ?", slug).First(&w).Error; err != nil {
		return nil, translate(err)
	}
	return &w, nil
}

// Workspaces lists every workspace, oldest first.
func (r *Registry) Workspaces() ([]Workspace, error) {
	out := []Workspace{}
	err := r.db.Order("id").Find(&out).Error
	return out, err
}

// CreateWorkspace inserts w, returning ErrDuplicateSlug when the slug is taken.
func (r *Registry) CreateWorkspace(w *Workspace) error {
	err := r.db.Create(w).Error
	if errors.Is(err, gorm.ErrDuplicatedKey) {
		return ErrDuplicateSlug
	}
	return err
}

// Close closes the underlying SQL connection pool of a store opened with Open.
// A store obtained from Registry.ForWorkspace shares the registry's pool and
// does nothing here; close the Registry instead.
func (s *Store) Close() error {
	if !s.owned {
		return nil
	}
	return closeDB(s.db)
}

// InTx runs fn with a Store bound to one transaction and the same workspace, so
// a mutation and the audit event describing it commit together. A method that
// opens its own transaction inside fn becomes a savepoint.
func (s *Store) InTx(fn func(tx *Store) error) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		return fn(&Store{db: tx, workspaceID: s.workspaceID})
	})
}

// scoped narrows a query to this store's workspace.
func (s *Store) scoped() *gorm.DB {
	return s.db
}

// Migrate creates or updates the tables; running it on every boot is safe. A
// database written by a newer binary is refused rather than silently mistreated.
func (s *Store) Migrate() error {
	return migrate(s.db)
}

// allModels lists every table model this package manages.
var allModels = []any{
	&Workspace{},
	&PendingSignup{},
	&User{},
	&Session{},
	&Team{},
	&TeamMembership{},
	&Project{},
	&APIToken{},
	&Entry{},
	&AuditEvent{},
	&ServerSecret{},
	&OAuthClient{},
	&OAuthCode{},
	&OAuthAccessToken{},
	&OAuthRefreshToken{},
	&OAuthDeviceCode{},
}

func migrate(db *gorm.DB) error {
	var version int
	if err := db.Raw("PRAGMA user_version").Row().Scan(&version); err != nil {
		return err
	}
	if version > SchemaVersion {
		return fmt.Errorf("database schema %d is newer than this binary (%d)", version, SchemaVersion)
	}
	if version == SchemaVersion {
		return nil
	}
	if err := db.AutoMigrate(allModels...); err != nil {
		return err
	}
	// Ensure server-wide HMAC secret exists (ID = 1)
	var secret ServerSecret
	if err := db.First(&secret, 1).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			buf := make([]byte, 32)
			if _, err := cryptoRandRead(buf); err != nil {
				return err
			}
			if err := db.Create(&ServerSecret{ID: 1, Secret: buf}).Error; err != nil {
				return err
			}
		} else {
			return err
		}
	}
	// A row with an empty zone gets its owner's zone. That is a guess: the zone
	// it was logged in is not recoverable.
	if err := db.Exec(`UPDATE entries SET timezone = COALESCE(
		(SELECT timezone FROM users WHERE users.id = entries.user_id), 'UTC')
		WHERE timezone = '' OR timezone IS NULL`).Error; err != nil {
		return err
	}
	// One running entry per user per workspace; struct tags cannot express a
	// partial index.
	if err := db.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS idx_entries_one_running
		ON entries(workspace_id, user_id) WHERE end_at IS NULL AND deleted_at IS NULL`).Error; err != nil {
		return err
	}
	return db.Exec(fmt.Sprintf("PRAGMA user_version = %d", SchemaVersion)).Error
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
	return ping(s.db)
}

// BackupTo writes a consistent snapshot to dest. VACUUM INTO is safe while the
// server is writing and refuses to overwrite an existing file.
func (s *Store) BackupTo(dest string) error {
	return backupTo(s.db, dest)
}

func closeDB(db *gorm.DB) error {
	sqlDB, err := db.DB()
	if err != nil {
		return err
	}
	return sqlDB.Close()
}

func ping(db *gorm.DB) error {
	sqlDB, err := db.DB()
	if err != nil {
		return err
	}
	return sqlDB.Ping()
}

func backupTo(db *gorm.DB, dest string) error {
	return db.Exec("VACUUM INTO ?", dest).Error
}

// CreateUser inserts u into this store's workspace, lowercasing and trimming
// u.Email in place. A taken email in the same workspace returns ErrDuplicateEmail.
func (s *Store) CreateUser(u *User) error {
	u.Email = normalizeEmail(u.Email)
	err := s.db.Create(u).Error
	if errors.Is(err, gorm.ErrDuplicatedKey) {
		return ErrDuplicateEmail
	}
	return err
}

// UserByID returns the user with id in this workspace, or ErrNotFound.
func (s *Store) UserByID(id uint) (*User, error) {
	var u User
	if err := s.scoped().First(&u, id).Error; err != nil {
		return nil, translate(err)
	}
	return &u, nil
}

// UserByEmail matches email case-insensitively within this workspace and returns
// ErrNotFound when nobody has it.
func (s *Store) UserByEmail(email string) (*User, error) {
	var u User
	if err := s.scoped().Where("email = ?", normalizeEmail(email)).First(&u).Error; err != nil {
		return nil, translate(err)
	}
	return &u, nil
}

// CreateSession stores a session in this workspace; TokenHash must already be
// hashed by auth.HashToken.
func (s *Store) CreateSession(sess *Session) error {
	return s.db.Create(sess).Error
}

// SessionByTokenHash looks up a session in this workspace by auth.HashToken(token).
// It does not check ExpiresAt; callers do.
func (s *Store) SessionByTokenHash(hash string) (*Session, error) {
	var sess Session
	if err := s.scoped().Where("token_hash = ?", hash).First(&sess).Error; err != nil {
		return nil, translate(err)
	}
	return &sess, nil
}

// DeleteSession removes a session; deleting one that is already gone is not an error.
func (s *Store) DeleteSession(id uint) error {
	return s.scoped().Delete(&Session{}, id).Error
}

// SetUserActive enables or disables a user's ability to log in and use tokens.
// An unknown id returns ErrNotFound.
func (s *Store) SetUserActive(id uint, active bool) error {
	res := s.scoped().Model(&User{}).Where("id = ?", id).Update("active", active)
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

type workspaceContextKey struct{}

// withWorkspaceID returns a child context with the workspace ID attached.
func withWorkspaceID(ctx context.Context, id uint) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, workspaceContextKey{}, id)
}

// getWorkspaceID extracts the workspace ID from the db statement's context or settings.
func getWorkspaceID(db *gorm.DB) (uint, bool) {
	if db.Statement != nil && db.Statement.Context != nil {
		if id, ok := db.Statement.Context.Value(workspaceContextKey{}).(uint); ok && id > 0 {
			return id, true
		}
	}
	if v, ok := db.Get("workspace_id"); ok {
		if id, ok := v.(uint); ok && id > 0 {
			return id, true
		}
	}
	return 0, false
}

// getTenantWorkspaceField reports whether stmt operates on a model that has a WorkspaceID field,
// and returns the Field if found. Server-global models without WorkspaceID return nil.
func getTenantWorkspaceField(stmt *gorm.Statement) *schema.Field {
	if stmt == nil {
		return nil
	}
	if stmt.Schema == nil {
		if stmt.Model != nil {
			_ = stmt.Parse(stmt.Model)
		} else if stmt.Dest != nil {
			_ = stmt.Parse(stmt.Dest)
		}
	}
	if stmt.Schema == nil {
		return nil
	}
	return stmt.Schema.LookUpField("WorkspaceID")
}

func tenantCreateCallback(db *gorm.DB) {
	if db.Error != nil || db.Statement == nil {
		return
	}
	field := getTenantWorkspaceField(db.Statement)
	if field == nil {
		return
	}

	wsID, ok := getWorkspaceID(db)
	if !ok {
		db.AddError(fmt.Errorf("workspace ID required for tenant model %s", db.Statement.Schema.Name))
		return
	}

	setOrVerify := func(val reflect.Value) error {
		for val.Kind() == reflect.Ptr {
			if val.IsNil() {
				return nil
			}
			val = val.Elem()
		}
		if val.Kind() != reflect.Struct {
			return nil
		}
		currentVal, isZero := field.ValueOf(db.Statement.Context, val)
		if !isZero {
			if curID, ok := currentVal.(uint); ok && curID != 0 && curID != wsID {
				return fmt.Errorf("workspace ID mismatch: got %d, want %d", curID, wsID)
			}
		}
		return field.Set(db.Statement.Context, val, wsID)
	}

	reflectVal := db.Statement.ReflectValue
	switch reflectVal.Kind() {
	case reflect.Slice, reflect.Array:
		for i := 0; i < reflectVal.Len(); i++ {
			if err := setOrVerify(reflectVal.Index(i)); err != nil {
				db.AddError(err)
				return
			}
		}
	case reflect.Struct:
		if err := setOrVerify(reflectVal); err != nil {
			db.AddError(err)
			return
		}
	default:
		if db.Statement.Dest != nil {
			if err := setOrVerify(reflect.ValueOf(db.Statement.Dest)); err != nil {
				db.AddError(err)
				return
			}
		}
	}
}

func tenantScopeCallback(db *gorm.DB) {
	if db.Error != nil || db.Statement == nil {
		return
	}
	field := getTenantWorkspaceField(db.Statement)
	if field == nil {
		return
	}

	wsID, ok := getWorkspaceID(db)
	if !ok {
		db.AddError(fmt.Errorf("workspace ID required for tenant model %s", db.Statement.Schema.Name))
		return
	}

	const appliedKey = "tenancy:applied"
	if _, loaded := db.Statement.Settings.LoadOrStore(appliedKey, true); loaded {
		return
	}

	tableName := db.Statement.Schema.Table
	if db.Statement.Table != "" {
		tableName = db.Statement.Table
	}

	col := clause.Column{Table: tableName, Name: field.DBName}
	db.Statement.AddClause(clause.Where{
		Exprs: []clause.Expression{
			clause.Eq{Column: col, Value: wsID},
		},
	})
}

func registerTenancyCallbacks(db *gorm.DB) {
	_ = db.Callback().Create().Before("gorm:before_create").Register("tenancy:create", tenantCreateCallback)
	_ = db.Callback().Query().Before("gorm:query").Register("tenancy:query", tenantScopeCallback)
	_ = db.Callback().Row().Before("gorm:row").Register("tenancy:row", tenantScopeCallback)
	_ = db.Callback().Update().Before("gorm:before_update").Register("tenancy:update", tenantScopeCallback)
	_ = db.Callback().Delete().Before("gorm:before_delete").Register("tenancy:delete", tenantScopeCallback)
}
