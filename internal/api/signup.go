package api

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/havspect/tstamps/internal/clock"
)

const (
	signupLimit  = 5
	signupWindow = time.Hour
)

// Signup errors the HTTP layer maps to a status and a stable code.
var (
	ErrSignupDisabled = errors.New("signup disabled")
	ErrInvalidToken   = errors.New("invalid signup token")
	ErrSlugTaken      = errors.New("workspace slug taken")
	ErrMailFailed     = errors.New("verification email failed")
)

// SignupInputError is a field problem the service can explain to the person.
type SignupInputError struct{ Detail string }

func (e *SignupInputError) Error() string { return e.Detail }

// SignupRequest is the second page: the verified token plus the workspace and
// its first admin.
type SignupRequest struct {
	Token    string
	Slug     string
	Name     string
	Password string
}

// SignupResult carries the new workspace and a session for its admin.
type SignupResult struct {
	Slug    string
	Token   string
	Expires time.Time
}

// SignupService is implemented by the hosted workspace layer: email verification
// state and database creation live there, HTTP concerns stay here. A nil service
// means self-service signup is not configured on this server.
type SignupService interface {
	Start(ctx context.Context, email string) error
	Verify(ctx context.Context, token string) (email string, err error)
	Complete(ctx context.Context, in SignupRequest) (SignupResult, error)
}

type signupHandler struct {
	svc   SignupService
	clock clock.Clock
	limit *limiter
}

// NewSignupHandler serves /api/v1/signup and /api/v1/signup/verify at the hosted
// root, outside the per-workspace OpenAPI document.
func NewSignupHandler(svc SignupService, clk clock.Clock) http.Handler {
	return &signupHandler{svc: svc, clock: clk, limit: newLimiter(signupLimit, signupWindow)}
}

func (h *signupHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if h.svc == nil {
		writeProblem(w, problem(http.StatusServiceUnavailable, urnUnavailable, "self-service signup is not enabled on this server"))
		return
	}
	switch {
	case r.URL.Path == apiPrefix+"/signup" && r.Method == http.MethodPost:
		h.start(w, r)
	case r.URL.Path == apiPrefix+"/signup/verify" && r.Method == http.MethodGet:
		h.verify(w, r)
	case r.URL.Path == apiPrefix+"/signup/verify" && r.Method == http.MethodPost:
		h.complete(w, r)
	case r.URL.Path == apiPrefix+"/signup" || r.URL.Path == apiPrefix+"/signup/verify":
		writeProblem(w, problem(http.StatusMethodNotAllowed, urnInvalidInput, "method not allowed"))
	default:
		writeProblem(w, problem(http.StatusNotFound, urnNotFound, "not found"))
	}
}

func (h *signupHandler) start(w http.ResponseWriter, r *http.Request) {
	if !h.crossOriginOK(w, r) {
		return
	}
	if !h.limit.allow(clientIP(r), h.clock.Now()) {
		writeProblem(w, problem(http.StatusTooManyRequests, urnRateLimited, "too many signup attempts, try again later"))
		return
	}
	var in struct {
		Email string `json:"email"`
	}
	if err := decodeJSON(w, r, &in); err != nil {
		writeProblem(w, problem(http.StatusUnprocessableEntity, urnInvalidInput, "invalid request body"))
		return
	}
	if err := h.svc.Start(r.Context(), in.Email); err != nil {
		h.fail(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]string{"status": "sent"})
}

func (h *signupHandler) verify(w http.ResponseWriter, r *http.Request) {
	email, err := h.svc.Verify(r.Context(), r.URL.Query().Get("token"))
	if err != nil {
		h.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"email": email})
}

func (h *signupHandler) complete(w http.ResponseWriter, r *http.Request) {
	if !h.crossOriginOK(w, r) {
		return
	}
	var in struct {
		Token    string `json:"token"`
		Slug     string `json:"slug"`
		Name     string `json:"name"`
		Password string `json:"password"`
	}
	if err := decodeJSON(w, r, &in); err != nil {
		writeProblem(w, problem(http.StatusUnprocessableEntity, urnInvalidInput, "invalid request body"))
		return
	}
	res, err := h.svc.Complete(r.Context(), SignupRequest{
		Token: in.Token, Slug: in.Slug, Name: in.Name, Password: in.Password,
	})
	if err != nil {
		h.fail(w, err)
		return
	}
	w.Header().Add("Set-Cookie", cookie(sessionCookieName(r), res.Token, res.Expires, r))
	writeJSON(w, http.StatusCreated, map[string]string{"workspace": res.Slug})
}

// crossOriginOK rejects a state-changing request a browser did not send from this
// origin. Signup has no session to ride, but creating a workspace and signing its
// admin in are state changes all the same.
func (h *signupHandler) crossOriginOK(w http.ResponseWriter, r *http.Request) bool {
	if sameOrigin(r) {
		return true
	}
	writeProblem(w, problem(http.StatusForbidden, urnInvalidOrigin, "cross-origin request rejected"))
	return false
}

func (h *signupHandler) fail(w http.ResponseWriter, err error) {
	var input *SignupInputError
	switch {
	case errors.Is(err, ErrSignupDisabled):
		writeProblem(w, problem(http.StatusServiceUnavailable, urnUnavailable, "self-service signup is not enabled on this server"))
	case errors.Is(err, ErrInvalidToken):
		writeProblem(w, problem(http.StatusNotFound, urnInvalidToken, "this link is invalid or has already been used"))
	case errors.Is(err, ErrSlugTaken):
		writeProblem(w, problem(http.StatusUnprocessableEntity, urnInvalidInput, "that workspace URL is already taken"))
	case errors.Is(err, ErrMailFailed):
		writeProblem(w, problem(http.StatusBadGateway, urnEmailFailed, "the verification email could not be sent, try again"))
	case errors.As(err, &input):
		writeProblem(w, problem(http.StatusUnprocessableEntity, urnInvalidInput, input.Detail))
	default:
		slog.Error("signup", "error", err)
		writeProblem(w, problem(http.StatusInternalServerError, urnInternal, "internal error"))
	}
}

// decodeJSON bounds the body so an unauthenticated request cannot make the server
// allocate without limit.
func decodeJSON(w http.ResponseWriter, r *http.Request, v any) error {
	r.Body = http.MaxBytesReader(w, r.Body, 4096)
	return json.NewDecoder(r.Body).Decode(v)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
