package api

import (
	"net/http"
	"time"

	"github.com/havspect/tstamps/internal/store"
)

// This file is the read side of entries: turn plain dates into a range, then
// fetch the entries a scope allows. It never decides what a scope is; policy.go
// does that. Reports and the entry list share it, so the padding, the default
// week and the team filter exist once.

// dateRange is a query range: from inclusive, to exclusive, both plain dates.
type dateRange struct{ From, To time.Time }

// padded widens the range by 14h either side, the widest offset a timezone can
// be from UTC, so an entry crossing a local midnight is fetched before it is
// split in Go.
func (r dateRange) padded() (time.Time, time.Time) {
	return r.From.Add(-14 * time.Hour), r.To.Add(14 * time.Hour)
}

// parseRange reads from/to as plain dates, defaulting to the caller's current
// ISO week. An empty one of the pair is a 422, never half a range.
func parseRange(tz string, now time.Time, from, to string) (dateRange, error) {
	if from == "" && to == "" {
		loc, err := time.LoadLocation(tz)
		if err != nil {
			return dateRange{}, internal(err)
		}
		local := now.In(loc)
		monday := time.Date(local.Year(), local.Month(), local.Day()-((int(local.Weekday())+6)%7), 0, 0, 0, 0, loc)
		// from/to are plain dates, so take the local calendar dates, not the
		// instants: a Monday 00:00 in UTC+14 is still Sunday in UTC.
		y, m, d := monday.Date()
		ny, nm, nd := monday.AddDate(0, 0, 7).Date()
		return dateRange{
			time.Date(y, m, d, 0, 0, 0, 0, time.UTC),
			time.Date(ny, nm, nd, 0, 0, 0, 0, time.UTC),
		}, nil
	}
	rng, err := parseDates(from, to)
	if err != nil {
		return dateRange{}, err
	}
	if rng.To.Sub(rng.From) > 366*24*time.Hour {
		return dateRange{}, problem(http.StatusUnprocessableEntity, urnInvalidInput, "the range may not be longer than 366 days")
	}
	return rng, nil
}

// parseDates reads a from/to pair as plain dates without a span limit. Both are
// required: half a range is a 422, never a guess.
func parseDates(from, to string) (dateRange, error) {
	const layout = "2006-01-02"
	if from == "" || to == "" {
		return dateRange{}, problem(http.StatusUnprocessableEntity, urnInvalidInput, "from and to must be given together")
	}
	f, err := time.Parse(layout, from)
	if err != nil {
		return dateRange{}, problem(http.StatusUnprocessableEntity, urnInvalidInput, "from must be a YYYY-MM-DD date")
	}
	t, err := time.Parse(layout, to)
	if err != nil {
		return dateRange{}, problem(http.StatusUnprocessableEntity, urnInvalidInput, "to must be a YYYY-MM-DD date")
	}
	if !t.After(f) {
		return dateRange{}, problem(http.StatusUnprocessableEntity, urnInvalidInput, "to must be after from")
	}
	return dateRange{f, t}, nil
}

// entryFilter narrows a listing or report on top of the caller's scope.
type entryFilter struct {
	UserID    uint
	ProjectID uint
	TeamID    uint
}

// matching keeps only the entries the filter names; 0 means "no filter". Team
// membership comes from the project's team, so it is applied here, not in SQL.
func (f entryFilter) matching(entries []store.Entry, projects map[uint]store.Project) []store.Entry {
	out := make([]store.Entry, 0, len(entries))
	for _, e := range entries {
		if f.UserID != 0 && e.UserID != f.UserID {
			continue
		}
		if f.ProjectID != 0 && e.ProjectID != f.ProjectID {
			continue
		}
		if f.TeamID != 0 && projects[e.ProjectID].TeamID != f.TeamID {
			continue
		}
		out = append(out, e)
	}
	return out
}

// entryPage is the readable entries for a range, plus the projects they
// reference. Reports load users and teams themselves; a plain list does not
// need them.
type entryPage struct {
	entries  []store.Entry
	projects map[uint]store.Project
}

// readEntries fetches the caller's readable entries for rng, already narrowed by
// filter, and the projects those entries reference.
func (s *Server) readEntries(scope store.Scope, rng dateRange, filter entryFilter) (entryPage, error) {
	from, to := rng.padded()
	entries, err := s.Store.EntriesInRange(from, to, scope)
	if err != nil {
		return entryPage{}, err
	}
	ids := make([]uint, 0, len(entries))
	for _, e := range entries {
		ids = append(ids, e.ProjectID)
	}
	projects, err := s.Store.ProjectsByIDs(ids)
	if err != nil {
		return entryPage{}, err
	}
	byID := make(map[uint]store.Project, len(projects))
	for _, p := range projects {
		byID[p.ID] = p
	}
	return entryPage{entries: filter.matching(entries, byID), projects: byID}, nil
}
