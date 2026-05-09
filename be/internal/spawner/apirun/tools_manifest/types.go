// Package tools_manifest provides the manifest-backed tool provider for api-mode agents.
// It implements the apirun.ManifestProvider interface so the apirun registry
// can compose manifest tools alongside builtins and HTTP defs without a direct import.
package tools_manifest

import (
	"be/internal/clock"
	"be/internal/manifest/config"
	"be/internal/manifest/python"
	"be/internal/repo"
	"be/internal/service"
)

// deps groups all external dependencies for the manifest provider so tests can
// substitute fakes without changing the public New() signature.
type deps struct {
	manifest     *config.Manifest
	runner       python.Runner
	projectID    string
	sessionID    string
	dispatchRepo *repo.DispatchRepo
	reviewRepo   *repo.ReviewRepo
	hub          service.WSHub
	clock        clock.Clock
	projectEnv   []string
}
