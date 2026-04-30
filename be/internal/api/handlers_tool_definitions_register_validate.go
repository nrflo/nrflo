package api

import (
	"encoding/json"
	"fmt"
)

var validAuthMethods = map[string]bool{
	"none":               true,
	"bearer_env":         true,
	"bearer_secret_ref":  true,
}

func validateRegisterEntries(entries []registerToolEntry) error {
	seen := make(map[string]bool, len(entries))
	for i, e := range entries {
		if e.Name == "" {
			return fmt.Errorf("entry %d: name is required", i)
		}
		if e.Endpoint == "" {
			return fmt.Errorf("entry %d (%s): endpoint is required", i, e.Name)
		}
		if len(e.InputSchema) == 0 || string(e.InputSchema) == "null" {
			return fmt.Errorf("entry %d (%s): input_schema is required", i, e.Name)
		}
		if !json.Valid(e.InputSchema) {
			return fmt.Errorf("entry %d (%s): input_schema must be valid JSON", i, e.Name)
		}
		am := e.AuthMethod
		if am == "" {
			am = "none"
		}
		if !validAuthMethods[am] {
			return fmt.Errorf("entry %d (%s): invalid auth_method %q", i, e.Name, am)
		}
		if am != "none" && (e.AuthRef == nil || *e.AuthRef == "") {
			return fmt.Errorf("entry %d (%s): auth_ref is required when auth_method is %q", i, e.Name, am)
		}
		if e.TimeoutSec < 0 {
			return fmt.Errorf("entry %d (%s): timeout_sec must be >= 0", i, e.Name)
		}
		lower := e.Name
		if seen[lower] {
			return fmt.Errorf("duplicate tool name %q in request", e.Name)
		}
		seen[lower] = true
	}
	return nil
}
