package httpapi

import (
	"context"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"github.com/havspect/tstamps/internal/auth"
	gormrepo "github.com/havspect/tstamps/internal/repository/gorm"
)

type validateSlugInput struct {
	Body struct {
		Slug string `json:"slug" required:"true"`
	}
}

type validateSlugOutput struct {
	Body struct {
		Valid bool   `json:"valid"`
		Error string `json:"error,omitempty"`
	}
}

type startSignupInput struct {
	Body struct {
		Email string `json:"email" required:"true" format:"email"`
	}
}

type startSignupOutput struct {
	Body struct {
		Token string `json:"token,omitempty"`
	}
}

type verifySignupInput struct {
	Token string `query:"token" required:"true"`
}

type verifySignupOutput struct {
	Body struct {
		Email string `json:"email"`
	}
}

type completeSignupInput struct {
	Body struct {
		Token    string `json:"token" required:"true"`
		Slug     string `json:"slug" required:"true"`
		Name     string `json:"name" required:"true"`
		Password string `json:"password" required:"true" minLength:"8"`
	}
}

type completeSignupOutput struct {
	SetCookie string `header:"Set-Cookie"`
	Body      struct {
		Workspace     string `json:"workspace"`
		WorkspaceSlug string `json:"workspace_slug"`
		Email         string `json:"email"`
		Name          string `json:"name"`
		Token         string `json:"token"`
		UserID        uint   `json:"user_id"`
	}
}

func (s *Server) signupRoutes(api huma.API) {
	huma.Register(api, huma.Operation{
		OperationID: "signup-validate-slug",
		Method:      http.MethodPost,
		Path:        apiPrefix + "/signup/validate-slug",
		Tags:        []string{"signup"},
		Errors:      []int{http.StatusUnprocessableEntity, http.StatusInternalServerError},
	}, s.validateSlug)

	huma.Register(api, huma.Operation{
		OperationID:   "signup-start",
		Method:        http.MethodPost,
		Path:          apiPrefix + "/signup",
		Tags:          []string{"signup"},
		DefaultStatus: http.StatusAccepted,
		Errors:        []int{http.StatusUnprocessableEntity, http.StatusServiceUnavailable, http.StatusInternalServerError},
	}, s.startSignup)

	huma.Register(api, huma.Operation{
		OperationID: "signup-verify",
		Method:      http.MethodGet,
		Path:        apiPrefix + "/signup/verify",
		Tags:        []string{"signup"},
		Errors:      []int{http.StatusUnprocessableEntity, http.StatusInternalServerError},
	}, s.verifySignup)

	huma.Register(api, huma.Operation{
		OperationID:   "signup-complete",
		Method:        http.MethodPost,
		Path:          apiPrefix + "/signup/complete",
		Tags:          []string{"signup"},
		DefaultStatus: http.StatusCreated,
		Errors:        []int{http.StatusUnprocessableEntity, http.StatusInternalServerError},
	}, s.completeSignup)

	huma.Register(api, huma.Operation{
		OperationID:   "signup-complete-verify",
		Method:        http.MethodPost,
		Path:          apiPrefix + "/signup/verify",
		Tags:          []string{"signup"},
		DefaultStatus: http.StatusCreated,
		Errors:        []int{http.StatusUnprocessableEntity, http.StatusInternalServerError},
	}, s.completeSignup)
}

func (s *Server) validateSlug(ctx context.Context, in *validateSlugInput) (*validateSlugOutput, error) {
	if s.deps.OnboardingService == nil {
		return nil, problem(http.StatusServiceUnavailable, urnUnavailable, "onboarding service not available")
	}

	err := s.deps.OnboardingService.ValidateSlug(in.Body.Slug)
	out := &validateSlugOutput{}
	if err != nil {
		out.Body.Valid = false
		out.Body.Error = err.Error()
	} else {
		out.Body.Valid = true
	}
	return out, nil
}

func (s *Server) startSignup(ctx context.Context, in *startSignupInput) (*startSignupOutput, error) {
	if s.deps.OnboardingService == nil {
		return nil, problem(http.StatusServiceUnavailable, urnUnavailable, "onboarding service not available")
	}

	token, err := s.deps.OnboardingService.StartSignup(ctx, in.Body.Email)
	if err != nil {
		return nil, fail(err)
	}

	out := &startSignupOutput{}
	out.Body.Token = token
	return out, nil
}

func (s *Server) verifySignup(ctx context.Context, in *verifySignupInput) (*verifySignupOutput, error) {
	if s.deps.OnboardingService == nil {
		return nil, problem(http.StatusServiceUnavailable, urnUnavailable, "onboarding service not available")
	}

	email, err := s.deps.OnboardingService.VerifySignup(ctx, in.Token)
	if err != nil {
		return nil, fail(err)
	}

	out := &verifySignupOutput{}
	out.Body.Email = email
	return out, nil
}

func (s *Server) completeSignup(ctx context.Context, in *completeSignupInput) (*completeSignupOutput, error) {
	if s.deps.OnboardingService == nil || s.deps.DB == nil {
		return nil, problem(http.StatusServiceUnavailable, urnUnavailable, "onboarding service not available")
	}

	res, err := s.deps.OnboardingService.CompleteSignup(ctx, in.Body.Token, in.Body.Slug, in.Body.Name, in.Body.Password)
	if err != nil {
		return nil, fail(err)
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

	var ws gormrepo.Workspace
	if err := s.deps.DB.Where("slug = ?", res.WorkspaceSlug).First(&ws).Error; err != nil {
		return nil, internal(err)
	}

	sess := gormrepo.Session{
		WorkspaceID: ws.ID,
		UserID:      res.UserID,
		TokenHash:   tokenHash,
		ExpiresAt:   expires,
		CreatedAt:   now,
	}
	if err := s.deps.DB.WithContext(gormrepo.WithWorkspaceID(ctx, ws.ID)).Create(&sess).Error; err != nil {
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

	out := &completeSignupOutput{
		SetCookie: cookie.String(),
	}
	out.Body.Workspace = res.WorkspaceSlug
	out.Body.WorkspaceSlug = res.WorkspaceSlug
	out.Body.Email = res.Email
	out.Body.Name = res.Name
	out.Body.Token = token
	out.Body.UserID = res.UserID
	return out, nil
}
