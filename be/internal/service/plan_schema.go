package service

import (
	"encoding/json"
	"fmt"
	"sort"

	"be/internal/db"
	"be/internal/types"
)

// WorkflowPlanFindingKey is the server-owned finding key a planner agent
// writes its manifest to via emit_findings. It is resolved ahead of a
// workflow's editable finding_schemas in FindingsService.Emit and can never be
// declared/overridden by an operator (ValidateFindingSchemas rejects it).
const WorkflowPlanFindingKey = "_workflow_plan"

// reservedSchema pairs a server-owned JSON Schema + example with a semantic
// validator that runs after the JSON Schema check passes in Emit.
type reservedSchema struct {
	Schema   json.RawMessage
	Example  json.RawMessage
	Validate func(pool *db.Pool, projectID, workflowID string, raw json.RawMessage) error
}

// workflowPlanJSONSchema enforces the manifest v1 shape structurally;
// ValidatePlanManifest enforces the semantic rules (dense layers, caps,
// cross-layer references, template resolution, ...). additionalProperties is
// false at every level so a plan can never smuggle a model/effort/tools/
// finding-key field through emit_findings.
const workflowPlanJSONSchema = `{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "type": "object",
  "additionalProperties": false,
  "required": ["version", "goal", "layers"],
  "properties": {
    "version": {"type": "integer", "const": 1},
    "goal": {"type": "string", "minLength": 1},
    "layers": {
      "type": "array",
      "minItems": 1,
      "items": {
        "type": "object",
        "additionalProperties": false,
        "required": ["layer", "policy", "nodes"],
        "properties": {
          "layer": {"type": "integer", "minimum": 0},
          "policy": {"type": "string"},
          "nodes": {
            "type": "array",
            "minItems": 1,
            "items": {
              "type": "object",
              "additionalProperties": false,
              "required": ["id", "template", "instructions"],
              "properties": {
                "id": {"type": "string"},
                "template": {"type": "string"},
                "instructions": {"type": "string"}
              }
            }
          }
        }
      }
    },
    "questions": {
      "type": "array",
      "items": {
        "type": "object",
        "additionalProperties": false,
        "required": ["id", "question"],
        "properties": {
          "id": {"type": "string"},
          "question": {"type": "string"}
        }
      }
    }
  }
}`

const workflowPlanExample = `{
  "version": 1,
  "goal": "Investigate and fix the failing checkout flow",
  "layers": [
    {
      "layer": 0,
      "policy": "all",
      "nodes": [
        {
          "id": "investigate",
          "template": "investigator",
          "instructions": "Reproduce the checkout failure and identify the root cause."
        }
      ]
    },
    {
      "layer": 1,
      "policy": "any",
      "nodes": [
        {
          "id": "fix",
          "template": "implementor",
          "instructions": "Fix the root cause found by #{NODE_FINDINGS:investigate}."
        }
      ]
    }
  ],
  "questions": []
}`

var reservedFindingSchemas = map[string]reservedSchema{
	WorkflowPlanFindingKey: {
		Schema:   json.RawMessage(workflowPlanJSONSchema),
		Example:  json.RawMessage(workflowPlanExample),
		Validate: validateWorkflowPlanFinding,
	},
}

func validateWorkflowPlanFinding(pool *db.Pool, projectID, workflowID string, raw json.RawMessage) error {
	m, err := ParsePlanManifest(raw)
	if err != nil {
		return err
	}
	return ValidatePlanManifest(pool, projectID, workflowID, m)
}

// IsReservedFindingKey reports whether key is a server-owned finding key that
// must never be writable via findings_add/append or declared in a workflow's
// finding_schemas.
func IsReservedFindingKey(key string) bool {
	_, ok := reservedFindingSchemas[key]
	return ok
}

// ReservedFindingSchema returns the server-owned schema/example for a
// reserved key, resolved ahead of (and never overridable by) a workflow's
// finding_schemas.
func ReservedFindingSchema(key string) (*types.FindingSchema, bool) {
	rs, ok := reservedFindingSchemas[key]
	if !ok {
		return nil, false
	}
	return &types.FindingSchema{Key: key, Schema: rs.Schema, Example: rs.Example}, true
}

// GuardReservedFindingKey rejects a reserved key with an error naming
// emit_findings as the correct tool, closing the bypass around Emit's
// semantic validation for findings_add/append (both single and bulk).
func GuardReservedFindingKey(key string) error {
	if IsReservedFindingKey(key) {
		return fmt.Errorf("key '%s' is server-owned and schema-validated; use emit_findings", key)
	}
	return nil
}

// GuardReservedFindingKeys applies GuardReservedFindingKey to every key in a
// bulk key_values map.
func GuardReservedFindingKeys(keyValues map[string]string) error {
	for k := range keyValues {
		if err := GuardReservedFindingKey(k); err != nil {
			return err
		}
	}
	return nil
}

// ReservedFindingKeys returns all reserved finding keys, sorted.
func ReservedFindingKeys() []string {
	keys := make([]string, 0, len(reservedFindingSchemas))
	for k := range reservedFindingSchemas {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
