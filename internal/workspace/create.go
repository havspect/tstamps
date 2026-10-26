package workspace

import (
	"errors"
	"fmt"

	"github.com/havspect/tstamps/internal/auth"
	"github.com/havspect/tstamps/internal/store"
)

// ErrExists is returned when the workspace slug is already taken.
var ErrExists = errors.New("workspace already exists")

// Create makes a workspace with its first admin and returns the generated
// initial password.
func Create(db, slug, email, name string) (string, error) {
	password, err := auth.GeneratePassword()
	if err != nil {
		return "", err
	}
	if err := CreateWithPassword(db, slug, email, name, password); err != nil {
		return "", err
	}
	return password, nil
}

// CreateWithPassword is Create for a first admin who chose the password, as a
// self-service signup does. The workspace row and its admin commit together, so
// a failed create leaves no half-workspace behind.
func CreateWithPassword(db, slug, email, name, password string) error {
	if err := ValidateSlug(slug); err != nil {
		return err
	}
	reg, err := store.OpenRegistry(db)
	if err != nil {
		return err
	}
	defer reg.Close()
	if err := reg.Migrate(); err != nil {
		return err
	}
	// Hash before opening the transaction: one Argon2 run would otherwise hold
	// SQLite's only write lock and stall every workspace.
	hash, err := auth.HashPassword(password)
	if err != nil {
		return err
	}
	err = reg.InTx(func(tx *store.Registry) error {
		w := &store.Workspace{Slug: slug}
		if err := tx.CreateWorkspace(w); err != nil {
			return err
		}
		return addAdminHash(tx.ForWorkspace(w.ID), email, name, hash)
	})
	if errors.Is(err, store.ErrDuplicateSlug) {
		return fmt.Errorf("%w: %s", ErrExists, slug)
	}
	return err
}

func addAdminHash(st *store.Store, email, name, hash string) error {
	if _, err := st.UserByEmail(email); err == nil {
		return fmt.Errorf("%s already exists", email)
	} else if !errors.Is(err, store.ErrNotFound) {
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
