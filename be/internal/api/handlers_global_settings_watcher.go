package api

import (
	"encoding/json"
	"net/http"
	"strconv"

	"be/internal/service"
)

// watcherIntSettings are the context-watcher / proactive-restart tuning
// knobs stored as integer global config, keyed the same as the seeding
// migration (000187_watcher_defaults.up.sql) and the spawner code defaults
// they mirror (context_watcher.go, context_restart*.go).
var watcherIntSettings = []struct {
	key string
	def int
}{
	{"context_budget_default", 0},
	{"context_decay_turns", 20},
	{"cache_ttl_sec", 300},
	{"min_epoch_interval_calls", 20},
	{"proactive_restart_threshold_default", 250000},
	{"proactive_restart_min_interval_sec", 600},
	{"proactive_restart_max_per_session", 0},
	{"proactive_restart_boundary_window_turns", 10},
	{"proactive_restart_console_pct", 75},
}

// watcherFloatSettings are the float-valued watcher knobs.
var watcherFloatSettings = []struct {
	key string
	def float64
}{
	{"context_budget_fraction", 0.65},
}

// intWithDefault mirrors boolWithDefault for an integer-valued global config key.
func intWithDefault(svc *service.GlobalSettingsService, key string, def int) (int, error) {
	val, err := svc.Get(key)
	if err != nil {
		return 0, err
	}
	if val == "" {
		return def, nil
	}
	parsed, parseErr := strconv.Atoi(val)
	if parseErr != nil {
		return def, nil
	}
	return parsed, nil
}

// floatWithDefault mirrors boolWithDefault for a float-valued global config key.
func floatWithDefault(svc *service.GlobalSettingsService, key string, def float64) (float64, error) {
	val, err := svc.Get(key)
	if err != nil {
		return 0, err
	}
	if val == "" {
		return def, nil
	}
	parsed, parseErr := strconv.ParseFloat(val, 64)
	if parseErr != nil {
		return def, nil
	}
	return parsed, nil
}

// watcherPatchFields carries the watcher knobs on the settings PATCH body as
// raw JSON so each can independently be absent (no-op), null (clear), or a
// number (set) — same convention as applyOptionalIntSetting.
type watcherPatchFields struct {
	ContextBudgetFraction               json.RawMessage `json:"context_budget_fraction"`
	ContextBudgetDefault                json.RawMessage `json:"context_budget_default"`
	ContextDecayTurns                   json.RawMessage `json:"context_decay_turns"`
	CacheTTLSec                         json.RawMessage `json:"cache_ttl_sec"`
	MinEpochIntervalCalls               json.RawMessage `json:"min_epoch_interval_calls"`
	ProactiveRestartThresholdDefault    json.RawMessage `json:"proactive_restart_threshold_default"`
	ProactiveRestartMinIntervalSec      json.RawMessage `json:"proactive_restart_min_interval_sec"`
	ProactiveRestartMaxPerSession       json.RawMessage `json:"proactive_restart_max_per_session"`
	ProactiveRestartBoundaryWindowTurns json.RawMessage `json:"proactive_restart_boundary_window_turns"`
	ProactiveRestartConsolePct          json.RawMessage `json:"proactive_restart_console_pct"`
}

// applyWatcherSettings persists each present field via applyOptionalIntSetting
// (int knobs) or applyOptionalFloatSetting (context_budget_fraction).
func applyWatcherSettings(fields watcherPatchFields, svc *service.GlobalSettingsService, w http.ResponseWriter) error {
	if err := applyOptionalFloatSetting(svc, fields.ContextBudgetFraction, "context_budget_fraction", w); err != nil {
		return err
	}
	intFields := []struct {
		raw json.RawMessage
		key string
	}{
		{fields.ContextBudgetDefault, "context_budget_default"},
		{fields.ContextDecayTurns, "context_decay_turns"},
		{fields.CacheTTLSec, "cache_ttl_sec"},
		{fields.MinEpochIntervalCalls, "min_epoch_interval_calls"},
		{fields.ProactiveRestartThresholdDefault, "proactive_restart_threshold_default"},
		{fields.ProactiveRestartMinIntervalSec, "proactive_restart_min_interval_sec"},
		{fields.ProactiveRestartMaxPerSession, "proactive_restart_max_per_session"},
		{fields.ProactiveRestartBoundaryWindowTurns, "proactive_restart_boundary_window_turns"},
		{fields.ProactiveRestartConsolePct, "proactive_restart_console_pct"},
	}
	for _, f := range intFields {
		if err := applyOptionalIntSetting(svc, f.raw, f.key, w); err != nil {
			return err
		}
	}
	return nil
}

// applyOptionalFloatSetting handles a json.RawMessage field that can be
// absent (nil), null ("null" -> clear), or a float (set) — mirrors
// applyOptionalIntSetting for the one float-valued watcher knob.
func applyOptionalFloatSetting(svc *service.GlobalSettingsService, raw json.RawMessage, key string, w http.ResponseWriter) error {
	if raw == nil {
		return nil
	}
	if string(raw) == "null" {
		if err := svc.Set(key, ""); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return err
		}
		return nil
	}
	var v float64
	if err := json.Unmarshal(raw, &v); err != nil {
		writeError(w, http.StatusBadRequest, key+" must be a number or null")
		return err
	}
	if err := svc.Set(key, strconv.FormatFloat(v, 'g', -1, 64)); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return err
	}
	return nil
}
