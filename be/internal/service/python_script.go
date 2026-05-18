package service

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	jsonschema "github.com/santhosh-tekuri/jsonschema/v5"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/id"
	"be/internal/model"
	"be/internal/repo"
	"be/internal/types"
)

// PythonScriptService handles python script business logic
type PythonScriptService struct {
	pool  *db.Pool
	clock clock.Clock
}

// NewPythonScriptService creates a new python script service
func NewPythonScriptService(pool *db.Pool, clk clock.Clock) *PythonScriptService {
	return &PythonScriptService{pool: pool, clock: clk}
}

// validateFilePath checks that a file path is valid for script use.
// Empty string is allowed (means no file-path override). Non-empty must be
// absolute, exist, be a regular file, and end in .py.
func validateFilePath(path string) error {
	if path == "" {
		return nil
	}
	if !filepath.IsAbs(path) {
		return fmt.Errorf("file_path must be absolute")
	}
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("file_path does not exist")
		}
		return fmt.Errorf("file_path: %w", err)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("file_path must be a regular file")
	}
	if !strings.HasSuffix(path, ".py") {
		return fmt.Errorf("file_path must end in .py")
	}
	return nil
}

// compileInputSchema validates that s is a valid JSON Schema via Draft2020 and returns an error if not.
func compileInputSchema(s string) error {
	compiler := jsonschema.NewCompiler()
	compiler.Draft = jsonschema.Draft2020
	if err := compiler.AddResource("schema://compile", strings.NewReader(s)); err != nil {
		return fmt.Errorf("invalid input_schema: %w", err)
	}
	if _, err := compiler.Compile("schema://compile"); err != nil {
		return fmt.Errorf("invalid input_schema: %w", err)
	}
	return nil
}

// Create creates a new python script for a project
func (s *PythonScriptService) Create(projectID string, req *types.PythonScriptCreateRequest) (*model.PythonScript, error) {
	if req.Name == "" {
		return nil, fmt.Errorf("name is required")
	}

	kind := req.Kind
	if kind == "" {
		kind = "agent"
	}
	if kind != "agent" && kind != "tool" {
		return nil, fmt.Errorf("kind must be agent or tool")
	}

	timeoutSec := req.TimeoutSec
	toolDescription := req.ToolDescription
	inputSchema := req.InputSchema

	if kind == "tool" {
		if toolDescription == "" {
			return nil, fmt.Errorf("tool_description is required for kind=tool")
		}
		if inputSchema == "" {
			inputSchema = "{}"
		}
		if err := compileInputSchema(inputSchema); err != nil {
			return nil, err
		}
		if timeoutSec == 0 {
			timeoutSec = 30
		}
		if timeoutSec < 1 || timeoutSec > 600 {
			return nil, fmt.Errorf("timeout_sec must be between 1 and 600")
		}
	}

	fp := ""
	if req.FilePath != nil {
		fp = *req.FilePath
	}
	if err := validateFilePath(fp); err != nil {
		return nil, err
	}

	gen := id.New("ps")
	scriptID, err := gen.Generate()
	if err != nil {
		return nil, fmt.Errorf("failed to generate id: %w", err)
	}

	r := repo.NewPythonScriptRepo(s.pool, s.clock)
	script := &model.PythonScript{
		ID:              scriptID,
		ProjectID:       strings.ToLower(projectID),
		Name:            req.Name,
		Description:     req.Description,
		Code:            req.Code,
		FilePath:        fp,
		Kind:            kind,
		ToolDescription: toolDescription,
		InputSchema:     inputSchema,
		TimeoutSec:      timeoutSec,
	}

	if err := r.Create(script); err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint") || strings.Contains(err.Error(), "already exists") {
			return nil, fmt.Errorf("python script already exists: %s", scriptID)
		}
		return nil, err
	}

	return script, nil
}

// Get retrieves a python script by project and ID
func (s *PythonScriptService) Get(projectID, id string) (*model.PythonScript, error) {
	r := repo.NewPythonScriptRepo(s.pool, s.clock)
	return r.Get(projectID, id)
}

// List retrieves all python scripts for a project
func (s *PythonScriptService) List(projectID string) ([]*model.PythonScript, error) {
	r := repo.NewPythonScriptRepo(s.pool, s.clock)
	scripts, err := r.List(projectID)
	if err != nil {
		return nil, err
	}
	if scripts == nil {
		return []*model.PythonScript{}, nil
	}
	return scripts, nil
}

// ListByKind retrieves all python scripts of the given kind for a project
func (s *PythonScriptService) ListByKind(projectID, kind string) ([]*model.PythonScript, error) {
	r := repo.NewPythonScriptRepo(s.pool, s.clock)
	scripts, err := r.ListByKind(projectID, kind)
	if err != nil {
		return nil, err
	}
	if scripts == nil {
		return []*model.PythonScript{}, nil
	}
	return scripts, nil
}

// ListTools retrieves all kind=tool rows for a project
func (s *PythonScriptService) ListTools(projectID string) ([]*model.PythonScript, error) {
	return s.ListByKind(projectID, "tool")
}

// Update updates a python script
func (s *PythonScriptService) Update(projectID, id string, req *types.PythonScriptUpdateRequest) error {
	if req.Name != nil && *req.Name == "" {
		return fmt.Errorf("name cannot be empty")
	}
	if req.FilePath != nil {
		if err := validateFilePath(*req.FilePath); err != nil {
			return err
		}
	}
	if req.InputSchema != nil {
		if err := compileInputSchema(*req.InputSchema); err != nil {
			return err
		}
	}
	if req.TimeoutSec != nil {
		t := *req.TimeoutSec
		if t < 1 || t > 600 {
			return fmt.Errorf("timeout_sec must be between 1 and 600")
		}
	}
	r := repo.NewPythonScriptRepo(s.pool, s.clock)
	return r.Update(projectID, id, req)
}

// Delete deletes a python script
func (s *PythonScriptService) Delete(projectID, id string) error {
	r := repo.NewPythonScriptRepo(s.pool, s.clock)
	return r.Delete(projectID, id)
}
