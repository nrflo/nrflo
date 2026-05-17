package artifact

import (
	"context"
	"io"
	"os"
	"path/filepath"
)

type internalFS struct {
	projectID string
	root      string // $NRFLO_HOME/projects/{projectID}/artifacts
}

func newInternalFS(projectID string) *internalFS {
	return &internalFS{
		projectID: projectID,
		root:      resolveArtifactRoot(projectID),
	}
}

func resolveArtifactRoot(projectID string) string {
	base := os.Getenv("NRFLO_HOME")
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			base = "."
		} else {
			base = filepath.Join(home, ".nrflo")
		}
	}
	return filepath.Join(base, "projects", projectID, "artifacts")
}

func (fs *internalFS) fullPath(key string) string {
	return filepath.Join(fs.root, key)
}

func (fs *internalFS) Put(_ context.Context, key string, r io.Reader) error {
	full := fs.fullPath(key)
	parent := filepath.Dir(full)
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(parent, ".tmp-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	if _, err := io.Copy(tmp, r); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return err
	}
	if err := os.Rename(tmpName, full); err != nil {
		os.Remove(tmpName)
		return err
	}
	return nil
}

func (fs *internalFS) Get(_ context.Context, key string) (io.ReadCloser, error) {
	return os.Open(fs.fullPath(key))
}

func (fs *internalFS) Delete(_ context.Context, key string) error {
	full := fs.fullPath(key)
	if err := os.Remove(full); err != nil {
		return err
	}
	parent := filepath.Dir(full)
	for parent != fs.root {
		if err := os.Remove(parent); err != nil {
			break // non-empty directory or other error — stop climbing
		}
		parent = filepath.Dir(parent)
	}
	return nil
}
