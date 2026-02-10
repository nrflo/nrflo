package integration

import (
	"testing"
)

func TestFindingsAddAndGet(t *testing.T) {
	env := NewTestEnv(t)

	env.CreateTicket(t, "FIND-1", "Findings test")
	env.InitWorkflow(t, "FIND-1")

	wfiID := env.GetWorkflowInstanceID(t, "FIND-1", "test")
	env.InsertAgentSession(t, "sess-find-1", "FIND-1", wfiID, "analyzer", "analyzer", "")

	// Add finding with JSON array value
	env.MustExecute(t, "findings.add", map[string]interface{}{
		"ticket_id":  "FIND-1",
		"workflow":   "test",
		"agent_type": "analyzer",
		"key":        "issues",
		"value":      `["issue1", "issue2"]`,
	}, nil)

	// Get by agent_type (all findings)
	var allFindings map[string]interface{}
	env.MustExecute(t, "findings.get", map[string]interface{}{
		"ticket_id":  "FIND-1",
		"workflow":   "test",
		"agent_type": "analyzer",
	}, &allFindings)

	issues, ok := allFindings["issues"].([]interface{})
	if !ok {
		t.Fatalf("expected issues array, got %T: %v", allFindings["issues"], allFindings)
	}
	if len(issues) != 2 {
		t.Fatalf("expected 2 issues, got %d", len(issues))
	}

	// Get by specific key
	var keyResult interface{}
	env.MustExecute(t, "findings.get", map[string]interface{}{
		"ticket_id":  "FIND-1",
		"workflow":   "test",
		"agent_type": "analyzer",
		"key":        "issues",
	}, &keyResult)

	arr, ok := keyResult.([]interface{})
	if !ok {
		t.Fatalf("expected array for key result, got %T", keyResult)
	}
	if len(arr) != 2 || arr[0] != "issue1" {
		t.Fatalf("unexpected key result: %v", arr)
	}
}

func TestFindingsAppend(t *testing.T) {
	env := NewTestEnv(t)

	env.CreateTicket(t, "FIND-2", "Append test")
	env.InitWorkflow(t, "FIND-2")

	wfiID := env.GetWorkflowInstanceID(t, "FIND-2", "test")
	env.InsertAgentSession(t, "sess-append-1", "FIND-2", wfiID, "analyzer", "analyzer", "")

	// Add initial value
	env.MustExecute(t, "findings.add", map[string]interface{}{
		"ticket_id":  "FIND-2",
		"workflow":   "test",
		"agent_type": "analyzer",
		"key":        "items",
		"value":      `"v1"`,
	}, nil)

	// Append
	env.MustExecute(t, "findings.append", map[string]interface{}{
		"ticket_id":  "FIND-2",
		"workflow":   "test",
		"agent_type": "analyzer",
		"key":        "items",
		"value":      `"v2"`,
	}, nil)

	// Verify array result
	var result interface{}
	env.MustExecute(t, "findings.get", map[string]interface{}{
		"ticket_id":  "FIND-2",
		"workflow":   "test",
		"agent_type": "analyzer",
		"key":        "items",
	}, &result)

	arr, ok := result.([]interface{})
	if !ok {
		t.Fatalf("expected array, got %T: %v", result, result)
	}
	if len(arr) != 2 || arr[0] != "v1" || arr[1] != "v2" {
		t.Fatalf("expected [v1, v2], got %v", arr)
	}

	// Append again
	env.MustExecute(t, "findings.append", map[string]interface{}{
		"ticket_id":  "FIND-2",
		"workflow":   "test",
		"agent_type": "analyzer",
		"key":        "items",
		"value":      `"v3"`,
	}, nil)

	env.MustExecute(t, "findings.get", map[string]interface{}{
		"ticket_id":  "FIND-2",
		"workflow":   "test",
		"agent_type": "analyzer",
		"key":        "items",
	}, &result)

	arr, ok = result.([]interface{})
	if !ok || len(arr) != 3 {
		t.Fatalf("expected 3 items, got %v", result)
	}
}

func TestFindingsAddBulk(t *testing.T) {
	env := NewTestEnv(t)

	env.CreateTicket(t, "FIND-3", "Bulk add test")
	env.InitWorkflow(t, "FIND-3")

	wfiID := env.GetWorkflowInstanceID(t, "FIND-3", "test")
	env.InsertAgentSession(t, "sess-bulk-1", "FIND-3", wfiID, "analyzer", "analyzer", "")

	// Add 3 key-value pairs at once
	env.MustExecute(t, "findings.add-bulk", map[string]interface{}{
		"ticket_id":  "FIND-3",
		"workflow":   "test",
		"agent_type": "analyzer",
		"key_values": map[string]string{
			"summary":    "All good",
			"score":      "95",
			"categories": `["cat1", "cat2"]`,
		},
	}, nil)

	// Verify all retrievable
	var all map[string]interface{}
	env.MustExecute(t, "findings.get", map[string]interface{}{
		"ticket_id":  "FIND-3",
		"workflow":   "test",
		"agent_type": "analyzer",
	}, &all)

	if all["summary"] != "All good" {
		t.Fatalf("expected summary 'All good', got %v", all["summary"])
	}
	if all["score"].(float64) != 95 {
		t.Fatalf("expected score 95, got %v", all["score"])
	}
	cats, ok := all["categories"].([]interface{})
	if !ok || len(cats) != 2 {
		t.Fatalf("expected categories array, got %v", all["categories"])
	}
}

