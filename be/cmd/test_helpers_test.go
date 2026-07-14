package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// getBeDir returns the absolute path to the be/ directory.
func getBeDir(t *testing.T) string {
	t.Helper()
	beDir, err := findBeDir()
	if err != nil {
		t.Fatal(err)
	}
	return beDir
}

func findBeDir() (string, error) {
	wd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("get working directory: %w", err)
	}
	if strings.HasSuffix(wd, "/be/cmd") || strings.HasSuffix(wd, "/be/cmd/server") {
		return filepath.Join(wd, ".."), nil
	}
	if strings.HasSuffix(wd, "/be") {
		return wd, nil
	}
	beDir := filepath.Join(wd, "be")
	if _, err := os.Stat(beDir); os.IsNotExist(err) {
		return "", fmt.Errorf("cannot find be/ directory from %s", wd)
	}
	return beDir, nil
}
