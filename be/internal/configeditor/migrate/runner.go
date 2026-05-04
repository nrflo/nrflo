package migrate

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"be/internal/model"
)

// configmigrateKey is the sentinel file used to track the applied migration pointer.
const configmigrateKey = "__configmigrate__"

// Run applies all registered migrations that are ahead of the stored pointer.
// It is idempotent: migrations already applied (version <= stored pointer) are skipped.
// The first migration error halts execution and returns that error.
func Run(ctx context.Context, dir string, deps Deps) error {
	migrations := List()

	pointer, err := readPointer(deps)
	if err != nil {
		return fmt.Errorf("read migration pointer: %w", err)
	}

	for _, m := range migrations {
		if m.Version <= pointer {
			continue
		}
		if err := m.Fn(ctx, deps); err != nil {
			return fmt.Errorf("migration %d (%s): %w", m.Version, m.Name, err)
		}
		if err := advancePointer(deps, m.Version); err != nil {
			return fmt.Errorf("advance pointer to %d: %w", m.Version, err)
		}
	}
	return nil
}

// readPointer retrieves the current migration level from the repo.
// Returns 0 if no migrations have been applied yet.
func readPointer(deps Deps) (int, error) {
	latestV, err := deps.repo.LatestVersion(deps.projectID, configmigrateKey)
	if err != nil {
		return 0, err
	}
	if latestV == 0 {
		return 0, nil
	}
	ver, err := deps.repo.Get(deps.projectID, configmigrateKey, latestV)
	if err != nil {
		return 0, err
	}
	level, err := strconv.Atoi(strings.TrimSpace(string(ver.Content)))
	if err != nil {
		return 0, fmt.Errorf("parse migration pointer content: %w", err)
	}
	return level, nil
}

// advancePointer records that migration level has been applied.
func advancePointer(deps Deps, level int) error {
	actor := "configmigrate"
	v := &model.ConfigVersion{
		ProjectID: deps.projectID,
		File:      configmigrateKey,
		Content:   []byte(strconv.Itoa(level)),
		Actor:     &actor,
	}
	return deps.repo.Insert(v)
}
