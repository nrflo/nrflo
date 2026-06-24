package spawner

import (
	"testing"

	"be/internal/spawner/apirun"
	"be/internal/spawner/apirun/provider"
)

func TestStripTool(t *testing.T) {
	specs := []provider.ToolSpec{{Name: "a"}, {Name: "web_deep_research"}, {Name: "b"}}
	handlers := apirun.Registry{"a": nil, "web_deep_research": nil, "b": nil}

	out := stripTool(specs, handlers, "web_deep_research")

	if _, ok := handlers["web_deep_research"]; ok {
		t.Error("handler not removed from registry")
	}
	if len(out) != 2 {
		t.Fatalf("specs len = %d, want 2", len(out))
	}
	for _, s := range out {
		if s.Name == "web_deep_research" {
			t.Error("spec not removed")
		}
	}
}
