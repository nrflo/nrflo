package tools_web

import (
	"os"
	"strconv"

	"be/internal/db"
)

// Resolver resolves the active providers, their secrets, and tunables for a
// project using nrflo's standard precedence ladder:
//
//	project env var / project config  ->  global config  ->  built-in default
//
// Secrets fall back to the server process env (os.Getenv), mirroring
// orchestrator/api_provider.go credential resolution.
type Resolver struct {
	pool      *db.Pool
	projectID string
}

// NewResolver builds a Resolver bound to a project. pool must be non-nil.
func NewResolver(pool *db.Pool, projectID string) *Resolver {
	return &Resolver{pool: pool, projectID: projectID}
}

// secret returns a credential/base-URL value: project_env_vars row first, then
// the server process env, then "".
func (r *Resolver) secret(name string) string {
	if r.pool != nil && r.projectID != "" {
		var v string
		// Any read miss/error falls through to the server process env.
		if err := r.pool.QueryRow(
			"SELECT value FROM project_env_vars WHERE project_id = ? AND name = ?",
			r.projectID, name,
		).Scan(&v); err == nil && v != "" {
			return v
		}
	}
	return os.Getenv(name)
}

// setting returns a config value: project config -> global config -> def.
func (r *Resolver) setting(key, def string) string {
	if r.pool != nil {
		if r.projectID != "" {
			if v, _ := r.pool.GetProjectConfig(r.projectID, key); v != "" {
				return v
			}
		}
		if v, _ := r.pool.GetConfig(key); v != "" {
			return v
		}
	}
	return def
}

// settingInt parses setting(key) as an int, falling back to def on error.
func (r *Resolver) settingInt(key string, def int) int {
	if v := r.setting(key, ""); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

// SearchProvider returns the configured search provider (default "exa").
func (r *Resolver) SearchProvider() (SearchProvider, error) {
	return resolveSearch(r.setting("web_search_provider", defaultSearchProvider), r)
}

// FetchProvider returns the configured fetch provider (default "jina").
func (r *Resolver) FetchProvider() (FetchProvider, error) {
	return resolveFetch(r.setting("web_fetch_provider", defaultFetchProvider), r)
}

// Tunables (project/global config, with hardcoded fallbacks).
func (r *Resolver) ExcerptBytes() int {
	return r.settingInt("web_fetch_excerpt_bytes", defaultExcerptBytes)
}
func (r *Resolver) MaxPerDomain() int {
	return r.settingInt("web_search_max_per_domain", defaultMaxPerDomain)
}
func (r *Resolver) MaxResultsPerQuery() int {
	return r.settingInt("web_search_max_results_per_query", defaultMaxResultsPerQuery)
}

const (
	defaultSearchProvider     = "exa"
	defaultFetchProvider      = "jina"
	defaultExcerptBytes       = 6000
	defaultMaxPerDomain       = 3
	defaultMaxResultsPerQuery = 6
)
