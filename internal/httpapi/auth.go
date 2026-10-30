package httpapi

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"github.com/havspect/tstamps/internal/auth"
	"github.com/havspect/tstamps/internal/authz"
	gormrepo "github.com/havspect/tstamps/internal/repository/gorm"
)

type MembershipResponse struct {
	TeamID uint   `json:"team_id"`
	Team   string `json:"team"`
	Role   string `json:"role"`
}

type MeResponse struct {
	ID                 uint                 `json:"id"`
	Email              string               `json:"email"`
	Name               string               `json:"name"`
	Timezone           string               `json:"timezone"`
	WeeklyHours        int                  `json:"weekly_hours"`
	IsAdmin            bool                 `json:"is_admin"`
	MustChangePassword bool                 `json:"must_change_password"`
	Memberships        []MembershipResponse `json:"memberships"`
}

type LoginInput struct {
	Body struct {
		Email    string `json:"email" required:"true" format:"email"`
		Password string `json:"password" required:"true" minLength:"1"`
	}
}

type LoginOutput struct {
	SetCookie string `header:"Set-Cookie"`
	Body      MeResponse
}

type LogoutOutput struct {
	SetCookie string `header:"Set-Cookie"`
}

type MeOutput struct {
	Body MeResponse
}

type PasswordInput struct {
	Body struct {
		CurrentPassword string `json:"current_password" required:"true"`
		NewPassword     string `json:"new_password" required:"true" minLength:"8"`
	}
}

func (s *Server) authRoutes(api huma.API) {
	huma.Register(api, huma.Operation{
		OperationID: "auth-login",
		Method:      http.MethodPost,
		Path:        apiPrefix + "/auth/login",
		Tags:        []string{"auth"},
		Errors:      []int{http.StatusUnauthorized, http.StatusUnprocessableEntity, http.StatusInternalServerError},
	}, s.login)

	huma.Register(api, huma.Operation{
		OperationID:   "auth-logout",
		Method:        http.MethodPost,
		Path:          apiPrefix + "/auth/logout",
		Tags:          []string{"auth"},
		DefaultStatus: http.StatusNoContent,
		Errors:        []int{http.StatusUnauthorized, http.StatusInternalServerError},
	}, s.logout)

	huma.Register(api, huma.Operation{
		OperationID: "me",
		Method:      http.MethodGet,
		Path:        apiPrefix + "/me",
		Tags:        []string{"me"},
		Errors:      []int{http.StatusUnauthorized, http.StatusInternalServerError},
	}, s.me)

	huma.Register(api, huma.Operation{
		OperationID:   "me-password",
		Method:        http.MethodPost,
		Path:          apiPrefix + "/me/password",
		Tags:          []string{"me"},
		DefaultStatus: http.StatusNoContent,
		Errors:        []int{http.StatusUnauthorized, http.StatusUnprocessableEntity, http.StatusInternalServerError},
	}, s.changePassword)
}

func (s *Server) login(ctx context.Context, in *LoginInput) (*LoginOutput, error) {
	if s.deps.DB == nil {
		return nil, problem(http.StatusServiceUnavailable, urnUnavailable, "database not configured")
	}

	email := strings.ToLower(strings.TrimSpace(in.Body.Email))
	bypassCtx := gormrepo.WithBypassTenancy(ctx)
	var u gormrepo.User
	if err := s.deps.DB.WithContext(bypassCtx).Where("email = ?", email).First(&u).Error; err != nil || !u.Active {
		return nil, problem(http.StatusUnauthorized, urnInvalidCredentials, "email or password is incorrect")
	}

	if !auth.VerifyPassword(u.PasswordHash, in.Body.Password) {
		return nil, problem(http.StatusUnauthorized, urnInvalidCredentials, "email or password is incorrect")
	}

	token, tokenHash, err := auth.NewSessionToken()
	if err != nil {
		return nil, internal(err)
	}

	now := time.Now()
	if s.deps.Clock != nil {
		now = s.deps.Clock.Now()
	}
	expires := now.Add(auth.SessionTTL)

	sess := gormrepo.Session{
		WorkspaceID: u.WorkspaceID,
		UserID:      u.ID,
		TokenHash:   tokenHash,
		ExpiresAt:   expires,
		CreatedAt:   now,
	}

	if err := s.deps.DB.WithContext(gormrepo.WithWorkspaceID(ctx, u.WorkspaceID)).Create(&sess).Error; err != nil {
		return nil, internal(err)
	}

	meResp, err := s.buildMeResponse(ctx, &u)
	if err != nil {
		return nil, internal(err)
	}

	cookie := &http.Cookie{
		Name:     sessionCookie,
		Value:    token,
		Path:     "/",
		MaxAge:   int(auth.SessionTTL / time.Second),
		Expires:  expires.UTC(),
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	}

	return &LoginOutput{
		SetCookie: cookie.String(),
		Body:      meResp,
	}, nil
}

