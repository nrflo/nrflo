package spawner

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"be/internal/service"
	"be/internal/spawner/apirun/provider"
	"be/internal/spawner/apirun/provider/mock"
)

func writeFileToolUse(id, path, content string) provider.ContentBlock {
	return provider.ContentBlock{
		Type:      "tool_use",
		ToolUseID: id,
		ToolName:  "write_file",
		Input:     []byte(`{"path":"` + path + `","content":"` + content + `"}`),
	}
}

// TestAPIConsoleEngine_FSTools_WriteFile_ApprovalRoundtrip verifies write_file
// is gated like edit_file/bash: the target file must not exist until the
// human approves, and an allow lets the write through with a non-error
// tool_result.
func TestAPIConsoleEngine_FSTools_WriteFile_ApprovalRoundtrip(t *testing.T) {
	pool, clk := newAPIEngineTestPool(t)
	settings := service.NewGlobalSettingsService(pool, clk)
	if err := settings.Set("api_mode_enabled", "true"); err != nil {
		t.Fatal(err)
	}
	if err := settings.Set("api_native_tools_enabled", "true"); err != nil {
		t.Fatal(err)
	}

	workDir := t.TempDir()
	target := filepath.Join(workDir, "out.txt")

	prov := newRecordingSpawnerProvider(
		mock.Script{Final: provider.FinalResponse{
			StopReason: "tool_use",
			Content:    []provider.ContentBlock{writeFileToolUse("tu-wf-1", "out.txt", "hello")},
		}},
		mock.Script{Final: provider.FinalResponse{
			StopReason: "end_turn",
			Content:    []provider.ContentBlock{{Type: "text", Text: "done"}},
		}},
	)
	installFakeAPIProvider(t, prov, nil)

	sink := &testSink{}
	eng := newAPIConsoleEngine(EngineDeps{Sink: sink, API: APIEngineDeps{Pool: pool, Clock: clk}})
	if err := eng.Start(context.Background(), fsTestSpec("sess-fs-write-appr", workDir)); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(eng.Stop)

	if err := eng.SendUserTurn(context.Background(), UserTurn{Text: "write it"}); err != nil {
		t.Fatalf("SendUserTurn: %v", err)
	}

	req := waitForEventType(t, eng.Events(), EventApprovalRequest, 5*time.Second)
	if req.Approval == nil || req.ToolName != "write_file" {
		t.Fatalf("approval request = %+v, want write_file", req)
	}

	// Blocked until decision: the write must not have happened yet.
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Fatalf("target file exists before approval decision: err=%v", err)
	}

	if err := eng.ReplyApproval(req.Approval.ID, ApprovalApprove); err != nil {
		t.Fatalf("ReplyApproval: %v", err)
	}

	resolved := waitForEventType(t, eng.Events(), EventApprovalResolved, 5*time.Second)
	if resolved.Decision != ApprovalApprove {
		t.Errorf("resolved decision = %q, want approve", resolved.Decision)
	}
	waitForEventType(t, eng.Events(), EventTurnCompleted, 5*time.Second)

	data, err := os.ReadFile(target)
	if err != nil || string(data) != "hello" {
		t.Errorf("target file content = %q err=%v, want \"hello\"", data, err)
	}

	reqs := prov.Requests()
	if len(reqs) != 2 {
		t.Fatalf("provider calls = %d, want 2", len(reqs))
	}
	last := reqs[1].Messages[len(reqs[1].Messages)-1]
	if len(last.Content) != 1 || last.Content[0].IsError {
		t.Errorf("tool_result = %+v, want non-error result", last.Content)
	}
}

// TestAPIConsoleEngine_FSTools_WriteFile_DenyBlocksExecution verifies a
// denied write_file call never touches the filesystem and returns a
// denied-by-user tool error.
func TestAPIConsoleEngine_FSTools_WriteFile_DenyBlocksExecution(t *testing.T) {
	pool, clk := newAPIEngineTestPool(t)
	settings := service.NewGlobalSettingsService(pool, clk)
	if err := settings.Set("api_mode_enabled", "true"); err != nil {
		t.Fatal(err)
	}
	if err := settings.Set("api_native_tools_enabled", "true"); err != nil {
		t.Fatal(err)
	}

	workDir := t.TempDir()
	target := filepath.Join(workDir, "out.txt")

	prov := newRecordingSpawnerProvider(
		mock.Script{Final: provider.FinalResponse{
			StopReason: "tool_use",
			Content:    []provider.ContentBlock{writeFileToolUse("tu-wf-2", "out.txt", "hello")},
		}},
		mock.Script{Final: provider.FinalResponse{StopReason: "end_turn"}},
	)
	installFakeAPIProvider(t, prov, nil)

	eng := newAPIConsoleEngine(EngineDeps{Sink: &testSink{}, API: APIEngineDeps{Pool: pool, Clock: clk}})
	if err := eng.Start(context.Background(), fsTestSpec("sess-fs-write-deny", workDir)); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(eng.Stop)

	if err := eng.SendUserTurn(context.Background(), UserTurn{Text: "write it"}); err != nil {
		t.Fatalf("SendUserTurn: %v", err)
	}
	req := waitForEventType(t, eng.Events(), EventApprovalRequest, 5*time.Second)
	if err := eng.ReplyApproval(req.Approval.ID, ApprovalDeny); err != nil {
		t.Fatalf("ReplyApproval: %v", err)
	}
	waitForEventType(t, eng.Events(), EventTurnCompleted, 5*time.Second)

	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Errorf("target file was created despite deny: err=%v", err)
	}

	reqs := prov.Requests()
	last := reqs[1].Messages[len(reqs[1].Messages)-1]
	if !last.Content[0].IsError || !strings.Contains(last.Content[0].Output, "denied by user") {
		t.Errorf("tool_result = %+v, want denied-by-user error", last.Content)
	}
}
