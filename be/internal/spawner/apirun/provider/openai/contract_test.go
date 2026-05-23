//go:build openai_live

package openai_test

import (
	"context"
	"os"
	"testing"

	oaiprovider "be/internal/spawner/apirun/provider/openai"
)

// TestLiveRun is a smoke test against the real OpenAI Responses API.
// Run with: go test -tags openai_live -run TestLiveRun ./internal/spawner/apirun/provider/openai/
func TestLiveRun(t *testing.T) {
	key := os.Getenv("OPENAI_API_KEY")
	if key == "" {
		t.Skip("OPENAI_API_KEY not set")
	}
	_ = oaiprovider.New(oaiprovider.Credentials{Value: key})
	// Live test body is intentionally minimal; add assertions as needed.
	_ = context.Background()
}
