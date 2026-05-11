package spec_import

import (
	"context"
	"testing"
)

func TestMarkdownAdapter_Fetch(t *testing.T) {
	a := &MarkdownAdapter{}
	spec, err := a.Fetch(context.Background(), Input{Body: "# foo"})
	if err != nil {
		t.Fatalf("Fetch error: %v", err)
	}
	if spec.RawText != "# foo" {
		t.Errorf("RawText = %q, want %q", spec.RawText, "# foo")
	}
	if len(spec.AttachedRefs) != 0 {
		t.Errorf("AttachedRefs = %v, want empty", spec.AttachedRefs)
	}
}
