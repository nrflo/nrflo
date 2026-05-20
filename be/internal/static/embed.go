package static

import (
	"embed"
	"io/fs"
	"sort"
	"strings"
)

//go:embed all:dist
var distFS embed.FS

//go:embed all:doc
var docFS embed.FS

// DistFS returns the embedded UI distribution filesystem.
// Returns fs.Sub rooted at "dist" so files are accessible without the prefix.
func DistFS() (fs.FS, error) {
	return fs.Sub(distFS, "dist")
}

// Manual returns the embedded documentation for the given kind.
// For "common", concatenates all doc/common*.md files in sorted order.
// For other kinds, reads doc/<kind>.md directly.
// Returns empty string on any error or if content is absent.
func Manual(kind string) string {
	if kind == "common" {
		matches, err := fs.Glob(docFS, "doc/common*.md")
		if err != nil || len(matches) == 0 {
			return ""
		}
		sort.Strings(matches)
		var parts []string
		for _, m := range matches {
			data, err := fs.ReadFile(docFS, m)
			if err != nil {
				continue
			}
			parts = append(parts, string(data))
		}
		return strings.Join(parts, "\n")
	}
	data, err := fs.ReadFile(docFS, "doc/"+kind+".md")
	if err != nil {
		return ""
	}
	return string(data)
}
