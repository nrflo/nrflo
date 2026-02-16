package integration

import (
	"testing"
	"time"

	"be/internal/ws"
)

func TestProjectFindingsAddAndGet(t *testing.T) {
	env := NewTestEnv(t)

	// Add a single key-value
	env.MustExecute(t, "project_findings.add", map[string]interface{}{
		"key":   "status",
		"value": `"ready"`,
	}, nil)

	// Get by specific key
	var result interface{}
	env.MustExecute(t, "project_findings.get", map[string]interface{}{
		"key": "status",
	}, &result)

	if result != "ready" {
		t.Errorf("expected 'ready', got %v", result)
	}

	// Add another key with JSON array
	env.MustExecute(t, "project_findings.add", map[string]interface{}{
		"key":   "tags",
		"value": `["tag1", "tag2"]`,
	}, nil)

	// Get all findings
	var allFindings map[string]interface{}
	env.MustExecute(t, "project_findings.get", map[string]interface{}{}, &allFindings)

	if allFindings["status"] != "ready" {
		t.Errorf("expected status='ready', got %v", allFindings["status"])
	}

	tags, ok := allFindings["tags"].([]interface{})
	if !ok {
		t.Fatalf("expected tags array, got %T: %v", allFindings["tags"], allFindings["tags"])
	}
	if len(tags) != 2 {
		t.Errorf("expected 2 tags, got %d", len(tags))
	}
}

func TestProjectFindingsUpsertOverwrite(t *testing.T) {
	env := NewTestEnv(t)

	// Add initial value
	env.MustExecute(t, "project_findings.add", map[string]interface{}{
		"key":   "counter",
		"value": `1`,
	}, nil)

	// Verify initial value
	var result interface{}
	env.MustExecute(t, "project_findings.get", map[string]interface{}{
		"key": "counter",
	}, &result)

	if result.(float64) != 1 {
		t.Errorf("expected 1, got %v", result)
	}

	// Overwrite with same key
	env.MustExecute(t, "project_findings.add", map[string]interface{}{
		"key":   "counter",
		"value": `2`,
	}, nil)

	// Verify overwrite
	env.MustExecute(t, "project_findings.get", map[string]interface{}{
		"key": "counter",
	}, &result)

	if result.(float64) != 2 {
		t.Errorf("expected 2 (overwritten), got %v", result)
	}
}

func TestProjectFindingsAddBulk(t *testing.T) {
	env := NewTestEnv(t)

	// Add multiple key-value pairs at once
	env.MustExecute(t, "project_findings.add-bulk", map[string]interface{}{
		"key_values": map[string]string{
			"build":   `"passing"`,
			"version": `"1.0.0"`,
			"deps":    `["dep1", "dep2", "dep3"]`,
		},
	}, nil)

	// Verify all retrievable
	var all map[string]interface{}
	env.MustExecute(t, "project_findings.get", map[string]interface{}{}, &all)

	if all["build"] != "passing" {
		t.Errorf("expected build='passing', got %v", all["build"])
	}
	if all["version"] != "1.0.0" {
		t.Errorf("expected version='1.0.0', got %v", all["version"])
	}

	deps, ok := all["deps"].([]interface{})
	if !ok || len(deps) != 3 {
		t.Errorf("expected 3 deps, got %v", all["deps"])
	}
}

func TestProjectFindingsGetMultipleKeys(t *testing.T) {
	env := NewTestEnv(t)

	// Add multiple keys
	env.MustExecute(t, "project_findings.add-bulk", map[string]interface{}{
		"key_values": map[string]string{
			"key1": `"value1"`,
			"key2": `"value2"`,
			"key3": `"value3"`,
		},
	}, nil)

	// Get specific keys
	var subset map[string]interface{}
	env.MustExecute(t, "project_findings.get", map[string]interface{}{
		"keys": []string{"key1", "key3"},
	}, &subset)

	if subset["key1"] != "value1" {
		t.Errorf("expected key1='value1', got %v", subset["key1"])
	}
	if subset["key3"] != "value3" {
		t.Errorf("expected key3='value3', got %v", subset["key3"])
	}
	if _, exists := subset["key2"]; exists {
		t.Errorf("key2 should not be in subset")
	}
}

