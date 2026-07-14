package console

import (
	"context"
	"encoding/json"
	"errors"

	"be/internal/spawner/apirun"
)

// ErrToolNotFound is returned by Dispatch when name is not in reg.
var ErrToolNotFound = errors.New("console: tool not found")

// Dispatch is the single call site the HTTP handler uses to invoke a console
// tool: defaults empty args to "{}" and normalizes handler results.
func Dispatch(ctx context.Context, reg apirun.Registry, env apirun.ToolEnv, name string, args json.RawMessage) (output string, isError bool, err error) {
	h, ok := reg[name]
	if !ok {
		return "", false, ErrToolNotFound
	}
	if len(args) == 0 {
		args = json.RawMessage(`{}`)
	}
	return h.Invoke(ctx, env, args)
}
