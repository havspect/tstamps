package httpapi

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/havspect/tstamps/internal/user"
)

type userResponse struct {
	ID                 uint   `json:"id"`
	Email              string `json:"email"`
	Name               string `json:"name"`
	Timezone           string `json:"timezone"`
	WeeklyHours        int    `json:"weekly_hours"`
	IsAdmin            bool   `json:"is_admin"`
	Active             bool   `json:"active"`
	MustChangePassword bool   `json:"must_change_password"`
}

func toUserResponse(u *user.User) userResponse {
	if u == nil {
		return userResponse{}
	}
	return userResponse{
		ID:                 u.ID,
		Email:              u.Email,
		Name:               u.Name,
		Timezone:           u.Timezone,
		WeeklyHours:        u.WeeklyHours,
		IsAdmin:            u.IsAdmin,
		Active:             u.Active,
		MustChangePassword: u.MustChangePassword,
	}
}

type userListOutput struct {
	Body []userResponse
}

type userCreateInput struct {
	Body struct {
		Email       string `json:"email" required:"true" format:"email"`
		Name        string `json:"name" required:"true"`
		Timezone    string `json:"timezone,omitempty"`
		WeeklyHours int    `json:"weekly_hours,omitempty" doc:"Hours per week this person can work. Defaults to 40."`
		IsAdmin     bool   `json:"is_admin,omitempty"`
	}
}

type userCreateOutput struct {
	Body struct {
		userResponse
		InitialPassword string `json:"initial_password"`
	}
}

type userUpdateInput struct {
	ID   uint `path:"id"`
	Body struct {
		Name        *string `json:"name,omitempty"`
		Timezone    *string `json:"timezone,omitempty"`
		WeeklyHours *int    `json:"weekly_hours,omitempty"`
		IsAdmin     *bool   `json:"is_admin,omitempty"`
		Active      *bool   `json:"active,omitempty"`
	}
}

type userUpdateOutput struct {
	Body userResponse
}

type resetPasswordInput struct {
	ID uint `path:"id"`
}

type resetPasswordOutput struct {
	Body struct {
		InitialPassword string `json:"initial_password"`
	}
}

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
	}, s.resetPassword)
}

func (s *Server) listUsers(ctx context.Context, _ *struct{}) (*userListOutput, error) {
	if s.deps.UserService == nil {
		return nil, problem(http.StatusServiceUnavailable, urnUnavailable, "user service not available")
	}

	users, err := s.deps.UserService.ListUsers(ctx)
	if err != nil {
		return nil, fail(err)
	}

	out := make([]userResponse, len(users))
	for i, u := range users {
		out[i] = toUserResponse(&u)
	}
	return &userListOutput{Body: out}, nil
}

func (s *Server) createUser(ctx context.Context, in *userCreateInput) (*userCreateOutput, error) {
	if s.deps.UserService == nil {
		return nil, problem(http.StatusServiceUnavailable, urnUnavailable, "user service not available")
	}

	u, pass, err := s.deps.UserService.CreateUser(ctx, user.CreateInput{
		Email:       in.Body.Email,
		Name:        in.Body.Name,
		Timezone:    in.Body.Timezone,
		WeeklyHours: in.Body.WeeklyHours,
		IsAdmin:     in.Body.IsAdmin,
	})
	if err != nil {
		return nil, fail(err)
	}

	out := &userCreateOutput{}
	out.Body.userResponse = toUserResponse(u)
	out.Body.InitialPassword = pass
	return out, nil
}

func (s *Server) updateUser(ctx context.Context, in *userUpdateInput) (*userUpdateOutput, error) {
	if s.deps.UserService == nil {
		return nil, problem(http.StatusServiceUnavailable, urnUnavailable, "user service not available")
	}

	u, err := s.deps.UserService.UpdateUser(ctx, user.UpdateInput{
		ID:          in.ID,
		Name:        in.Body.Name,
		Timezone:    in.Body.Timezone,
		WeeklyHours: in.Body.WeeklyHours,
		IsAdmin:     in.Body.IsAdmin,
		Active:      in.Body.Active,
	})
	if err != nil {
		return nil, fail(err)
	}

	return &userUpdateOutput{Body: toUserResponse(u)}, nil
}

func (s *Server) resetPassword(ctx context.Context, in *resetPasswordInput) (*resetPasswordOutput, error) {
	if s.deps.UserService == nil {
		return nil, problem(http.StatusServiceUnavailable, urnUnavailable, "user service not available")
	}

	pass, err := s.deps.UserService.ResetPassword(ctx, in.ID)
	if err != nil {
		return nil, fail(err)
	}

	out := &resetPasswordOutput{}
	out.Body.InitialPassword = pass
	return out, nil
}