func (s *Server) logout(ctx context.Context, _ *struct{}) (*LogoutOutput, error) {
	p, ok := auth.PrincipalFrom(ctx)
	if !ok || p == nil {
		return nil, authz.ErrUnauthorized
	}

	if s.deps.DB != nil && p.Source == "session" && p.TokenID != "" {
		_ = s.deps.DB.Where("token_hash = ?", p.TokenID).Delete(&gormrepo.Session{})
	}

	clearCookie := (&http.Cookie{
		Name:     sessionCookie,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
	}).String()

	return &LogoutOutput{SetCookie: clearCookie}, nil
}

func (s *Server) me(ctx context.Context, _ *struct{}) (*MeOutput, error) {
	if s.deps.UserService == nil {
		return nil, problem(http.StatusServiceUnavailable, urnUnavailable, "user service not available")
	}

	prof, err := s.deps.UserService.GetProfile(ctx)
	if err != nil {
		return nil, fail(err)
	}

	var u gormrepo.User
	u.ID = prof.User.ID
	u.WorkspaceID = prof.User.WorkspaceID
	u.Email = prof.User.Email
	u.Name = prof.User.Name
	u.Timezone = prof.User.Timezone
	u.IsAdmin = prof.User.IsAdmin
	u.MustChangePassword = prof.User.MustChangePassword

	meResp, err := s.buildMeResponse(ctx, &u)
	if err != nil {
		return nil, internal(err)
	}

	return &MeOutput{Body: meResp}, nil
}

func (s *Server) changePassword(ctx context.Context, in *PasswordInput) (*struct{}, error) {
	if s.deps.UserService == nil {
		return nil, problem(http.StatusServiceUnavailable, urnUnavailable, "user service not available")
	}

	err := s.deps.UserService.ChangeOwnPassword(ctx, in.Body.CurrentPassword, in.Body.NewPassword)
	if err != nil {
		return nil, fail(err)
	}

	return &struct{}{}, nil
}

func (s *Server) buildMeResponse(ctx context.Context, u *gormrepo.User) (MeResponse, error) {
	out := MeResponse{
		ID:                 u.ID,
		Email:              u.Email,
		Name:               u.Name,
		Timezone:           u.Timezone,
		WeeklyHours:        u.WeeklyHours,
		IsAdmin:            u.IsAdmin,
		MustChangePassword: u.MustChangePassword,
		Memberships:        []MembershipResponse{},
	}

	if s.deps.DB != nil {
		type result struct {
			TeamID uint
			Team   string
			Role   string
		}
		var results []result
		err := s.deps.DB.Table("team_memberships").
			Select("team_memberships.team_id, teams.name as team, team_memberships.role").
			Joins("JOIN teams ON teams.id = team_memberships.team_id").
			Where("team_memberships.user_id = ?", u.ID).
			Scan(&results).Error
		if err == nil {
			for _, r := range results {
				out.Memberships = append(out.Memberships, MembershipResponse{
					TeamID: r.TeamID,
					Team:   r.Team,
					Role:   r.Role,
				})
			}
		}
	}

	return out, nil
}
