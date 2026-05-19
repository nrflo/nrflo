package service

import (
	"testing"

	"be/internal/clock"
)

func TestObserverSettings_EnabledDefaultFalse(t *testing.T) {
	t.Parallel()
	pool, _, _ := setupObserverTestEnv(t)
	gs := NewGlobalSettingsService(pool, clock.Real())

	enabled, err := gs.GetExperimentalObserverEnabled()
	if err != nil {
		t.Fatalf("GetExperimentalObserverEnabled: %v", err)
	}
	if enabled {
		t.Error("observer enabled by default, want false")
	}
}

func TestObserverSettings_SetAndGetEnabled(t *testing.T) {
	t.Parallel()
	pool, _, _ := setupObserverTestEnv(t)
	gs := NewGlobalSettingsService(pool, clock.Real())

	for _, want := range []bool{true, false, true} {
		if err := gs.SetExperimentalObserverEnabled(want); err != nil {
			t.Fatalf("set %v: %v", want, err)
		}
		got, err := gs.GetExperimentalObserverEnabled()
		if err != nil {
			t.Fatalf("get: %v", err)
		}
		if got != want {
			t.Errorf("enabled = %v, want %v", got, want)
		}
	}
}

func TestObserverSettings_GlobalAccessors(t *testing.T) {
	t.Parallel()
	pool, _, _ := setupObserverTestEnv(t)
	gs := NewGlobalSettingsService(pool, clock.Real())

	cases := []struct {
		name string
		set  func(string) error
		get  func() (string, error)
	}{
		{"system_context", gs.SetObserverSystemContext, gs.GetObserverSystemContext},
		{"provider", gs.SetObserverProvider, gs.GetObserverProvider},
		{"model", gs.SetObserverModel, gs.GetObserverModel},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			want := "val-" + tc.name
			if err := tc.set(want); err != nil {
				t.Fatalf("set %s: %v", tc.name, err)
			}
			got, err := tc.get()
			if err != nil {
				t.Fatalf("get %s: %v", tc.name, err)
			}
			if got != want {
				t.Errorf("%s = %q, want %q", tc.name, got, want)
			}
		})
	}
}

func TestObserverSettings_ProjectAccessors(t *testing.T) {
	t.Parallel()
	pool, _, _ := setupObserverTestEnv(t)
	gs := NewGlobalSettingsService(pool, clock.Real())

	cases := []struct {
		name string
		set  func(string) error
		get  func() (string, error)
	}{
		{
			name: "system_context",
			set:  func(v string) error { return gs.SetObserverSystemContextForProject("proj1", v) },
			get:  func() (string, error) { return gs.GetObserverSystemContextForProject("proj1") },
		},
		{
			name: "provider",
			set:  func(v string) error { return gs.SetObserverProviderForProject("proj1", v) },
			get:  func() (string, error) { return gs.GetObserverProviderForProject("proj1") },
		},
		{
			name: "model",
			set:  func(v string) error { return gs.SetObserverModelForProject("proj1", v) },
			get:  func() (string, error) { return gs.GetObserverModelForProject("proj1") },
		},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			want := "proj-" + tc.name
			if err := tc.set(want); err != nil {
				t.Fatalf("set %s: %v", tc.name, err)
			}
			got, err := tc.get()
			if err != nil {
				t.Fatalf("get %s: %v", tc.name, err)
			}
			if got != want {
				t.Errorf("%s = %q, want %q", tc.name, got, want)
			}
		})
	}
}

func TestObserverSettings_GlobalAndProjectAreIsolated(t *testing.T) {
	t.Parallel()
	pool, _, _ := setupObserverTestEnv(t)
	gs := NewGlobalSettingsService(pool, clock.Real())

	mustSet(t, gs.SetObserverSystemContext("global-ctx"))
	mustSet(t, gs.SetObserverSystemContextForProject("proj1", "proj-ctx"))

	global, err := gs.GetObserverSystemContext()
	if err != nil {
		t.Fatalf("get global: %v", err)
	}
	proj, err := gs.GetObserverSystemContextForProject("proj1")
	if err != nil {
		t.Fatalf("get project: %v", err)
	}
	if global != "global-ctx" {
		t.Errorf("global = %q, want global-ctx", global)
	}
	if proj != "proj-ctx" {
		t.Errorf("project = %q, want proj-ctx", proj)
	}
}
