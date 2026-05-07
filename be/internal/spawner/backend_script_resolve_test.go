package spawner

import "testing"

// TestResolvePythonBin verifies that resolvePythonBin returns the prep.pythonPath
// when set, and falls back to "python3" when empty.
func TestResolvePythonBin(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		pythonPath string
		want       string
	}{
		{
			name:       "empty pythonPath falls back to python3",
			pythonPath: "",
			want:       "python3",
		},
		{
			name:       "venv path returned as-is",
			pythonPath: "/tmp/venv/bin/python3",
			want:       "/tmp/venv/bin/python3",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			prep := &prepResult{pythonPath: tt.pythonPath}
			got := resolvePythonBin(prep)
			if got != tt.want {
				t.Errorf("resolvePythonBin(%q) = %q, want %q", tt.pythonPath, got, tt.want)
			}
		})
	}
}