func TestFindingsAppendBulk(t *testing.T) {
	env := NewTestEnv(t)

	env.CreateTicket(t, "FIND-4", "Append bulk")
	env.InitWorkflow(t, "FIND-4")

	wfiID := env.GetWorkflowInstanceID(t, "FIND-4", "test")
	env.InsertAgentSession(t, "sess-abk-1", "FIND-4", wfiID, "builder", "builder", "")

	// Add initial values
	env.MustExecute(t, "findings.add", map[string]interface{}{
		"ticket_id":  "FIND-4",
		"workflow":   "test",
		"agent_type": "builder",
		"key":        "files",
		"value":      `"main.go"`,
	}, nil)
	env.MustExecute(t, "findings.add", map[string]interface{}{
		"ticket_id":  "FIND-4",
		"workflow":   "test",
		"agent_type": "builder",
		"key":        "tests",
		"value":      `"main_test.go"`,
	}, nil)

	// Append bulk
	env.MustExecute(t, "findings.append-bulk", map[string]interface{}{
		"ticket_id":  "FIND-4",
		"workflow":   "test",
		"agent_type": "builder",
		"key_values": map[string]string{
			"files": `"util.go"`,
			"tests": `"util_test.go"`,
		},
	}, nil)

	// Verify arrays
	var all map[string]interface{}
	env.MustExecute(t, "findings.get", map[string]interface{}{
		"ticket_id":  "FIND-4",
		"workflow":   "test",
		"agent_type": "builder",
	}, &all)

	files, ok := all["files"].([]interface{})
	if !ok || len(files) != 2 {
		t.Fatalf("expected 2 files, got %v", all["files"])
	}
	tests, ok := all["tests"].([]interface{})
	if !ok || len(tests) != 2 {
		t.Fatalf("expected 2 tests, got %v", all["tests"])
	}
}

func TestFindingsDelete(t *testing.T) {
	env := NewTestEnv(t)

	env.CreateTicket(t, "FIND-5", "Delete findings")
	env.InitWorkflow(t, "FIND-5")

	wfiID := env.GetWorkflowInstanceID(t, "FIND-5", "test")
	env.InsertAgentSession(t, "sess-del-1", "FIND-5", wfiID, "analyzer", "analyzer", "")

	// Add 3 findings
	env.MustExecute(t, "findings.add-bulk", map[string]interface{}{
		"ticket_id":  "FIND-5",
		"workflow":   "test",
		"agent_type": "analyzer",
		"key_values": map[string]string{
			"keep":    "important",
			"remove1": "temp",
			"remove2": "temp",
		},
	}, nil)

	// Delete 2
	var delResult map[string]interface{}
	env.MustExecute(t, "findings.delete", map[string]interface{}{
		"ticket_id":  "FIND-5",
		"workflow":   "test",
		"agent_type": "analyzer",
		"keys":       []string{"remove1", "remove2"},
	}, &delResult)

	if delResult["deleted"].(float64) != 2 {
		t.Fatalf("expected 2 deleted, got %v", delResult["deleted"])
	}

	// Verify only 1 remains
	var all map[string]interface{}
	env.MustExecute(t, "findings.get", map[string]interface{}{
		"ticket_id":  "FIND-5",
		"workflow":   "test",
		"agent_type": "analyzer",
	}, &all)

	if all["keep"] != "important" {
		t.Fatalf("expected keep='important', got %v", all["keep"])
	}
	if _, exists := all["remove1"]; exists {
		t.Fatal("remove1 should have been deleted")
	}
}

func TestFindingsWithModel(t *testing.T) {
	env := NewTestEnv(t)

	env.CreateTicket(t, "FIND-6", "Model findings")
	env.InitWorkflow(t, "FIND-6")

	wfiID := env.GetWorkflowInstanceID(t, "FIND-6", "test")
	env.InsertAgentSession(t, "sess-model-s", "FIND-6", wfiID, "analyzer", "analyzer", "sonnet")
	env.InsertAgentSession(t, "sess-model-o", "FIND-6", wfiID, "analyzer", "analyzer", "opus")

	// Add findings with model "sonnet"
	env.MustExecute(t, "findings.add", map[string]interface{}{
		"ticket_id":  "FIND-6",
		"workflow":   "test",
		"agent_type": "analyzer",
		"key":        "result",
		"value":      `"sonnet-result"`,
		"model":      "sonnet",
	}, nil)

	// Add findings with model "opus"
	env.MustExecute(t, "findings.add", map[string]interface{}{
		"ticket_id":  "FIND-6",
		"workflow":   "test",
		"agent_type": "analyzer",
		"key":        "result",
		"value":      `"opus-result"`,
		"model":      "opus",
	}, nil)

	// Get with specific model
	var sonnetResult interface{}
	env.MustExecute(t, "findings.get", map[string]interface{}{
		"ticket_id":  "FIND-6",
		"workflow":   "test",
		"agent_type": "analyzer",
		"key":        "result",
		"model":      "sonnet",
	}, &sonnetResult)
	if sonnetResult != "sonnet-result" {
		t.Fatalf("expected 'sonnet-result', got %v", sonnetResult)
	}

	// Get without model (should return grouped by model since 2 models exist)
	var grouped interface{}
	env.MustExecute(t, "findings.get", map[string]interface{}{
		"ticket_id":  "FIND-6",
		"workflow":   "test",
		"agent_type": "analyzer",
	}, &grouped)

	groupedMap, ok := grouped.(map[string]interface{})
	if !ok {
		t.Fatalf("expected grouped map, got %T: %v", grouped, grouped)
	}
	if len(groupedMap) < 2 {
		t.Fatalf("expected at least 2 model groups, got %d: %v", len(groupedMap), groupedMap)
	}
}
