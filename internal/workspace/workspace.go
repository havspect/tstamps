// Package workspace hosts every workspace in one SQLite file and selects it
// from the /w/{slug}/ URL prefix.
package workspace

import (
	"errors"
	"strings"
)

// ErrNotFound is returned when a workspace does not exist.
var ErrNotFound = errors.New("workspace not found")

// ErrInvalidSlug is returned for an identifier that cannot name a workspace.
var ErrInvalidSlug = errors.New("invalid workspace slug")

// ValidateSlug accepts lowercase letters, digits, dots and hyphens. The slug is
// unique per server, so path separators and traversal are refused.
func ValidateSlug(slug string) error {
	if len(slug) == 0 || len(slug) > 63 {
		return ErrInvalidSlug
	}
	if slug[0] == '.' || slug[len(slug)-1] == '.' || strings.Contains(slug, "..") {
		return ErrInvalidSlug
	}
	for i, r := range slug {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
		case (r == '.' || r == '-') && i > 0 && i < len(slug)-1:
		default:
			return ErrInvalidSlug
		}
	}
	return nil
}
