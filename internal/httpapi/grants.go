package httpapi

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"github.com/havspect/tstamps/internal/auth"
	"github.com/havspect/tstamps/internal/oauth"
)

type grantListOutput struct {
	Body []oauth.Grant
}

type grantRevokeInput struct {
	ID string `path:"id"`
}

func (s *Server) grantRoutes(api huma.API) {
	huma.Register(api, huma.Operation{
		OperationID: "oauth-grants-list",
		Method:      http.MethodGet,
		Path:        apiPrefix + "/me/oauth/grants",
		Tags:        []string{"tokens"},
		Errors:      []int{http.StatusUnauthorized, http.StatusInternalServerError},
	}, s.listGrants)

	huma.Register(api, huma.Operation{
		OperationID:   "oauth-grants-revoke",
		Method:        http.MethodDelete,
		Path:          apiPrefix + "/me/oauth/grants/{id}",
		Tags:          []string{"tokens"},
		DefaultStatus: http.StatusNoContent,
		Errors:        []int{http.StatusUnauthorized, http.StatusNotFound, http.StatusInternalServerError},
	}, s.revokeGrant)
}

func (s *Server) listGrants(ctx context.Context, _ *struct{}) (*grantListOutput, error) {
	if s.deps.OAuthProvider == nil || s.deps.OAuthProvider.Store() == nil {
		return nil, problem(http.StatusServiceUnavailable, urnUnavailable, "oauth provider not available")
	}

	p, ok := auth.PrincipalFrom(ctx)
	if !ok || p == nil {
		return nil, problem(http.StatusUnauthorized, urnUnauthorized, "authentication required")
	}

	grants, err := s.deps.OAuthProvider.Store().GetGrants(ctx, p.UserID)
	if err != nil {
		return nil, fail(err)
	}
	if grants == nil {
		grants = []oauth.Grant{}
	}
	return &grantListOutput{Body: grants}, nil
}

func (s *Server) revokeGrant(ctx context.Context, in *grantRevokeInput) (*struct{}, error) {
	if s.deps.OAuthProvider == nil || s.deps.OAuthProvider.Store() == nil {
		return nil, problem(http.StatusServiceUnavailable, urnUnavailable, "oauth provider not available")
	}

	p, ok := auth.PrincipalFrom(ctx)
	if !ok || p == nil {
		return nil, problem(http.StatusUnauthorized, urnUnauthorized, "authentication required")
	}

	err := s.deps.OAuthProvider.Store().RevokeGrant(ctx, p.UserID, in.ID, time.Now())
	if errors.Is(err, oauth.ErrNotFound) {
		return nil, problem(http.StatusNotFound, urnNotFound, "grant not found")
	}
	if err != nil {
		return nil, fail(err)
	}
	return &struct{}{}, nil
}
