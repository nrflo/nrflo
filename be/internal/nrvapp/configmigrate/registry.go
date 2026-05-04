package configmigrate

import (
	"context"
	"fmt"
	"sort"
	"sync"
)

// MigrationFn is the function signature for a config migration.
type MigrationFn func(ctx context.Context, deps Deps) error

// Migration describes a single forward-only config migration.
type Migration struct {
	Version int
	Name    string
	Fn      MigrationFn
}

var (
	mu       sync.Mutex
	registry []Migration
)

// Register adds a migration to the global registry.
// Panics on duplicate version, zero version, or nil function.
func Register(version int, name string, fn MigrationFn) {
	mu.Lock()
	defer mu.Unlock()
	if version <= 0 {
		panic(fmt.Sprintf("configmigrate: Register called with zero/negative version %d", version))
	}
	if fn == nil {
		panic(fmt.Sprintf("configmigrate: Register called with nil fn for version %d", version))
	}
	for _, m := range registry {
		if m.Version == version {
			panic(fmt.Sprintf("configmigrate: duplicate version %d (existing: %q)", version, m.Name))
		}
	}
	registry = append(registry, Migration{Version: version, Name: name, Fn: fn})
}

// List returns a sorted copy of all registered migrations (ascending version).
func List() []Migration {
	mu.Lock()
	defer mu.Unlock()
	cp := make([]Migration, len(registry))
	copy(cp, registry)
	sort.Slice(cp, func(i, j int) bool { return cp[i].Version < cp[j].Version })
	return cp
}

// resetForTest clears the registry. Only for use in tests.
func resetForTest() {
	mu.Lock()
	defer mu.Unlock()
	registry = nil
}
