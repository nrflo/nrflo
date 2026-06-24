package service

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	jsonschema "github.com/santhosh-tekuri/jsonschema/v5"

	"be/internal/db"
	"be/internal/types"
)

const (
	maxFindingSchemas     = 50
	maxFindingSchemaBytes = 16 * 1024 // per schema or example JSON blob
)

// compileJSONSchema compiles s as a JSON Schema (Draft 2020) and returns the
// compiled schema. Shared by python tool input_schema validation and finding
// schema validation.
func compileJSONSchema(s string) (*jsonschema.Schema, error) {
	compiler := jsonschema.NewCompiler()
	compiler.Draft = jsonschema.Draft2020
	if err := compiler.AddResource("schema://compile", strings.NewReader(s)); err != nil {
		return nil, err
	}
	return compiler.Compile("schema://compile")
}

// ValidateFindingSchemas validates a workflow's finding-schema definitions:
// non-empty unique keys, each schema compiles, each example validates against
// its schema, and per-entry/count caps.
func ValidateFindingSchemas(defs []types.FindingSchema) error {
	if len(defs) > maxFindingSchemas {
		return fmt.Errorf("too many finding schemas: %d (max %d)", len(defs), maxFindingSchemas)
	}
	seen := make(map[string]bool, len(defs))
	for _, d := range defs {
		key := strings.TrimSpace(d.Key)
		if key == "" {
			return fmt.Errorf("finding schema key cannot be empty")
		}
		if seen[key] {
			return fmt.Errorf("duplicate finding schema key: %s", key)
		}
		seen[key] = true

		if len(d.Schema) == 0 {
			return fmt.Errorf("finding schema for key '%s' is required", key)
		}
		if len(d.Schema) > maxFindingSchemaBytes || len(d.Example) > maxFindingSchemaBytes {
			return fmt.Errorf("finding schema for key '%s' exceeds %d bytes", key, maxFindingSchemaBytes)
		}

		sch, err := compileJSONSchema(string(d.Schema))
		if err != nil {
			return fmt.Errorf("invalid schema for key '%s': %w", key, err)
		}
		if len(d.Example) == 0 {
			return fmt.Errorf("example for key '%s' is required", key)
		}
		var ex interface{}
		if err := json.Unmarshal(d.Example, &ex); err != nil {
			return fmt.Errorf("invalid example JSON for key '%s': %w", key, err)
		}
		if err := sch.Validate(ex); err != nil {
			return fmt.Errorf("example for key '%s' does not satisfy its schema: %w", key, err)
		}
	}
	return nil
}

// loadFindingSchemas reads the finding-schema definitions for a workflow. For a
// global workflow the instance runs under the selected project but the workflow
// row lives under GlobalProjectID, so a not-found lookup falls back there.
func loadFindingSchemas(pool *db.Pool, projectID, workflowID string) ([]types.FindingSchema, error) {
	var raw string
	err := pool.QueryRow(
		`SELECT finding_schemas FROM workflows WHERE LOWER(project_id) = LOWER(?) AND LOWER(id) = LOWER(?)`,
		projectID, workflowID).Scan(&raw)
	if err == sql.ErrNoRows {
		err = pool.QueryRow(
			`SELECT finding_schemas FROM workflows WHERE LOWER(project_id) = LOWER(?) AND LOWER(id) = LOWER(?)`,
			GlobalProjectID, workflowID).Scan(&raw)
	}
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}
	var defs []types.FindingSchema
	if err := json.Unmarshal([]byte(raw), &defs); err != nil {
		return nil, fmt.Errorf("parse finding_schemas: %w", err)
	}
	return defs, nil
}
