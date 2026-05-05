package pythonsdk

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

// TestWriteSDK_CreatesFile verifies that WriteSDK creates nrflo_sdk.py in the target dir.
func TestWriteSDK_CreatesFile(t *testing.T) {
	dir := t.TempDir()
	if err := WriteSDK(dir); err != nil {
		t.Fatalf("WriteSDK: %v", err)
	}
	path := filepath.Join(dir, "nrflo_sdk.py")
	if _, err := os.Stat(path); err != nil {
		t.Errorf("nrflo_sdk.py not created: %v", err)
	}
}

// TestWriteSDK_NonEmpty verifies the written file is non-empty.
func TestWriteSDK_NonEmpty(t *testing.T) {
	dir := t.TempDir()
	if err := WriteSDK(dir); err != nil {
		t.Fatalf("WriteSDK: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "nrflo_sdk.py"))
	if err != nil {
		t.Fatalf("read nrflo_sdk.py: %v", err)
	}
	if len(data) == 0 {
		t.Error("nrflo_sdk.py is empty")
	}
}

// TestWriteSDK_ContainsExpectedMarkers verifies the SDK file contains key symbols.
func TestWriteSDK_ContainsExpectedMarkers(t *testing.T) {
	dir := t.TempDir()
	if err := WriteSDK(dir); err != nil {
		t.Fatalf("WriteSDK: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "nrflo_sdk.py"))
	if err != nil {
		t.Fatalf("read nrflo_sdk.py: %v", err)
	}

	markers := []string{
		"class _Findings",
		"def context",
		"def add",
		"NrfloError",
		"class Client",
		"class _Connection",
	}
	for _, m := range markers {
		if !bytes.Contains(data, []byte(m)) {
			t.Errorf("nrflo_sdk.py missing expected marker: %q", m)
		}
	}
}

// TestWriteSDK_Idempotent verifies that WriteSDK can be called twice without error.
func TestWriteSDK_Idempotent(t *testing.T) {
	dir := t.TempDir()
	if err := WriteSDK(dir); err != nil {
		t.Fatalf("WriteSDK (first call): %v", err)
	}
	if err := WriteSDK(dir); err != nil {
		t.Fatalf("WriteSDK (second call): %v", err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "nrflo_sdk.py"))
	if err != nil {
		t.Fatalf("read nrflo_sdk.py after second write: %v", err)
	}
	if len(data) == 0 {
		t.Error("nrflo_sdk.py empty after idempotent second write")
	}
}

// TestWriteSDK_CreatesParentDir verifies that WriteSDK creates missing parent directories.
func TestWriteSDK_CreatesParentDir(t *testing.T) {
	base := t.TempDir()
	dir := filepath.Join(base, "nested", "sdk")
	if err := WriteSDK(dir); err != nil {
		t.Fatalf("WriteSDK with nested missing dir: %v", err)
	}
	path := filepath.Join(dir, "nrflo_sdk.py")
	if _, err := os.Stat(path); err != nil {
		t.Errorf("nrflo_sdk.py not created in nested dir: %v", err)
	}
}

// TestEmbeddedSDKSource verifies the embedded sdkSource byte slice is non-empty,
// confirming the //go:embed directive wired correctly.
func TestEmbeddedSDKSource(t *testing.T) {
	if len(sdkSource) == 0 {
		t.Error("embedded sdkSource is empty — //go:embed nrflo_sdk.py likely missing or file absent")
	}
}

// TestWriteSDK_FileMatchesEmbed verifies the written file content matches the embedded source.
func TestWriteSDK_FileMatchesEmbed(t *testing.T) {
	dir := t.TempDir()
	if err := WriteSDK(dir); err != nil {
		t.Fatalf("WriteSDK: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "nrflo_sdk.py"))
	if err != nil {
		t.Fatalf("read nrflo_sdk.py: %v", err)
	}
	if !bytes.Equal(data, sdkSource) {
		t.Error("written file content does not match embedded sdkSource")
	}
}
