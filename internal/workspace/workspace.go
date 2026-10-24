// Package workspace hosts one SQLite database per workspace and selects it from
// the /w/{slug}/ URL prefix.
package workspace

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// ErrNotFound is returned when a workspace database does not exist.
var ErrNotFound = errors.New("workspace not found")

// ErrInvalidSlug is returned for an identifier that cannot safely name a file.
var ErrInvalidSlug = errors.New("invalid workspace slug")

// ValidateSlug accepts lowercase letters, digits, dots and hyphens. The value
// names a database file directly, so path separators and traversal are refused.
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

// DatabasePath maps a valid slug to its SQLite file.
func DatabasePath(dir, slug string) (string, error) {
	if err := ValidateSlug(slug); err != nil {
		return "", err
	}
	return filepath.Join(dir, slug+".db"), nil
}

// Databases lists the workspace databases in dir, sorted by slug. Files that do
// not match a valid workspace name are reported rather than silently skipped.
func Databases(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var paths []string
	for _, entry := range entries {
		if !entry.Type().IsRegular() || !strings.HasSuffix(entry.Name(), ".db") {
			continue
		}
		slug := strings.TrimSuffix(entry.Name(), ".db")
		if err := ValidateSlug(slug); err != nil {
			return nil, fmt.Errorf("%s: %w", entry.Name(), err)
		}
		paths = append(paths, filepath.Join(dir, entry.Name()))
	}
	sort.Strings(paths)
	return paths, nil
}
