package usagelimits

import (
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/model"
)

// mockStore is an in-memory Store implementation for persistence tests.
type mockStore struct {
	mu     sync.Mutex
	data   map[string]mockEntry
	setErr error
	getErr error
	notify chan struct{}
}

type mockEntry struct {
	value     string
	updatedAt time.Time
}

func newMockStore() *mockStore {
	return &mockStore{
		data:   make(map[string]mockEntry),
		notify: make(chan struct{}, 10),
	}
}

func (m *mockStore) Get(name string) (*model.Preference, error) {
	if m.getErr != nil {
		return nil, m.getErr
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	e, ok := m.data[name]
	if !ok {
		return nil, nil
	}
	return &model.Preference{
		Name:      name,
		Value:     e.value,
		UpdatedAt: e.updatedAt,
	}, nil
}

func (m *mockStore) Set(name, value string) error {
	// Signal first so waitSet unblocks even when setErr is set.
	select {
	case m.notify <- struct{}{}:
	default:
	}
	if m.setErr != nil {
		return m.setErr
	}
	m.mu.Lock()
	m.data[name] = mockEntry{value: value, updatedAt: time.Now()}
	m.mu.Unlock()
	return nil
}

// waitSet blocks until store.Set is called or the test times out.
func (m *mockStore) waitSet(t *testing.T) {
	t.Helper()
	select {
	case <-m.notify:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for store.Set to be called")
	}
}

func (m *mockStore) getValue(name string) (string, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	e, ok := m.data[name]
	return e.value, ok
}

func (m *mockStore) putEntry(name, value string, updatedAt time.Time) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.data[name] = mockEntry{value: value, updatedAt: updatedAt}
}

// ---- Persistence tests ----

func TestCache_SetPersistsToStore(t *testing.T) {
	store := newMockStore()
	clk := clock.NewTest(time.Now())
	c := NewCache(store, clk)

	data := &UsageLimits{
		Claude:    ToolUsage{Available: true, Session: &UsageMetric{UsedPct: 75.0, ResetsAt: "in 1h"}},
		Codex:     ToolUsage{Available: false, Error: "not installed"},
		FetchedAt: time.Date(2025, 1, 15, 10, 0, 0, 0, time.UTC),
	}

	c.Set(data)
	store.waitSet(t) // wait for async goroutine to complete

	raw, ok := store.getValue(preferenceKey)
	if !ok {
		t.Fatal("store missing 'usage_limits' key after Set()")
	}

	var got UsageLimits
	if err := json.Unmarshal([]byte(raw), &got); err != nil {
		t.Fatalf("unmarshal stored JSON: %v", err)
	}
	if !got.Claude.Available {
		t.Error("Claude.Available = false, want true")
	}
	if got.Claude.Session == nil {
		t.Fatal("Claude.Session is nil after round-trip")
	}
	if got.Claude.Session.UsedPct != 75.0 {
		t.Errorf("Claude.Session.UsedPct = %v, want 75.0", got.Claude.Session.UsedPct)
	}
	if got.Claude.Session.ResetsAt != "in 1h" {
		t.Errorf("Claude.Session.ResetsAt = %q, want 'in 1h'", got.Claude.Session.ResetsAt)
	}
	if got.Codex.Error != "not installed" {
		t.Errorf("Codex.Error = %q, want 'not installed'", got.Codex.Error)
	}
}

func TestCache_SetNilDataDoesNotPersist(t *testing.T) {
	store := newMockStore()
	clk := clock.NewTest(time.Now())
	c := NewCache(store, clk)

	// cache.go returns early before spawning a goroutine when data is nil.
	c.Set(nil)

	if _, ok := store.getValue(preferenceKey); ok {
		t.Error("Set(nil) must not write to store")
	}
}

func TestCache_SetStoreErrorDoesNotPanic(t *testing.T) {
	store := newMockStore()
	store.setErr = errors.New("DB write failed")
	clk := clock.NewTest(time.Now())
	c := NewCache(store, clk)

	data := &UsageLimits{Claude: ToolUsage{Available: true}}
	c.Set(data) // must not panic; store error is logged internally

	// Memory cache should still be updated despite store error.
	store.waitSet(t) // wait for goroutine — it calls Set (which returns the error)
	if c.Get() != data {
		t.Error("in-memory cache not updated when store.Set returns an error")
	}
}

// ---- LoadFromDB tests ----

func TestCache_LoadFromDB_NilStore(t *testing.T) {
	c := NewCache(nil, nil)
	if c.LoadFromDB() {
		t.Error("LoadFromDB() with nil store should return false")
	}
	if c.Get() != nil {
		t.Error("cache should remain nil when store is nil")
	}
}

