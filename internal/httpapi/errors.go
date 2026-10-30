package httpapi

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/havspect/tstamps/internal/authz"
	"github.com/havspect/tstamps/internal/entry"
	"github.com/havspect/tstamps/internal/onboarding"
	"github.com/havspect/tstamps/internal/plan"
	"github.com/havspect/tstamps/internal/project"
	"github.com/havspect/tstamps/internal/team"
	"github.com/havspect/tstamps/internal/user"
)

// Stable machine-readable codes (RFC 9457 type URNs).
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
	urnEntryOverlap           = "urn:tstamps:error:entry-overlap"
	urnEntryTooLong           = "urn:tstamps:error:entry-too-long"
	urnPlanWeekFrozen         = "urn:tstamps:error:plan-week-frozen"
	urnProjectHasEntries      = "urn:tstamps:error:project-has-entries"
	urnUnavailable            = "urn:tstamps:error:unavailable"
	urnInvalidToken           = "urn:tstamps:error:invalid-token"
	urnEmailFailed            = "urn:tstamps:error:email-failed"
)

func problem(status int, urn, detail string) *huma.ErrorModel {
	return &huma.ErrorModel{
		Type:   urn,
		Title:  http.StatusText(status),
		Status: status,
		Detail: detail,
	}
}

// fail maps domain errors across all domains to RFC 9457 Problem Details.
func fail(err error) *huma.ErrorModel {
	if err == nil {
		return nil
	}

	var alreadyRunning *entry.AlreadyRunning
	if errors.As(err, &alreadyRunning) {
		return problem(http.StatusConflict, urnTimerAlreadyRunning, "an entry is already running")
	}

	switch {
	// Authorization errors
	case errors.Is(err, authz.ErrUnauthorized):
		return problem(http.StatusUnauthorized, urnUnauthorized, "authentication required")
	case errors.Is(err, authz.ErrForbidden):
		return problem(http.StatusForbidden, urnForbidden, "forbidden")
	case errors.Is(err, authz.ErrReadOnly):
		return problem(http.StatusForbidden, urnForbidden, "write operation forbidden with read-only token")

	// Not Found
	case errors.Is(err, entry.ErrNotFound),
		errors.Is(err, project.ErrNotFound),
		errors.Is(err, team.ErrNotFound),
		errors.Is(err, user.ErrNotFound),
		errors.Is(err, plan.ErrNotFound),
		errors.Is(err, onboarding.ErrNotFound):
		return problem(http.StatusNotFound, urnNotFound, "not found")

	// Conflicts
	case errors.Is(err, entry.ErrRunningExists):
		return problem(http.StatusConflict, urnTimerAlreadyRunning, "an entry is already running")
	case errors.Is(err, entry.ErrOverlap):
		return problem(http.StatusConflict, urnEntryOverlap, "another entry already covers that time on this project")

	// Domain invariants & validation (422 Unprocessable Entity)
	case errors.Is(err, entry.ErrForbidden):
		return problem(http.StatusForbidden, urnForbidden, "you can only change your own entries")
	case errors.Is(err, entry.ErrTooLong):
		return problem(http.StatusUnprocessableEntity, urnEntryTooLong, "an entry may not be longer than 24 hours")
	case errors.Is(err, project.ErrProjectHasEntries):
		return problem(http.StatusUnprocessableEntity, urnProjectHasEntries, "a project with entries cannot change team")
	case errors.Is(err, entry.ErrProjectNotLoggable):
		return problem(http.StatusUnprocessableEntity, urnInvalidInput, "project is not loggable")
	case errors.Is(err, entry.ErrUnknownProject):
		return problem(http.StatusUnprocessableEntity, urnInvalidInput, "unknown project")
	case errors.Is(err, entry.ErrArchivedProject):
		return problem(http.StatusUnprocessableEntity, urnInvalidInput, "project is archived")
	case errors.Is(err, entry.ErrRetimeRunning):
		return problem(http.StatusUnprocessableEntity, urnInvalidInput, "a running entry needs an end_at to be retimed")
	case errors.Is(err, entry.ErrFuture):
		return problem(http.StatusUnprocessableEntity, urnInvalidInput, "an entry may not be more than 5 minutes in the future")
	case errors.Is(err, entry.ErrInvalidRange):
		return problem(http.StatusUnprocessableEntity, urnInvalidInput, "end must be after start")

	// User domain
	case errors.Is(err, user.ErrLastAdmin):
		return problem(http.StatusUnprocessableEntity, urnLastAdmin, "the last active admin cannot be demoted or deactivated")
	case errors.Is(err, user.ErrDuplicateEmail):
		return problem(http.StatusUnprocessableEntity, urnInvalidInput, "email is already in use")
	case errors.Is(err, user.ErrInvalidEmail):
		return problem(http.StatusUnprocessableEntity, urnInvalidInput, "invalid email")
	case errors.Is(err, user.ErrInvalidName):
		return problem(http.StatusUnprocessableEntity, urnInvalidInput, "name cannot be empty")
	case errors.Is(err, user.ErrInvalidTokenName):
		return problem(http.StatusUnprocessableEntity, urnInvalidInput, "token name cannot be empty")
	case errors.Is(err, user.ErrInvalidPassword):
		return problem(http.StatusUnauthorized, urnInvalidCredentials, "invalid password")
	case errors.Is(err, user.ErrPasswordTooShort):
		return problem(http.StatusUnprocessableEntity, urnInvalidInput, "password must be at least 8 characters")

	// Project domain
	case errors.Is(err, project.ErrTeamNotActive):
		return problem(http.StatusUnprocessableEntity, urnInvalidInput, "team is not active")
	case errors.Is(err, project.ErrInvalidName):
		return problem(http.StatusUnprocessableEntity, urnInvalidInput, "project name cannot be empty")

	// Team domain
	case errors.Is(err, team.ErrUnknownUser):
		return problem(http.StatusUnprocessableEntity, urnInvalidInput, "unknown user")
	case errors.Is(err, team.ErrDuplicateMember):
		return problem(http.StatusUnprocessableEntity, urnInvalidInput, "duplicate user in memberships")
	case errors.Is(err, team.ErrInvalidName):
		return problem(http.StatusUnprocessableEntity, urnInvalidInput, "team name cannot be empty")
	case errors.Is(err, team.ErrInvalidRole):
		return problem(http.StatusUnprocessableEntity, urnInvalidInput, "invalid role")

	// Plan domain
	case errors.Is(err, plan.ErrInvalidHours):
		return problem(http.StatusUnprocessableEntity, urnInvalidInput, "plan hours must be between 0 and 168")
	case errors.Is(err, plan.ErrInvalidRange), errors.Is(err, plan.ErrUnknownMonth):
		return problem(http.StatusUnprocessableEntity, urnInvalidInput, "from and to must form a range of weeks, and months are YYYY-MM")
	case errors.Is(err, plan.ErrProjectNotPlannable):
		return problem(http.StatusUnprocessableEntity, urnInvalidInput, "an archived project cannot take new plan hours")
	case errors.Is(err, plan.ErrFrozenWeek):
		return problem(http.StatusUnprocessableEntity, urnPlanWeekFrozen, "the week has ended")

	// Onboarding domain
	case errors.Is(err, onboarding.ErrSignupDisabled):
		return problem(http.StatusServiceUnavailable, urnUnavailable, "self-service signup is not enabled on this server")
	case errors.Is(err, onboarding.ErrInvalidSlug):
		return problem(http.StatusUnprocessableEntity, urnInvalidInput, "invalid workspace slug")
	case errors.Is(err, onboarding.ErrSlugTaken):
		return problem(http.StatusUnprocessableEntity, urnInvalidInput, "workspace slug already taken")
	case errors.Is(err, onboarding.ErrInvalidToken):
		return problem(http.StatusNotFound, urnInvalidToken, "this link is invalid or has already been used")
	case errors.Is(err, onboarding.ErrInvalidEmail):
		return problem(http.StatusUnprocessableEntity, urnInvalidInput, "invalid email address")
	case errors.Is(err, onboarding.ErrInvalidName):
		return problem(http.StatusUnprocessableEntity, urnInvalidInput, "name is required")
	case errors.Is(err, onboarding.ErrPasswordTooShort):
		return problem(http.StatusUnprocessableEntity, urnInvalidInput, "password must be at least 8 characters")
	}

	return internal(err)
}

func internal(err error) *huma.ErrorModel {
	slog.Error("internal error", "error", err)
	return problem(http.StatusInternalServerError, urnInternal, "internal error")
}

func writeProblem(w http.ResponseWriter, p *huma.ErrorModel) {
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(p.Status)
	_ = json.NewEncoder(w).Encode(p)
}
