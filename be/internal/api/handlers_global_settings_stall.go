package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"be/internal/service"
)

// applyOptionalIntSetting handles a json.RawMessage field that can be absent (nil),
// null ("null" → clear), or an integer (>= 0 → set). Returns a non-nil error sentinel
// when an HTTP error was already written and the caller should return.
func applyOptionalIntSetting(svc *service.GlobalSettingsService, raw json.RawMessage, key string, w http.ResponseWriter) error {
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
	if v < 0 {
		writeError(w, http.StatusBadRequest, key+" must be >= 0")
		return errors.New("bad request")
	}
	if err := svc.Set(key, strconv.Itoa(v)); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return err
	}
	return nil
}
