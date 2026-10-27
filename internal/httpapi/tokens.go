package httpapi

import (
	"context"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"github.com/havspect/tstamps/internal/user"
)

type tokenResponse struct {
	ID         uint       `json:"id"`
	Name       string     `json:"name"`
	LastUsedAt *time.Time `json:"last_used_at,omitempty"`
	RevokedAt  *time.Time `json:"revoked_at,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
}

func toTokenResponse(t *user.APIToken) tokenResponse {
	if t == nil {
		return tokenResponse{}
	}
	return tokenResponse{
		ID:         t.ID,
		Name:       t.Name,
		LastUsedAt: t.LastUsedAt,
		RevokedAt:  t.RevokedAt,
		CreatedAt:  t.CreatedAt,
	}
}

type tokenListOutput struct {
	Body []tokenResponse
}

type tokenCreateInput struct {
	Body struct {
		Name string `json:"name" required:"true"`
	}
}

type tokenCreateOutput struct {
	Body struct {
		tokenResponse
		Token string `json:"token"`
	}
}

type tokenRevokeInput struct {
	ID uint `path:"id"`
}

func (s *Server) tokenRoutes(api huma.API) {
	huma.Register(api, huma.Operation{
		OperationID: "tokens-list",
		Method:      http.MethodGet,
		Path:        apiPrefix + "/me/tokens",
		Tags:        []string{"tokens"},
		Errors:      []int{http.StatusUnauthorized, http.StatusInternalServerError},
	}, s.listTokens)

	huma.Register(api, huma.Operation{
		OperationID:   "tokens-create",
		Method:        http.MethodPost,
		Path:          apiPrefix + "/me/tokens",
		Tags:          []string{"tokens"},
		DefaultStatus: http.StatusCreated,
		Errors:        []int{http.StatusUnauthorized, http.StatusUnprocessableEntity, http.StatusInternalServerError},
	}, s.createToken)

	huma.Register(api, huma.Operation{
		OperationID:   "tokens-revoke",
		Method:        http.MethodDelete,
		Path:          apiPrefix + "/me/tokens/{id}",
		Tags:          []string{"tokens"},
		DefaultStatus: http.StatusNoContent,
		Errors:        []int{http.StatusUnauthorized, http.StatusNotFound, http.StatusInternalServerError},
	}, s.revokeToken)
}

func (s *Server) listTokens(ctx context.Context, _ *struct{}) (*tokenListOutput, error) {
	if s.deps.UserService == nil {
		return nil, problem(http.StatusServiceUnavailable, urnUnavailable, "user service not available")
	}

	tokens, err := s.deps.UserService.ListPersonalTokens(ctx)
	if err != nil {
		return nil, fail(err)
	}

	out := make([]tokenResponse, len(tokens))
	for i, t := range tokens {
		out[i] = toTokenResponse(&t)
	}
	return &tokenListOutput{Body: out}, nil
}

func (s *Server) createToken(ctx context.Context, in *tokenCreateInput) (*tokenCreateOutput, error) {
	if s.deps.UserService == nil {
		return nil, problem(http.StatusServiceUnavailable, urnUnavailable, "user service not available")
	}

	tok, raw, err := s.deps.UserService.CreatePersonalToken(ctx, in.Body.Name)
	if err != nil {
		return nil, fail(err)
	}

	out := &tokenCreateOutput{}
	out.Body.tokenResponse = toTokenResponse(tok)
	out.Body.Token = raw
	return out, nil
}

func (s *Server) revokeToken(ctx context.Context, in *tokenRevokeInput) (*struct{}, error) {
	if s.deps.UserService == nil {
		return nil, problem(http.StatusServiceUnavailable, urnUnavailable, "user service not available")
	}

	err := s.deps.UserService.RevokePersonalToken(ctx, in.ID)
	if err != nil {
		return nil, fail(err)
	}

	return &struct{}{}, nil
}
