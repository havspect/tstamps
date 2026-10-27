package api

import (
	"context"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"github.com/havspect/tstamps/internal/auth"
	"github.com/havspect/tstamps/internal/change"
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
	actor := s.changeActorOf(ctx, admin)
	created, err := s.change.Users.Create(ctx, actor, u)
	if err != nil {
		return nil, fail(err)
	}
	return &createUserOutput{Body: createdUser{userResponse: userOf(created), InitialPassword: password}}, nil
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

	actor := s.changeActorOf(ctx, admin)
	after, err := s.change.Users.Update(ctx, actor, in.ID, change.UserPatch{
		Name:     in.Body.Name,
		Timezone: in.Body.Timezone,
		IsAdmin:  in.Body.IsAdmin,
		Active:   in.Body.Active,
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
	actor := s.changeActorOf(ctx, admin)
	if _, err := s.change.Users.ResetPassword(ctx, actor, in.ID, hash); err != nil {
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
