package service

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"

	"be/internal/artifact"
	"be/internal/clock"
	"be/internal/db"
	"be/internal/model"
	"be/internal/repo"
	"be/internal/types"
	"be/internal/ws"
)

// ArtifactService handles artifact lifecycle: staging uploads, attaching to runs, listing, streaming, deleting.
type ArtifactService struct {
	pool     *db.Pool
	clock    clock.Clock
	hub      WSHub
	dataPath string
}

// NewArtifactService creates a new ArtifactService.
func NewArtifactService(pool *db.Pool, clk clock.Clock, hub WSHub, dataPath string) *ArtifactService {
	return &ArtifactService{pool: pool, clock: clk, hub: hub, dataPath: dataPath}
}

type uploadMeta struct {
	ProjectID   string `json:"project_id"`
	UserID      string `json:"user_id"`
	Name        string `json:"name"`
	ContentType string `json:"content_type"`
	Size        int64  `json:"size"`
	CreatedAt   string `json:"created_at"`
}

func (s *ArtifactService) stagingRoot() string {
	return filepath.Join(filepath.Dir(s.dataPath), "tmp", "uploads")
}

// StagingRoot returns the root directory for staged uploads.
func (s *ArtifactService) StagingRoot() string { return s.stagingRoot() }

func (s *ArtifactService) uploadDir(uploadID string) string {
	return filepath.Join(s.stagingRoot(), uploadID)
}

// StageUpload writes a file to a temp staging area and returns an uploadID.
func (s *ArtifactService) StageUpload(_ context.Context, projectID, userID, name string, r io.Reader, size int64, contentType string) (string, error) {
	uploadID := uuid.New().String()
	dir := s.uploadDir(uploadID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("create upload dir: %w", err)
	}

	filePath := filepath.Join(dir, "file")
	tmp, err := os.CreateTemp(dir, ".tmp-*")
	if err != nil {
		os.RemoveAll(dir)
		return "", fmt.Errorf("create temp file: %w", err)
	}
	tmpName := tmp.Name()
	if _, err := io.Copy(tmp, r); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		os.RemoveAll(dir)
		return "", fmt.Errorf("write upload: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		os.RemoveAll(dir)
		return "", fmt.Errorf("sync upload: %w", err)
	}
	tmp.Close()
	if err := os.Rename(tmpName, filePath); err != nil {
		os.Remove(tmpName)
		os.RemoveAll(dir)
		return "", fmt.Errorf("finalize upload: %w", err)
	}

	meta := uploadMeta{
		ProjectID:   projectID,
		UserID:      userID,
		Name:        name,
		ContentType: contentType,
		Size:        size,
		CreatedAt:   s.clock.Now().UTC().Format(time.RFC3339),
	}
	metaJSON, _ := json.Marshal(meta)
	if err := os.WriteFile(filepath.Join(dir, "upload.json"), metaJSON, 0o644); err != nil {
		os.RemoveAll(dir)
		return "", fmt.Errorf("write upload meta: %w", err)
	}
	return uploadID, nil
}

// ReadUploadMeta reads the sidecar metadata for a staged upload.
func (s *ArtifactService) ReadUploadMeta(uploadID string) (*uploadMeta, error) {
	data, err := os.ReadFile(filepath.Join(s.uploadDir(uploadID), "upload.json"))
	if err != nil {
		return nil, fmt.Errorf("upload not found: %s", uploadID)
	}
	var meta uploadMeta
	if err := json.Unmarshal(data, &meta); err != nil {
		return nil, fmt.Errorf("corrupt upload meta: %w", err)
	}
	return &meta, nil
}

// CancelUpload removes a staged upload directory.
func (s *ArtifactService) CancelUpload(_ context.Context, uploadID string) error {
	return os.RemoveAll(s.uploadDir(uploadID))
}