func TestCache_LoadFromDB_NoData(t *testing.T) {
	store := newMockStore()
	clk := clock.NewTest(time.Now())
	c := NewCache(store, clk)

	if c.LoadFromDB() {
		t.Error("LoadFromDB() with empty store should return false")
	}
	if c.Get() != nil {
		t.Error("cache should remain nil when store has no data")
	}
}

func TestCache_LoadFromDB_FreshData(t *testing.T) {
	store := newMockStore()
	now := time.Date(2025, 1, 15, 12, 0, 0, 0, time.UTC)
	clk := clock.NewTest(now)
	c := NewCache(store, clk)

	// Data written 10 minutes ago — within the 30-minute freshness threshold.
	limits := &UsageLimits{
		Claude:    ToolUsage{Available: true, Session: &UsageMetric{UsedPct: 50.0, ResetsAt: "in 3h"}},
		Codex:     ToolUsage{Available: false},
		FetchedAt: now.Add(-10 * time.Minute),
	}
	b, err := json.Marshal(limits)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	store.putEntry(preferenceKey, string(b), now.Add(-10*time.Minute))

	if !c.LoadFromDB() {
		t.Error("LoadFromDB() with fresh data (10 min old) should return true")
	}

	got := c.Get()
	if got == nil {
		t.Fatal("cache should be populated after LoadFromDB() with fresh data")
	}
	if !got.Claude.Available {
		t.Error("Claude.Available = false, want true")
	}
	if got.Claude.Session == nil {
		t.Fatal("Claude.Session is nil")
	}
	if got.Claude.Session.UsedPct != 50.0 {
		t.Errorf("Claude.Session.UsedPct = %v, want 50.0", got.Claude.Session.UsedPct)
	}
}

func TestCache_LoadFromDB_StaleData(t *testing.T) {
	store := newMockStore()
	now := time.Date(2025, 1, 15, 12, 0, 0, 0, time.UTC)
	clk := clock.NewTest(now)
	c := NewCache(store, clk)

	// Data written 45 minutes ago — past the 30-minute threshold.
	limits := &UsageLimits{Claude: ToolUsage{Available: true}}
	b, _ := json.Marshal(limits)
	store.putEntry(preferenceKey, string(b), now.Add(-45*time.Minute))

	if c.LoadFromDB() {
		t.Error("LoadFromDB() with stale data (45 min old) should return false")
	}
	if c.Get() != nil {
		t.Error("cache should remain nil when DB data is stale")
	}
}

func TestCache_LoadFromDB_ExactlyAtThreshold(t *testing.T) {
	store := newMockStore()
	now := time.Date(2025, 1, 15, 12, 0, 0, 0, time.UTC)
	clk := clock.NewTest(now)
	c := NewCache(store, clk)

	// Data age == 30 min exactly: age > stalenessThreshold is false, so this
	// is NOT stale and should be loaded. Let's verify boundary behaviour.
	limits := &UsageLimits{Claude: ToolUsage{Available: true}}
	b, _ := json.Marshal(limits)
	store.putEntry(preferenceKey, string(b), now.Add(-30*time.Minute))

	// age = 30min, threshold = 30min: age > threshold is false → fresh.
	if !c.LoadFromDB() {
		t.Error("LoadFromDB() with data exactly at 30-min threshold should return true (age not > threshold)")
	}
}

func TestCache_LoadFromDB_InvalidJSON(t *testing.T) {
	store := newMockStore()
	now := time.Date(2025, 1, 15, 12, 0, 0, 0, time.UTC)
	clk := clock.NewTest(now)
	c := NewCache(store, clk)

	// Fresh timestamp, but corrupt JSON value.
	store.putEntry(preferenceKey, `{"invalid json`, now.Add(-5*time.Minute))

	if c.LoadFromDB() {
		t.Error("LoadFromDB() with invalid JSON should return false")
	}
	if c.Get() != nil {
		t.Error("cache should remain nil when JSON is invalid")
	}
}

func TestCache_LoadFromDB_StoreError(t *testing.T) {
	store := newMockStore()
	store.getErr = errors.New("DB connection refused")
	clk := clock.NewTest(time.Now())
	c := NewCache(store, clk)

	if c.LoadFromDB() {
		t.Error("LoadFromDB() with store.Get error should return false")
	}
	if c.Get() != nil {
		t.Error("cache should remain nil when store.Get returns an error")
	}
}
