package migrations

import (
	"context"

	"be/internal/configeditor/migrate"
)

func init() {
	migrate.Register(1, "initial", initial)
}

// initial is a no-op anchor migration that advances the pointer from 0 to 1.
// Add subsequent migrations by calling migrate.Register with higher version numbers.
func initial(_ context.Context, _ migrate.Deps) error {
	return nil
}
