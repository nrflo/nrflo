package static

import (
	"io/fs"
	"testing"
)

// TestDistFS_NoError verifies that DistFS() does not return an error.
func TestDistFS_NoError(t *testing.T) {
	fsys, err := DistFS()
	if err != nil {
		t.Fatalf("DistFS() error = %v, want nil", err)
	}
	if fsys == nil {
		t.Fatal("DistFS() returned nil fs.FS")
	}
}

// TestDistFS_ContainsGitkeep verifies that dist/.gitkeep is accessible from
// the returned FS (as the bare ".gitkeep" path after fs.Sub strips "dist/").
// When a real UI build is present, .gitkeep is replaced by build artifacts.
func TestDistFS_ContainsGitkeep(t *testing.T) {
	fsys, err := DistFS()
	if err != nil {
		t.Fatalf("DistFS() error = %v", err)
	}

	// If a built UI is present, .gitkeep won't exist (build-ui replaces dist contents)
	if _, err := fs.Stat(fsys, "index.html"); err == nil {
		t.Skip("UI was built — .gitkeep replaced by build artifacts, skipping dev-mode check")
	}

	info, err := fs.Stat(fsys, ".gitkeep")
	if err != nil {
		t.Fatalf("fs.Stat(.gitkeep) error = %v; dist/.gitkeep must exist for //go:embed to compile", err)
	}
	if info.IsDir() {
		t.Error(".gitkeep should be a file, not a directory")
	}
}

// TestDistFS_NoIndexHTMLInDevMode verifies that when only .gitkeep exists in dist/
// (i.e., no UI was built), index.html is not present. This drives the spaHandler
// nil-return behaviour documented in the ticket.
func TestDistFS_NoIndexHTMLInDevMode(t *testing.T) {
	fsys, err := DistFS()
	if err != nil {
		t.Fatalf("DistFS() error = %v", err)
	}

	_, err = fs.Stat(fsys, "index.html")
	if err == nil {
		// A built UI is present — skip rather than fail, since CI with a pre-built
		// dist is also valid.
		t.Skip("index.html found in dist/ — UI was built, skipping dev-mode check")
	}
	// err != nil means index.html absent — expected in dev/CI without UI build.
}
