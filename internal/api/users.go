package api

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"github.com/havspect/tstamps/internal/auth"
	"github.com/havspect/tstamps/internal/store"
)

// userResponse is a user as the admin screens see it; never includes the hash.
type userResponse struct {
	ID                 uint   `json:"id"`
	Email              string `json:"email"`
	Name               string `json:"name"`
	Timezone           string `json:"timezone"`
	IsAdmin            bool   `json:"is_admin"`
	Active             bool   `json:"active"`
	MustChangePassword bool   `json:"must_change_password"`
}

// userRoutes registers the user administration and password endpoints.
func (s *Server) userRoutes(api huma.API) {
	huma.Register(api, huma.Operation{
		OperationID: "users-list",
		Method:      http.MethodGet,
		Path:        apiPrefix + "/users",
		Tags:        []string{"users"},
		Errors:      []int{http.StatusUnauthorized, http.StatusForbidden, http.StatusInternalServerError},
	}, s.listUsers)

	huma.Register(api, huma.Operation{
		OperationID:   "users-create",
		Method:        http.MethodPost,
		Path:          apiPrefix + "/users",
		Tags:          []string{"users"},
		DefaultStatus: http.StatusCreated,
		Errors:        []int{http.StatusUnauthorized, http.StatusForbidden, http.StatusUnprocessableEntity, http.StatusInternalServerError},
	}, s.createUser)

	huma.Register(api, huma.Operation{
		OperationID: "users-update",
		Method:      http.MethodPatch,
		Path:        apiPrefix + "/users/{id}",
		Tags:        []string{"users"},
		Errors:      []int{http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound, http.StatusUnprocessableEntity, http.StatusInternalServerError},
	}, s.updateUser)

	huma.Register(api, huma.Operation{
		OperationID: "users-reset-password",
		Method:      http.MethodPost,
		Path:        apiPrefix + "/users/{id}/password",
		Tags:        []string{"users"},
		Errors:      []int{http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound, http.StatusInternalServerError},
	}, s.resetUserPassword)

	huma.Register(api, huma.Operation{
		OperationID:   "me-password",
		Method:        http.MethodPost,
		Path:          apiPrefix + "/me/password",
		Tags:          []string{"users"},
		DefaultStatus: http.StatusNoContent,
		Errors:        []int{http.StatusUnauthorized, http.StatusForbidden, http.StatusUnprocessableEntity, http.StatusInternalServerError},
	}, s.changePassword)
}

type listUsersOutput struct{ Body []userResponse }

func (s *Server) listUsers(ctx context.Context, _ *struct{}) (*listUsersOutput, error) {
	if _, err := requireAdmin(ctx); err != nil {
		return nil, err
	}
	users, err := s.Store.ListUsers()
	if err != nil {
		return nil, internal(err)
	}
	out := make([]userResponse, 0, len(users))
	for i := range users {
		out = append(out, userOf(&users[i]))
	}
	return &listUsersOutput{Body: out}, nil
}

type createUserInput struct {
	Body struct {
		Email    string `json:"email" required:"true" format:"email"`
		Name     string `json:"name" required:"true" minLength:"1"`
		Timezone string `json:"timezone" required:"true" minLength:"1"`
		IsAdmin  bool   `json:"is_admin,omitempty"`
	}
}

// createdUser carries the initial password, which is shown only in this response.
type createdUser struct {
	userResponse
	InitialPassword string `json:"initial_password"`
}

type createUserOutput struct{ Body createdUser }

func (s *Server) createUser(ctx context.Context, in *createUserInput) (*createUserOutput, error) {
	admin, err := requireAdmin(ctx)
	if err != nil {
		return nil, err
	}
	if err := validateTimezone(in.Body.Timezone); err != nil {
		return nil, err
	}
	password, err := auth.GeneratePassword()
	if err != nil {
		return nil, internal(err)
	}
	hash, err := auth.HashPassword(password)
	if err != nil {
		return nil, internal(err)
	}
	u := &store.User{
		Email: in.Body.Email, Name: in.Body.Name, Timezone: in.Body.Timezone,
		PasswordHash: hash, IsAdmin: in.Body.IsAdmin, Active: true, MustChangePassword: true,
	}
	now := s.Clock.Now()
	err = s.Store.InTx(func(tx *store.Store) error {
		if err := tx.CreateUser(u); err != nil {
			return err
		}
		return tx.RecordAudit(now, s.actorOf(ctx, admin), "user.create", "user", u.ID,
			store.Change{After: u.AuditFields()})
	})
	if err != nil {
		return nil, fail(err)
	}
	return &createUserOutput{Body: createdUser{userResponse: userOf(u), InitialPassword: password}}, nil
}

type updateUserInput struct {
	ID   uint `path:"id"`
	Body struct {
		Name     *string `json:"name,omitempty" minLength:"1"`
		Timezone *string `json:"timezone,omitempty" minLength:"1"`
		IsAdmin  *bool   `json:"is_admin,omitempty"`
		Active   *bool   `json:"active,omitempty"`
	}
}

type userOutput struct{ Body userResponse }

