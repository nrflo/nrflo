package migrations

import (
	"context"

	"be/internal/nrvapp/configmigrate"
)

func init() {
	configmigrate.Register(1, "initial", initial)
}

// initial is a no-op anchor migration that advances the pointer from 0 to 1.
// Add subsequent migrations by calling configmigrate.Register with higher version numbers.
func initial(_ context.Context, _ configmigrate.Deps) error {
	return nil
}
