package spawner

import "testing"

// TestCodexFirstTurnText covers codexFirstTurnText's pure first-turn prefix
// logic without spawning a real codex app-server: systemPrompt/seededContext
// combinations on the first turn, retry-safety (firstTurnSent stays false
// until turn/start succeeds, so a retried first call must re-prepend), and
// later turns returning text unchanged.
func TestCodexFirstTurnText(t *testing.T) {
	tests := []struct {
		name          string
		text          string
		systemPrompt  string
		seededContext string
		firstTurnSent bool
		want          string
	}{
		{
			name:          "first turn, no system prompt, no seed",
			text:          "hello",
			firstTurnSent: false,
			want:          "hello",
		},
		{
			name:          "first turn, system prompt only",
			text:          "hello",
			systemPrompt:  "SYS",
			firstTurnSent: false,
			want:          "SYS\n\nhello",
		},
		{
			name:          "first turn, seed only",
			text:          "hello",
			seededContext: "SEED",
			firstTurnSent: false,
			want:          "SEED\n\nhello",
		},
		{
			name:          "first turn, system prompt and seed both present",
			text:          "hello",
			systemPrompt:  "SYS",
			seededContext: "SEED",
			firstTurnSent: false,
			want:          "SYS\n\nSEED\n\nhello",
		},
		{
			name:          "later turn ignores system prompt and seed",
			text:          "hello",
			systemPrompt:  "SYS",
			seededContext: "SEED",
			firstTurnSent: true,
			want:          "hello",
		},
		{
			name:          "retry of a not-yet-committed first turn re-prepends",
			text:          "hello",
			systemPrompt:  "SYS",
			seededContext: "SEED",
			firstTurnSent: false, // simulates a failed first turn/start: never flipped to true
			want:          "SYS\n\nSEED\n\nhello",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := codexFirstTurnText(tt.text, tt.systemPrompt, tt.seededContext, tt.firstTurnSent)
			if got != tt.want {
				t.Errorf("codexFirstTurnText(%q, %q, %q, %v) = %q, want %q",
					tt.text, tt.systemPrompt, tt.seededContext, tt.firstTurnSent, got, tt.want)
			}
		})
	}
}
