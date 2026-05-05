package pythonsdk

import (
	_ "embed"
	"os"
	"path/filepath"
)

//go:embed nrflo_sdk.py
var sdkSource []byte

// WriteSDK writes the embedded nrflo_sdk.py to dir/nrflo_sdk.py.
// Idempotent overwrite — server upgrades automatically refresh the SDK file.
func WriteSDK(dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "nrflo_sdk.py"), sdkSource, 0o644)
}
