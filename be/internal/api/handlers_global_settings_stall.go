package api

import (
	"encoding/json"
	"errors"
	"math"
	"net/http"
	"strconv"

	"be/internal/service"
)

// applyOptionalIntSetting handles a json.RawMessage field that can be absent (nil),
// null ("null" → clear), or an integer (>= 0 → set). Returns a non-nil error sentinel
// when an HTTP error was already written and the caller should return.
func applyOptionalIntSetting(svc *service.GlobalSettingsService, raw json.RawMessage, key string, w http.ResponseWriter) error {
	return applyOptionalBoundedIntSetting(svc, raw, key, 0, math.MaxInt, w)
}

// applyOptionalBoundedIntSetting is applyOptionalIntSetting with an inclusive
// [min, max] range check on the parsed integer.
func applyOptionalBoundedIntSetting(svc *service.GlobalSettingsService, raw json.RawMessage, key string, min, max int, w http.ResponseWriter) error {
	if raw == nil {
		return nil // absent → no-op
	}
	if string(raw) == "null" {
		if err := svc.Set(key, ""); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return err
		}
		return nil
	}
	var v int
	if err := json.Unmarshal(raw, &v); err != nil {
		writeError(w, http.StatusBadRequest, key+" must be an integer or null")
		return err
	}
	if v < min {
		writeError(w, http.StatusBadRequest, key+" must be >= "+strconv.Itoa(min))
		return errors.New("bad request")
	}
	if v > max {
		writeError(w, http.StatusBadRequest, key+" must be <= "+strconv.Itoa(max))
		return errors.New("bad request")
	}
	if err := svc.Set(key, strconv.Itoa(v)); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return err
	}
	return nil
}
