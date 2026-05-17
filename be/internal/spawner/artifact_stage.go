package spawner

import (
	"context"
	"io"
	"os"
	"path/filepath"

	"be/internal/artifact"
	"be/internal/db"
	"be/internal/logger"
	"be/internal/model"
	"be/internal/repo"
)

// EnsureStageDir creates and returns $NRFLO_HOME/projects/{projectID}/artifacts/{wfiID}/.
func EnsureStageDir(projectID, wfiID string) (string, error) {
	dir := filepath.Join(db.DefaultDataDir(), "projects", projectID, "artifacts", wfiID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	return dir, nil
}

// Materialize downloads a single artifact into stageDir. Idempotent: if the file
// already exists with matching size it is returned without re-downloading.
func Materialize(ctx context.Context, a *model.Artifact, stageDir string, storage artifact.Storage) (string, error) {
	dest := filepath.Join(stageDir, a.Name)
	if fi, err := os.Stat(dest); err == nil && fi.Size() == a.SizeBytes {
		return dest, nil
	}
	rc, err := storage.Get(ctx, a.PathKey)
	if err != nil {
		return "", err
	}
	defer rc.Close()

	tmp, err := os.CreateTemp(stageDir, ".tmp-*")
	if err != nil {
		return "", err
	}
	tmpName := tmp.Name()
	if _, err := io.Copy(tmp, rc); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return "", err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return "", err
	}
	tmp.Close()
	if err := os.Rename(tmpName, dest); err != nil {
		os.Remove(tmpName)
		return "", err
	}
	return dest, nil
}

// MaterializeAll downloads all artifacts for a workflow instance into the stage dir.
// Returns the stage dir path. Best-effort: individual artifact errors are logged and skipped.
func MaterializeAll(ctx context.Context, wfiID, projectID string, artifactRepo *repo.ArtifactRepo, storage artifact.Storage) (string, error) {
	stageDir, err := EnsureStageDir(projectID, wfiID)
	if err != nil {
		return "", err
	}
	artifacts, err := artifactRepo.List(wfiID)
	if err != nil {
		return stageDir, err
	}
	for _, a := range artifacts {
		if _, matErr := Materialize(ctx, a, stageDir, storage); matErr != nil {
			logger.Warn(ctx, "artifact materialize failed", "name", a.Name, "error", matErr)
		}
	}
	return stageDir, nil
}
