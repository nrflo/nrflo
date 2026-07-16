package spawner

import (
	"context"
	"strings"
	"testing"
)

func TestCodexAppServerRequiresRegistryModelAndEffort(t *testing.T) {
	t.Parallel()
	backend := newCodexAppServerBackend(&Spawner{})
	proc := &processInfo{sessionID: "config-required"}

	err := backend.Start(context.Background(), proc, &prepResult{opts: SpawnOptions{ReasoningEffort: "high"}})
	if err == nil || !strings.Contains(err.Error(), "mapped model is required") {
		t.Fatalf("missing mapped model error = %v", err)
	}
	err = backend.Start(context.Background(), proc, &prepResult{opts: SpawnOptions{MappedModel: "gpt-5.6-sol"}})
	if err == nil || !strings.Contains(err.Error(), "reasoning effort is required") {
		t.Fatalf("missing effort error = %v", err)
	}
}
