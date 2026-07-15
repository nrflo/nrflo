package cli

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"be/internal/model"
)

// resolveCLIModel resolves a --model flag value (a cli_models registry id,
// case-insensitive) against GET /api/v1/cli-models — a `protected` route. It
// runs after the session is open, so `do` uses the console session bearer
// (which satisfies requireAuth), falling back to the service token; either way
// no token is required beyond what opened the session. The caller skips this
// entirely for an empty --model ("omit the flag, let the CLI use its own default").
//
// Three outcomes:
//   - a matching enabled row for cliType -> that row; its mapped_model,
//     reasoning_effort and fallback_models all flow to the driver.
//   - no row with this id at all -> (nil, nil): a raw/unknown id is still a
//     legal CLI model name, so the driver falls back to its own
//     adapter.MapModel(id). Never an error.
//   - a row exists but belongs to another cli_type, or is disabled -> an
//     error, before Probe/session-create. The id is demonstrably a registry id
//     the user got wrong; passing it through verbatim would surface as an
//     opaque provider error inside the TUI with a console session already open.
//
// err is also returned for a registry-fetch transport/decode failure.
func (c *nrfloHTTPClient) resolveCLIModel(ctx context.Context, cliType, id string) (*model.CLIModel, error) {
	var models []model.CLIModel
	if err := c.do(ctx, "", http.MethodGet, "/api/v1/cli-models", nil, &models); err != nil {
		return nil, fmt.Errorf("fetch cli-models registry: %w", err)
	}
	for i := range models {
		m := &models[i]
		if !strings.EqualFold(m.ID, id) {
			continue
		}
		if m.CLIType != cliType {
			return nil, fmt.Errorf("model %q is registered for --cli %s, not %s", id, m.CLIType, cliType)
		}
		if !m.Enabled {
			return nil, fmt.Errorf("model %q is disabled in the cli_models registry", id)
		}
		return m, nil
	}
	return nil, nil
}
