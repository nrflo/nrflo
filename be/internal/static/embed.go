package static

import (
	"embed"
	"io/fs"
)

//go:embed all:dist
var distFS embed.FS

// DistFS returns the embedded UI distribution filesystem.
// Returns fs.Sub rooted at "dist" so files are accessible without the prefix.
func DistFS() (fs.FS, error) {
	return fs.Sub(distFS, "dist")
}
