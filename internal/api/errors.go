package api

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/havspect/tstamps/internal/entry"
	"github.com/havspect/tstamps/internal/store"
)

// Stable machine-readable codes; the type URI is the contract, the prose is not.
const (
	urnInvalidCredentials     = "urn:tstamps:error:invalid-credentials"
	urnUnauthorized           = "urn:tstamps:error:unauthorized"
	urnInvalidOrigin          = "urn:tstamps:error:invalid-origin"
	urnInternal               = "urn:tstamps:error:internal"
	urnRateLimited            = "urn:tstamps:error:rate-limited"
	urnForbidden              = "urn:tstamps:error:forbidden"
	urnNotFound               = "urn:tstamps:error:not-found"
	urnInvalidInput           = "urn:tstamps:error:invalid-input"
	urnLastAdmin              = "urn:tstamps:error:last-admin"
	urnPasswordChangeRequired = "urn:tstamps:error:password-change-required"
	urnTimerAlreadyRunning    = "urn:tstamps:error:timer-already-running"
	urnEntryTooLong           = "urn:tstamps:error:entry-too-long"
	urnProjectHasEntries      = "urn:tstamps:error:project-has-entries"
	urnUnavailable            = "urn:tstamps:error:unavailable"
)

func problem(status int, urn, detail string) *huma.ErrorModel {
	return &huma.ErrorModel{Type: urn, Title: http.StatusText(status), Status: status, Detail: detail}
}

// fail maps a store error to the reply, so each rule code has one status, one URN
// and one wording. Handlers use it for errors they cannot explain better.
func fail(err error) *huma.ErrorModel {
	switch {
	case errors.Is(err, store.ErrNotFound):
		return problem(http.StatusNotFound, urnNotFound, "not found")
	case errors.Is(err, store.ErrDuplicateEmail):
		return problem(http.StatusUnprocessableEntity, urnInvalidInput, "email is already in use")
	case errors.Is(err, store.ErrLastAdmin):
		return problem(http.StatusUnprocessableEntity, urnLastAdmin, "the last active admin cannot be demoted or deactivated")
	case errors.Is(err, store.ErrUnknownUser):
		return problem(http.StatusUnprocessableEntity, urnInvalidInput, "unknown user")
	case errors.Is(err, store.ErrEntryRunning):
		return problem(http.StatusConflict, urnTimerAlreadyRunning, "the user already has a running entry")
	case errors.Is(err, store.ErrProjectHasEntries):
		return problem(http.StatusUnprocessableEntity, urnProjectHasEntries, "a project with entries cannot change team")
	case errors.Is(err, entry.ErrForbidden):
		return problem(http.StatusForbidden, urnForbidden, "you can only change your own entries")
	case errors.Is(err, entry.ErrUnknownProject):
		return problem(http.StatusUnprocessableEntity, urnInvalidInput, "unknown project")
	case errors.Is(err, entry.ErrArchivedProject):
		return problem(http.StatusUnprocessableEntity, urnInvalidInput, "project is archived")
	case errors.Is(err, entry.ErrRetimeRunning):
		return problem(http.StatusUnprocessableEntity, urnInvalidInput, "a running entry needs an end_at to be retimed")
	case errors.Is(err, entry.ErrTooLong):
		return problem(http.StatusUnprocessableEntity, urnEntryTooLong, "an entry may not be longer than 24 hours")
	case errors.Is(err, entry.ErrFuture):
		return problem(http.StatusUnprocessableEntity, urnInvalidInput, "an entry may not be more than 5 minutes in the future")
	case errors.Is(err, entry.ErrInvalidRange):
		return problem(http.StatusUnprocessableEntity, urnInvalidInput, "end must be after start")
	}
	return internal(err)
}

// internal logs a store failure and answers 500, so a broken database never looks
// like a permission decision.
func internal(err error) *huma.ErrorModel {
	slog.Error("store", "error", err)
	return problem(http.StatusInternalServerError, urnInternal, "internal error")
}

// WriteNotFound and WriteInternal let the hosted workspace router answer in the
// same problem+json shape before a workspace handler exists.
func WriteNotFound(w http.ResponseWriter) {
	writeProblem(w, problem(http.StatusNotFound, urnNotFound, "not found"))
}

func WriteInternal(w http.ResponseWriter) {
	writeProblem(w, problem(http.StatusInternalServerError, urnInternal, "internal error"))
}

func WriteUnavailable(w http.ResponseWriter) {
	writeProblem(w, problem(http.StatusServiceUnavailable, urnUnavailable, "the workspace is not available"))
}
