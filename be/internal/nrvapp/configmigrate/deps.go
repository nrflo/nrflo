package configmigrate

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"be/internal/clock"
	"be/internal/model"
	"be/internal/nrvapp/configeditor"
	"be/internal/repo"
)

// Deps carries the resources available to migration functions.
type Deps struct {
	dir       string
	projectID string
	repo      *repo.NrvappConfigVersionRepo
	clk       clock.Clock
}

// NewDeps creates a Deps for the given config directory, project, repo, and clock.
func NewDeps(dir, projectID string, r *repo.NrvappConfigVersionRepo, clk clock.Clock) Deps {
	return Deps{dir: dir, projectID: projectID, repo: r, clk: clk}
}

// Dir returns the config directory path.
func (d Deps) Dir() string { return d.dir }

// Backup reads dir/file and inserts a snapshot into the config version repo.
// actor is recorded as "configmigrate" to identify automated backups.
func (d Deps) Backup(ctx context.Context, file string) error {
	if err := resolve(file); err != nil {
		return err
	}
	data, err := os.ReadFile(filepath.Join(d.dir, file))
	if err != nil {
		return fmt.Errorf("backup read %s: %w", file, err)
	}
	actor := "configmigrate"
	v := &model.NrvappConfigVersion{
		ProjectID: d.projectID,
		File:      file,
		Content:   data,
		Actor:     &actor,
	}
	return d.repo.Insert(v)
}

// Validate checks that dir/file conforms to schemaBytes (JSON Schema Draft2020).
// Returns nil when validation passes or schemaBytes is empty.
func (d Deps) Validate(file string, schemaBytes []byte) error {
	if err := resolve(file); err != nil {
		return err
	}
	data, err := os.ReadFile(filepath.Join(d.dir, file))
	if err != nil {
		return fmt.Errorf("validate read %s: %w", file, err)
	}
	if ve := configeditor.ValidateYAML(data, schemaBytes); ve != nil {
		return ve
	}
	return nil
}

// resolve checks that file is a safe relative path.
func resolve(file string) error {
	if file == "" {
		return fmt.Errorf("file must not be empty")
	}
	if filepath.IsAbs(file) {
		return fmt.Errorf("file must be relative, got %q", file)
	}
	clean := filepath.Clean(file)
	if clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return fmt.Errorf("file must not traverse parent directories: %q", file)
	}
	return nil
}