func TestProjectFindingsAppendToNonExistent(t *testing.T) {
	env := NewTestEnv(t)

	// Append to non-existent key creates it
	env.MustExecute(t, "project_findings.append", map[string]interface{}{
		"key":   "items",
		"value": `"first"`,
	}, nil)

	var result interface{}
	env.MustExecute(t, "project_findings.get", map[string]interface{}{
		"key": "items",
	}, &result)

	if result != "first" {
		t.Errorf("expected 'first', got %v", result)
	}
}

func TestProjectFindingsAppendToScalar(t *testing.T) {
	env := NewTestEnv(t)

	// Add initial scalar value
	env.MustExecute(t, "project_findings.add", map[string]interface{}{
		"key":   "logs",
		"value": `"log1"`,
	}, nil)

	// Append to scalar creates array
	env.MustExecute(t, "project_findings.append", map[string]interface{}{
		"key":   "logs",
		"value": `"log2"`,
	}, nil)

	var result interface{}
	env.MustExecute(t, "project_findings.get", map[string]interface{}{
		"key": "logs",
	}, &result)

	arr, ok := result.([]interface{})
	if !ok {
		t.Fatalf("expected array, got %T: %v", result, result)
	}
	if len(arr) != 2 {
		t.Errorf("expected 2 items, got %d", len(arr))
	}
	if arr[0] != "log1" || arr[1] != "log2" {
		t.Errorf("expected [log1, log2], got %v", arr)
	}
}

func TestProjectFindingsAppendToArray(t *testing.T) {
	env := NewTestEnv(t)

	// Add initial array
	env.MustExecute(t, "project_findings.add", map[string]interface{}{
		"key":   "files",
		"value": `["file1.go", "file2.go"]`,
	}, nil)

	// Append to array extends it
	env.MustExecute(t, "project_findings.append", map[string]interface{}{
		"key":   "files",
		"value": `"file3.go"`,
	}, nil)

	var result interface{}
	env.MustExecute(t, "project_findings.get", map[string]interface{}{
		"key": "files",
	}, &result)

	arr, ok := result.([]interface{})
	if !ok || len(arr) != 3 {
		t.Fatalf("expected 3 items, got %v", result)
	}
	if arr[2] != "file3.go" {
		t.Errorf("expected file3.go at index 2, got %v", arr[2])
	}
}

func TestProjectFindingsAppendBulk(t *testing.T) {
	env := NewTestEnv(t)

	// Add initial values
	env.MustExecute(t, "project_findings.add", map[string]interface{}{
		"key":   "errors",
		"value": `"error1"`,
	}, nil)
	env.MustExecute(t, "project_findings.add", map[string]interface{}{
		"key":   "warnings",
		"value": `"warn1"`,
	}, nil)

	// Append bulk
	env.MustExecute(t, "project_findings.append-bulk", map[string]interface{}{
		"key_values": map[string]string{
			"errors":   `"error2"`,
			"warnings": `"warn2"`,
		},
	}, nil)

	// Verify arrays
	var all map[string]interface{}
	env.MustExecute(t, "project_findings.get", map[string]interface{}{}, &all)

	errors, ok := all["errors"].([]interface{})
	if !ok || len(errors) != 2 {
		t.Errorf("expected 2 errors, got %v", all["errors"])
	}
	warnings, ok := all["warnings"].([]interface{})
	if !ok || len(warnings) != 2 {
		t.Errorf("expected 2 warnings, got %v", all["warnings"])
	}
}