// AttachInputArtifacts moves staged uploads into artifact storage for a workflow instance.
// Rolls back already-attached artifacts on any error.
func (s *ArtifactService) AttachInputArtifacts(ctx context.Context, projectID, wfiID string, uploads []types.InputArtifactRef) error {
	if len(uploads) == 0 {
		return nil
	}

	cfg, err := NewGlobalSettingsService(s.pool, s.clock).GetArtifactStorage(projectID)
	if err != nil {
		return fmt.Errorf("get artifact storage config: %w", err)
	}
	storage, err := artifact.NewFromProjectConfig(ctx, projectID, cfg)
	if err != nil {
		return fmt.Errorf("init artifact storage: %w", err)
	}

	artifactRepo := repo.NewArtifactRepo(s.pool, s.clock)
	committedIDs := make([]string, 0, len(uploads))
	committedKeys := make([]string, 0, len(uploads))

	rollback := func() {
		for i, id := range committedIDs {
			storage.Delete(ctx, committedKeys[i])
			artifactRepo.Delete(id)
		}
	}

	for _, upload := range uploads {
		meta, err := s.ReadUploadMeta(upload.UploadID)
		if err != nil {
			rollback()
			return fmt.Errorf("upload %s: %w", upload.UploadID, err)
		}
		if meta.ProjectID != projectID {
			rollback()
			return fmt.Errorf("upload %s belongs to a different project", upload.UploadID)
		}

		name := upload.Name
		if name == "" {
			name = meta.Name
		}
		artifactID := uuid.New().String()
		storageKey := wfiID + "/" + artifactID + "__" + name

		f, err := os.Open(filepath.Join(s.uploadDir(upload.UploadID), "file"))
		if err != nil {
			rollback()
			return fmt.Errorf("open upload %s: %w", upload.UploadID, err)
		}
		putErr := storage.Put(ctx, storageKey, f)
		f.Close()
		if putErr != nil {
			rollback()
			return fmt.Errorf("store artifact %s: %w", name, putErr)
		}

		a := &model.Artifact{
			ID:                 artifactID,
			ProjectID:          projectID,
			WorkflowInstanceID: wfiID,
			Name:               name,
			Type:               string(cfg.Mode),
			PathKey:            storageKey,
			SizeBytes:          meta.Size,
			ContentType:        meta.ContentType,
			Source:             model.ArtifactSourceInput,
		}
		if err := artifactRepo.Create(a); err != nil {
			storage.Delete(ctx, storageKey)
			rollback()
			return fmt.Errorf("record artifact %s: %w", name, err)
		}
		committedIDs = append(committedIDs, artifactID)
		committedKeys = append(committedKeys, storageKey)
	}

	for _, upload := range uploads {
		os.RemoveAll(s.uploadDir(upload.UploadID))
	}
	return nil
}

// List returns all artifacts for a workflow instance.
func (s *ArtifactService) List(_ context.Context, wfiID string) ([]*model.Artifact, error) {
	return repo.NewArtifactRepo(s.pool, s.clock).List(wfiID)
}

// Get returns an artifact by ID.
func (s *ArtifactService) Get(_ context.Context, id string) (*model.Artifact, error) {
	return repo.NewArtifactRepo(s.pool, s.clock).Get(id)
}

// Open returns a ReadCloser for the artifact's content.
func (s *ArtifactService) Open(ctx context.Context, a *model.Artifact) (io.ReadCloser, error) {
	cfg, err := NewGlobalSettingsService(s.pool, s.clock).GetArtifactStorage(a.ProjectID)
	if err != nil {
		return nil, fmt.Errorf("get artifact storage config: %w", err)
	}
	storage, err := artifact.NewFromProjectConfig(ctx, a.ProjectID, cfg)
	if err != nil {
		return nil, fmt.Errorf("init artifact storage: %w", err)
	}
	return storage.Get(ctx, a.PathKey)
}

// Delete removes an artifact from storage and the database, then broadcasts.
func (s *ArtifactService) Delete(ctx context.Context, id string) error {
	artifactRepo := repo.NewArtifactRepo(s.pool, s.clock)
	a, err := artifactRepo.Get(id)
	if err != nil {
		return fmt.Errorf("get artifact: %w", err)
	}
	if a == nil {
		return fmt.Errorf("artifact not found: %s", id)
	}

	cfg, err := NewGlobalSettingsService(s.pool, s.clock).GetArtifactStorage(a.ProjectID)
	if err != nil {
		return fmt.Errorf("get artifact storage config: %w", err)
	}
	storage, err := artifact.NewFromProjectConfig(ctx, a.ProjectID, cfg)
	if err != nil {
		return fmt.Errorf("init artifact storage: %w", err)
	}

	if err := storage.Delete(ctx, a.PathKey); err != nil {
		return fmt.Errorf("delete artifact from storage: %w", err)
	}
	if err := artifactRepo.Delete(id); err != nil {
		return fmt.Errorf("delete artifact from db: %w", err)
	}

	BroadcastFromCtx(s.hub, ws.EventArtifactDeleted, BroadcastCtx{ProjectID: a.ProjectID}, map[string]interface{}{
		"artifact_id":          id,
		"workflow_instance_id": a.WorkflowInstanceID,
	})
	return nil
}
