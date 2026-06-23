package apirun

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"be/internal/logger"
	"be/internal/spawner/apirun/provider"
	"be/internal/spawner/apirun/provider/mock"
)

// TestRunner_UsageLog_CacheHitMiss asserts updateContext emits a structured
// per-turn usage line carrying the cache_read/cache_creation split and a
// hit/miss flag — the only place those numbers surface (they are otherwise
// summed away into the context-left %).
func TestRunner_UsageLog_CacheHitMiss(t *testing.T) {
	cases := []struct {
		name      string
		usage     provider.Usage
		wantCache string
		wantParts []string
	}{
		{
			name:      "hit",
			usage:     provider.Usage{InputTokens: 20, OutputTokens: 5, CacheReadTokens: 80},
			wantCache: "cache=hit",
			// 80 read / (20+80+0) total = 80%
			wantParts: []string{"cache_read=80", "cache_creation=0", "cache_hit_pct=80", "input=20", "output=5", "session=sess-1"},
		},
		{
			name:      "miss",
			usage:     provider.Usage{InputTokens: 100, OutputTokens: 5, CacheCreationTokens: 50},
			wantCache: "cache=miss",
			// 0 read / (100+0+50) total = 0%
			wantParts: []string{"cache_read=0", "cache_creation=50", "cache_hit_pct=0"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			prev := logger.GetWriter()
			logger.SetWriter(&buf)
			defer logger.SetWriter(prev)

			prov := mock.New(mock.Script{
				Events: []mock.SinkEvent{{Kind: mock.EventUsage, Usage: tc.usage}},
				Final:  provider.FinalResponse{StopReason: "end_turn", Usage: tc.usage},
			})
			r := NewRunner(Config{
				Provider:      prov,
				Sink:          &recordingSink{},
				InitialPrompt: "go",
				MaxIterations: 2,
				MaxContext:    1000,
				Deadline:      time.Now().Add(5 * time.Second),
			})
			r.Run(context.Background(), newTestProc())

			out := buf.String()
			if !strings.Contains(out, "apirun turn usage") {
				t.Fatalf("usage line missing; log = %q", out)
			}
			if !strings.Contains(out, tc.wantCache) {
				t.Errorf("want %q in log; got %q", tc.wantCache, out)
			}
			for _, p := range tc.wantParts {
				if !strings.Contains(out, p) {
					t.Errorf("want %q in log; got %q", p, out)
				}
			}
		})
	}
}
