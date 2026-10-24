package web

import (
	"io/fs"
	"path"
	"strings"
)

// resolveDocs maps a cleaned URL path to a file in the embedded docs site.
// under reports whether the path belongs to /docs; an unknown docs path is a
// real 404, not the SPA. Non-docs paths return ("", false).
func resolveDocs(name string, fsys fs.FS) (file string, under bool) {
	name = strings.TrimSuffix(name, "/")
	if name != "docs" && !strings.HasPrefix(name, "docs/") {
		return "", false
	}
	if name == "docs" {
		return "docs/index.html", true
	}
	for _, candidate := range []string{name, name + ".html", path.Join(name, "index.html")} {
		if isFile(fsys, candidate) {
			return candidate, true
		}
	}
	return "", true
}

func isFile(fsys fs.FS, name string) bool {
	info, err := fs.Stat(fsys, name)
	return err == nil && !info.IsDir()
}