func TestProjectFindingsDelete(t *testing.T) {
	env := NewTestEnv(t)

	// Add multiple findings
	env.MustExecute(t, "project_findings.add-bulk", map[string]interface{}{
		"key_values": map[string]string{
			"keep":    `"important"`,
			"remove1": `"temp"`,
			"remove2": `"temp"`,
		},
	}, nil)

	// Delete 2 keys
	var delResult map[string]interface{}
	env.MustExecute(t, "project_findings.delete", map[string]interface{}{
		"keys": []string{"remove1", "remove2"},
	}, &delResult)

	// Verify deleted list
	deleted, ok := delResult["deleted"].([]interface{})
	if !ok {
		t.Fatalf("expected deleted array, got %T: %v", delResult["deleted"], delResult["deleted"])
	}
	if len(deleted) != 2 {
		t.Errorf("expected 2 deleted keys, got %d: %v", len(deleted), deleted)
	}

	// Verify only 1 remains
	var all map[string]interface{}
	env.MustExecute(t, "project_findings.get", map[string]interface{}{}, &all)

	if all["keep"] != "important" {
		t.Errorf("expected keep='important', got %v", all["keep"])
	}
	if _, exists := all["remove1"]; exists {
		t.Errorf("remove1 should have been deleted")
	}
	if _, exists := all["remove2"]; exists {
		t.Errorf("remove2 should have been deleted")
	}
}

func TestProjectFindingsDeleteNonExistent(t *testing.T) {
	env := NewTestEnv(t)

	// Delete non-existent key is no-op
	var result map[string]interface{}
	env.MustExecute(t, "project_findings.delete", map[string]interface{}{
		"keys": []string{"does-not-exist"},
	}, &result)

	// Should return nil or empty list
	if result["deleted"] != nil {
		deleted, ok := result["deleted"].([]interface{})
		if !ok || len(deleted) != 0 {
			t.Errorf("expected empty deleted list for non-existent key, got %v", result["deleted"])
		}
	}
}

func TestProjectFindingsWSBroadcast(t *testing.T) {
	env := NewTestEnv(t)

	// Create WS client subscribed to project (empty ticket_id for project-level)
	_, ch := env.NewWSClient(t, "ws-test-1", "")
	drainChannel(ch) // Clear any initial messages

	// Add triggers broadcast
	env.MustExecute(t, "project_findings.add", map[string]interface{}{
		"key":   "test",
		"value": `"value"`,
	}, nil)

	event := expectEvent(t, ch, ws.EventProjectFindingsUpdated, 2*time.Second)
	if event.Type != ws.EventProjectFindingsUpdated {
		t.Errorf("expected %s, got %s", ws.EventProjectFindingsUpdated, event.Type)
	}

	// Append triggers broadcast
	env.MustExecute(t, "project_findings.append", map[string]interface{}{
		"key":   "test",
		"value": `"value2"`,
	}, nil)

	event = expectEvent(t, ch, ws.EventProjectFindingsUpdated, 2*time.Second)
	if event.Type != ws.EventProjectFindingsUpdated {
		t.Errorf("expected %s, got %s", ws.EventProjectFindingsUpdated, event.Type)
	}

	// Delete triggers broadcast
	env.MustExecute(t, "project_findings.delete", map[string]interface{}{
		"keys": []string{"test"},
	}, nil)

	event = expectEvent(t, ch, ws.EventProjectFindingsUpdated, 2*time.Second)
	if event.Type != ws.EventProjectFindingsUpdated {
		t.Errorf("expected %s, got %s", ws.EventProjectFindingsUpdated, event.Type)
	}
}

func TestProjectFindingsJSONNormalization(t *testing.T) {
	env := NewTestEnv(t)

	// Add raw string (not valid JSON)
	env.MustExecute(t, "project_findings.add", map[string]interface{}{
		"key":   "raw",
		"value": `plain text`,
	}, nil)

	// Should be stored as JSON string and retrieved
	var result interface{}
	env.MustExecute(t, "project_findings.get", map[string]interface{}{
		"key": "raw",
	}, &result)

	if result != "plain text" {
		t.Errorf("expected 'plain text', got %v", result)
	}

	// Add valid JSON object
	env.MustExecute(t, "project_findings.add", map[string]interface{}{
		"key":   "obj",
		"value": `{"nested": "value"}`,
	}, nil)

	env.MustExecute(t, "project_findings.get", map[string]interface{}{
		"key": "obj",
	}, &result)

	obj, ok := result.(map[string]interface{})
	if !ok {
		t.Fatalf("expected object, got %T: %v", result, result)
	}
	if obj["nested"] != "value" {
		t.Errorf("expected nested='value', got %v", obj["nested"])
	}
}
