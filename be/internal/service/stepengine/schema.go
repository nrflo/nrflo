package stepengine

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"be/internal/model"
)

var orderedLinePattern = regexp.MustCompile(`^\s*(\d+)[.)]\s+\S`)

// validateSchemaValue dispatches to the fixed named-schema validators keyed
// by model.FindingSchema* constants (Rule 6: the schema-name switch lives in
// this one place, never scattered as name-checks at call sites). Returns the
// path-bearing candidates the value carries (only json_array_path_change
// produces any) and a specific, agent-actionable error naming what is wrong.
func validateSchemaValue(schemaName string, raw json.RawMessage) ([]string, error) {
	raw = unwrapOnce(raw)
	switch schemaName {
	case model.FindingSchemaJSONArrayPathChange:
		return validateJSONArrayPathChange(raw)
	case model.FindingSchemaNonemptyText:
		return nil, validateNonemptyText(raw)
	case model.FindingSchemaOrderedLines:
		return nil, validateOrderedLines(raw)
	default:
		return nil, fmt.Errorf("unknown schema %q", schemaName)
	}
}

// unwrapOnce tolerates a value stored as a JSON string that itself parses as
// an array/object — one unwrap only, mirroring service.unwrapDoubleEncodedJSON's
// contract without importing service.
func unwrapOnce(raw json.RawMessage) json.RawMessage {
	trimmed := strings.TrimSpace(string(raw))
	if len(trimmed) == 0 || trimmed[0] != '"' {
		return raw
	}
	var inner string
	if err := json.Unmarshal(raw, &inner); err != nil {
		return raw
	}
	innerTrim := strings.TrimSpace(inner)
	if len(innerTrim) == 0 || (innerTrim[0] != '[' && innerTrim[0] != '{') {
		return raw
	}
	if !json.Valid([]byte(innerTrim)) {
		return raw
	}
	return json.RawMessage(innerTrim)
}

// validateJSONArrayPathChange requires a JSON array (may be empty); each
// element must be an object with a non-empty string "path" plus at least one
// other non-empty descriptive string field (covers both {path,change} and
// {path,purpose} shapes). Every element's "path" is returned as a
// path-bearing candidate.
func validateJSONArrayPathChange(raw json.RawMessage) ([]string, error) {
	var arr []map[string]interface{}
	if err := json.Unmarshal(raw, &arr); err != nil {
		return nil, fmt.Errorf("must be a JSON array of objects: %v", err)
	}
	paths := make([]string, 0, len(arr))
	for i, el := range arr {
		pathVal, ok := el["path"]
		path, isStr := pathVal.(string)
		if !ok || !isStr || strings.TrimSpace(path) == "" {
			return nil, fmt.Errorf("element %d: missing non-empty string \"path\"", i)
		}
		if !hasDescriptiveSibling(el) {
			return nil, fmt.Errorf("element %d (%s): needs a non-empty descriptive string field alongside \"path\" (e.g. \"change\" or \"purpose\")", i, path)
		}
		paths = append(paths, path)
	}
	return paths, nil
}

func hasDescriptiveSibling(el map[string]interface{}) bool {
	for k, v := range el {
		if k == "path" {
			continue
		}
		if s, ok := v.(string); ok && strings.TrimSpace(s) != "" {
			return true
		}
	}
	return false
}

// validateNonemptyText requires the value to decode to a string that is
// non-empty after TrimSpace.
func validateNonemptyText(raw json.RawMessage) error {
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		return fmt.Errorf("must be a JSON string: %v", err)
	}
	if strings.TrimSpace(s) == "" {
		return fmt.Errorf("must be non-empty (whitespace-only is not accepted)")
	}
	return nil
}

// validateOrderedLines requires a string with >=2 non-empty lines, each
// matching ^\s*\d+[.)]\s+\S with strictly ascending numbers starting at 1.
func validateOrderedLines(raw json.RawMessage) error {
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		return fmt.Errorf("must be a JSON string: %v", err)
	}
	var lines []string
	for _, l := range strings.Split(s, "\n") {
		if strings.TrimSpace(l) != "" {
			lines = append(lines, l)
		}
	}
	if len(lines) < 2 {
		return fmt.Errorf("must have at least 2 numbered lines, got %d", len(lines))
	}
	expected := 1
	for _, l := range lines {
		m := orderedLinePattern.FindStringSubmatch(l)
		if m == nil {
			return fmt.Errorf("line %q must match \"N. text\" or \"N) text\"", l)
		}
		n, err := strconv.Atoi(m[1])
		if err != nil || n != expected {
			return fmt.Errorf("line %q: expected number %d, got %s (lines must be strictly ascending starting at 1)", l, expected, m[1])
		}
		expected++
	}
	return nil
}
