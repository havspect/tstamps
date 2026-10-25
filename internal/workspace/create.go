package workspace

import (
	"errors"
	"fmt"
	"io/fs"
	"os"

	"github.com/havspect/tstamps/internal/auth"
	"github.com/havspect/tstamps/internal/store"
)

// ErrExists is returned when the workspace database is already published.
var ErrExists = errors.New("workspace already exists")

// Create makes a workspace database with its first admin and returns the
// generated initial password. The database is built under a temporary name and
// linked into place only when complete, so a served workspace never appears
// half-migrated.
func Create(dir, slug, email, name string) (string, error) {
	password, err := auth.GeneratePassword()
	if err != nil {
		return "", err
	}
	if err := CreateWithPassword(dir, slug, email, name, password); err != nil {
		return "", err
	}
	return password, nil
}

// CreateWithPassword is Create for a first admin who chose the password, as a
// self-service signup does.
func CreateWithPassword(dir, slug, email, name, password string) error {
	db, err := DatabasePath(dir, slug)
	if err != nil {
		return err
	}
	if _, err := os.Stat(db); err == nil {
		return fmt.Errorf("%w: %s", ErrExists, slug)
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}

	tmp, err := os.CreateTemp(dir, ".workspace-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	if err := tmp.Close(); err != nil {
		return err
	}
	defer os.Remove(tmpPath)

	if err := addAdmin(tmpPath, email, name, password); err != nil {
		return err
	}
	if err := os.Link(tmpPath, db); err != nil {
		if errors.Is(err, fs.ErrExist) {
			return fmt.Errorf("%w: %s", ErrExists, slug)
		}
		return fmt.Errorf("publish workspace %s: %w", slug, err)
	}
	return nil
}

// AddAdmin creates an admin in the database at db, creating and migrating the
// database first. It returns the generated initial password.
func AddAdmin(db, email, name string) (string, error) {
	password, err := auth.GeneratePassword()
	if err != nil {
		return "", err
	}
	if err := addAdmin(db, email, name, password); err != nil {
		return "", err
	}
	return password, nil
}

func addAdmin(db, email, name, password string) error {
	st, err := store.Open(db)
	if err != nil {
		return err
	}
	defer st.Close()

	if err := st.Migrate(); err != nil {
		return err
	}
	if _, err := st.UserByEmail(email); err == nil {
		return fmt.Errorf("%s already exists", email)
	} else if !errors.Is(err, store.ErrNotFound) {
		return err
	}

	hash, err := auth.HashPassword(password)
	if err != nil {
		return err
	}
	u := &store.User{
		Email: email, Name: name, PasswordHash: hash,
		Timezone: "UTC", IsAdmin: true, Active: true,
	}
	if err := st.CreateUser(u); err != nil {
		if errors.Is(err, store.ErrDuplicateEmail) {
			return fmt.Errorf("%s already exists", email)
		}
		return err
	}
	return nil
}
