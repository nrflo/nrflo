package spawner

import (
	"strings"
	"testing"
)

func TestBuildSavePrompt(t *testing.T) {
	prompt := buildSavePrompt()

	if !strings.Contains(prompt, "nrflo findings add to_resume") {
		t.Error("save prompt should contain 'nrflo findings add to_resume' instruction")
	}
	if !strings.Contains(prompt, "nrflo agent continue") {
		t.Error("save prompt should contain 'nrflo agent continue' instruction")
	}
	if !strings.Contains(prompt, "URGENT") {
		t.Error("save prompt should start with URGENT")
	}
}
