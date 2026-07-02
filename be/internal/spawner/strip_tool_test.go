package spawner

import (
	"testing"

	"be/internal/spawner/apirun"
	"be/internal/spawner/apirun/provider"
)

func TestStripTool(t *testing.T) {
	specs := []provider.ToolSpec{{Name: "a"}, {Name: "run_subworkflow"}, {Name: "b"}}
	handlers := apirun.Registry{"a": nil, "run_subworkflow": nil, "b": nil}

	out := stripTool(specs, handlers, "run_subworkflow")

	if _, ok := handlers["run_subworkflow"]; ok {
		t.Error("handler not removed from registry")
	}
	if len(out) != 2 {
		t.Fatalf("specs len = %d, want 2", len(out))
	}
	for _, s := range out {
		if s.Name == "run_subworkflow" {
			t.Error("spec not removed")
		}
	}
}