func (s *Server) updateUser(ctx context.Context, in *updateUserInput) (*userOutput, error) {
	admin, err := requireAdmin(ctx)
	if err != nil {
		return nil, err
	}
	if in.Body.Timezone != nil {
		if err := validateTimezone(*in.Body.Timezone); err != nil {
			return nil, err
		}
	}

	now := s.Clock.Now()
	var after *store.User
	err = s.Store.InTx(func(tx *store.Store) error {
		before, err := tx.UserByID(in.ID)
		if err != nil {
			return err
		}
		u := *before
		if in.Body.Name != nil {
			u.Name = *in.Body.Name
		}
		if in.Body.Timezone != nil {
			u.Timezone = *in.Body.Timezone
		}
		if in.Body.IsAdmin != nil {
			u.IsAdmin = *in.Body.IsAdmin
		}
		if in.Body.Active != nil {
			u.Active = *in.Body.Active
		}

		// Read the entry SaveUser is about to stop, so the cascade event can show
		// the stop from the target's own row.
		var running *store.Entry
		if before.Active && !u.Active {
			running, err = tx.RunningEntry(in.ID)
			if err != nil && !errors.Is(err, store.ErrNotFound) {
				return err
			}
		}
		if err := tx.SaveUser(&u, now); err != nil {
			return err
		}
		if after, err = tx.UserByID(in.ID); err != nil {
			return err
		}
		actor := s.actorOf(ctx, admin)
		if err := tx.RecordAudit(now, actor, "user.update", "user", in.ID,
			store.Change{Before: before.AuditFields(), After: after.AuditFields()}); err != nil {
			return err
		}
		if running == nil {
			return nil
		}
		stopped, err := tx.EntryByID(running.ID)
		if err != nil {
			return err
		}
		return tx.RecordAudit(now, actor, "entry.stop", "entry", running.ID,
			store.Change{Before: running.AuditFields(), After: stopped.AuditFields()})
	})
	if err != nil {
		return nil, fail(err)
	}
	return &userOutput{Body: userOf(after)}, nil
}

type resetPasswordInput struct {
	ID uint `path:"id"`
}

type resetPasswordOutput struct {
	Body struct {
		InitialPassword string `json:"initial_password"`
	}
}

func (s *Server) resetUserPassword(ctx context.Context, in *resetPasswordInput) (*resetPasswordOutput, error) {
	admin, err := requireAdmin(ctx)
	if err != nil {
		return nil, err
	}
	password, err := auth.GeneratePassword()
	if err != nil {
		return nil, internal(err)
	}
	hash, err := auth.HashPassword(password)
	if err != nil {
		return nil, internal(err)
	}
	now := s.Clock.Now()
	err = s.Store.InTx(func(tx *store.Store) error {
		before, err := tx.UserByID(in.ID)
		if err != nil {
			return err
		}
		if err := tx.SetUserPassword(in.ID, hash, true, 0, now); err != nil {
			return err
		}
		after, err := tx.UserByID(in.ID)
		if err != nil {
			return err
		}
		return tx.RecordAudit(now, s.actorOf(ctx, admin), "user.password.reset", "user", in.ID,
			store.Change{Before: before.AuditFields(), After: after.AuditFields()})
	})
	if err != nil {
		return nil, fail(err)
	}
	out := &resetPasswordOutput{}
	out.Body.InitialPassword = password
	return out, nil
}

type changePasswordInput struct {
	Body struct {
		CurrentPassword string `json:"current_password" required:"true" minLength:"1"`
		NewPassword     string `json:"new_password" required:"true" minLength:"8"`
	}
}

func (s *Server) changePassword(ctx context.Context, in *changePasswordInput) (*struct{}, error) {
	u, err := requireUser(ctx)
	if err != nil {
		return nil, err
	}
	if !auth.VerifyPassword(u.PasswordHash, in.Body.CurrentPassword) {
		return nil, problem(http.StatusUnprocessableEntity, urnInvalidInput, "current password is incorrect")
	}
	hash, err := auth.HashPassword(in.Body.NewPassword)
	if err != nil {
		return nil, internal(err)
	}
	// The caller keeps the session it is changing the password with; everything
	// else the old password could reach dies with it.
	var keep uint
	if ri := requestFrom(ctx); ri != nil && ri.Session != nil {
		keep = ri.Session.ID
	}
	if err := s.Store.SetUserPassword(u.ID, hash, false, keep, s.Clock.Now()); err != nil {
		return nil, internal(err)
	}
	return &struct{}{}, nil
}

// userOf maps a stored user to the API response, leaving out the password hash.
func userOf(u *store.User) userResponse {
	return userResponse{
		ID: u.ID, Email: u.Email, Name: u.Name, Timezone: u.Timezone,
		IsAdmin: u.IsAdmin, Active: u.Active, MustChangePassword: u.MustChangePassword,
	}
}

// validateTimezone rejects a name time.LoadLocation does not know.
func validateTimezone(tz string) error {
	if _, err := time.LoadLocation(tz); err != nil {
		return problem(http.StatusUnprocessableEntity, urnInvalidInput, "unknown timezone "+tz)
	}
	return nil
}
