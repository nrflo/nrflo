package spawner

import (
	"strings"
	"testing"
)

func TestMintSpawnToken_LengthAndUniqueness(t *testing.T) {
	t.Parallel()
	seen := make(map[string]struct{}, 100)
	for i := 0; i < 100; i++ {
		tok := MintSpawnToken()
		if len(tok) != 64 {
			t.Fatalf("token length = %d, want 64 (hex of 32 bytes)", len(tok))
		}
		if strings.ContainsAny(tok, "ghijklmnopqrstuvwxyz") {
			t.Fatalf("token contains non-hex chars: %q", tok)
		}
		if _, dup := seen[tok]; dup {
			t.Fatalf("duplicate token after %d mints: %q", i, tok)
		}
		seen[tok] = struct{}{}
	}
}
