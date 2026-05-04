package configeditor

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"be/internal/clock"
	"be/internal/model"
	"be/internal/manifest/config"
	"be/internal/repo"
)

// Service manages versioned config files under a customer config directory.
type Service struct {
	configDir string
	manifest  *config.Manifest
	repo      *repo.ConfigVersionRepo
	clk       clock.Clock
}

// NewService creates a Service.
func NewService(configDir string, manifest *config.Manifest, r *repo.ConfigVersionRepo, clk clock.Clock) *Service {
	return &Service{configDir: configDir, manifest: manifest, repo: r, clk: clk}
}

// Get returns the content of a config file.
// If the file has been edited (DB version > 0), returns the latest DB version.
// Otherwise reads from disk.
func (s *Service) Get(projectID, file string) ([]byte, error) {
	if err := s.validateRelPath(file); err != nil {
		return nil, err
	}
	latestV, err := s.repo.LatestVersion(projectID, file)
	if err != nil {
		return nil, fmt.Errorf("latest version: %w", err)
	}
	if latestV > 0 {
		ver, err := s.repo.Get(projectID, file, latestV)
		if err != nil {
			return nil, fmt.Errorf("get db version: %w", err)
		}
		return ver.Content, nil
	}
	// Disk fallback.
	data, err := os.ReadFile(filepath.Join(s.configDir, file))
	if err != nil {
		return nil, fmt.Errorf("read file: %w", err)
	}
	return data, nil
}

// Put validates content against the file's sidecar schema (if present),
// then inserts a new version into the DB.
func (s *Service) Put(projectID, file, actor string, content []byte) error {
	if err := s.validateRelPath(file); err != nil {
		return err
	}
	if ve := s.validateContent(file, content); ve != nil {
		return ve
	}
	act := actor
	v := &model.ConfigVersion{
		ProjectID: projectID,
		File:      file,
		Content:   content,
		Actor:     &act,
	}
	return s.repo.Insert(v)
}

// History returns all versions of a file, newest first.
func (s *Service) History(projectID, file string) ([]*model.ConfigVersion, error) {
	if err := s.validateRelPath(file); err != nil {
		return nil, err
	}
	return s.repo.History(projectID, file)
}

// Rollback creates a new version whose content matches toVersion.
// This is append-only — it never mutates history.
func (s *Service) Rollback(projectID, file, actor string, toVersion int) error {
	if err := s.validateRelPath(file); err != nil {
		return err
	}
	target, err := s.repo.Get(projectID, file, toVersion)
	if err != nil {
		return fmt.Errorf("get version %d: %w", toVersion, err)
	}
	act := actor
	v := &model.ConfigVersion{
		ProjectID: projectID,
		File:      file,
		Content:   target.Content,
		Actor:     &act,
	}
	return s.repo.Insert(v)
}

// validateRelPath ensures the file path is safe.
func (s *Service) validateRelPath(file string) error {
	if file == "" {
		return fmt.Errorf("file path must not be empty")
	}
	if filepath.IsAbs(file) {
		return fmt.Errorf("file path must be relative")
	}
	clean := filepath.Clean(file)
	if clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return fmt.Errorf("file path must not traverse parent directories")
	}
	return nil
}

// validateContent checks content against a sidecar *.schema.json if present.
func (s *Service) validateContent(file string, content []byte) *ValidationError {
	ext := filepath.Ext(file)
	schemaPath := filepath.Join(s.configDir, strings.TrimSuffix(file, ext)+".schema.json")
	schemaBytes, err := os.ReadFile(schemaPath)
	if err != nil {
		return nil // no sidecar schema — skip validation
	}
	return ValidateYAML(content, schemaBytes)
}
