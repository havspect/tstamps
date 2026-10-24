package workspace

import (
	"errors"
	"fmt"
	"os"

	"github.com/havspect/tstamps/internal/auth"
	"github.com/havspect/tstamps/internal/store"
)

// Create makes a workspace database with its first admin. The database is built
// under a temporary name and linked into place only when complete, so a served
// workspace never appears half-migrated.
func Create(dir, slug, email, name string) (string, error) {
	db, err := DatabasePath(dir, slug)
	if err != nil {
		return "", err
	}
	if _, err := os.Stat(db); err == nil {
		return "", fmt.Errorf("workspace %s already exists", slug)
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}

	tmp, err := os.CreateTemp(dir, ".workspace-*")
	if err != nil {
		return "", err
	}
	tmpPath := tmp.Name()
	if err := tmp.Close(); err != nil {
		return "", err
	}
	defer os.Remove(tmpPath)

	password, err := AddAdmin(tmpPath, email, name)
	if err != nil {
		return "", err
	}
	if err := os.Link(tmpPath, db); err != nil {
		return "", fmt.Errorf("publish workspace %s: %w", slug, err)
	}
	return password, nil
}

// AddAdmin creates an admin in the database at db, creating and migrating the
// database first. It returns the generated initial password.
func AddAdmin(db, email, name string) (string, error) {
	st, err := store.Open(db)
	if err != nil {
		return "", err
	}
	defer st.Close()

	if err := st.Migrate(); err != nil {
		return "", err
	}
	if _, err := st.UserByEmail(email); err == nil {
		return "", fmt.Errorf("%s already exists", email)
	} else if !errors.Is(err, store.ErrNotFound) {
		return "", err
	}

	password, err := auth.GeneratePassword()
	if err != nil {
		return "", err
	}
	hash, err := auth.HashPassword(password)
	if err != nil {
		return "", err
	}
	u := &store.User{
		Email: email, Name: name, PasswordHash: hash,
		Timezone: "UTC", IsAdmin: true, Active: true,
	}
	if err := st.CreateUser(u); err != nil {
		if errors.Is(err, store.ErrDuplicateEmail) {
			return "", fmt.Errorf("%s already exists", email)
		}
		return "", err
	}
	return password, nil
}
