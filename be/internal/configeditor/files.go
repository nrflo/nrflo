package configeditor

import (
	"os"
	"path/filepath"
	"strings"
)

// FileMeta describes a config file managed by the editor.
type FileMeta struct {
	Path          string // relative path under configDir
	SchemaPath    string // sidecar schema path (empty if none)
	LatestVersion int    // 0 = disk-only, >0 = last DB version
}

// List enumerates managed files under configDir:
//  1. tool_manifest.yaml itself
//  2. ConfigPath entries from each manifest tool
//  3. Sibling *.yaml / *.yml / *.json files discovered on disk
//
// Duplicate paths are deduplicated (first occurrence wins).
// LatestVersion is populated from the repo for each file.
func (s *Service) List(projectID string) ([]FileMeta, error) {
	seen := make(map[string]struct{})
	var metas []FileMeta

	add := func(rel, schemaPath string) {
		if _, dup := seen[rel]; dup {
			return
		}
		seen[rel] = struct{}{}
		ver, _ := s.repo.LatestVersion(projectID, rel)
		metas = append(metas, FileMeta{Path: rel, SchemaPath: schemaPath, LatestVersion: ver})
	}

	// 1. The manifest itself.
	add("tool_manifest.yaml", "")

	// 2. ConfigPath entries from manifest tools.
	for i := range s.manifest.Tools {
		t := &s.manifest.Tools[i]
		for _, cf := range t.ConfigFiles {
			schemaPath := ""
			if cf.SchemaPath != "" {
				schemaPath = cf.SchemaPath
			} else {
				// Auto-detect sidecar: replace extension with .schema.json
				ext := filepath.Ext(cf.Path)
				candidate := strings.TrimSuffix(cf.Path, ext) + ".schema.json"
				if _, err := os.Stat(filepath.Join(s.configDir, candidate)); err == nil {
					schemaPath = candidate
				}
			}
			add(cf.Path, schemaPath)
		}
	}

	// 3. Sibling yaml/yml/json files discovered on disk.
	entries, err := os.ReadDir(s.configDir)
	if err != nil {
		return metas, nil // dir unreadable — return what we have
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		ext := strings.ToLower(filepath.Ext(name))
		if ext != ".yaml" && ext != ".yml" && ext != ".json" {
			continue
		}
		schemaPath := ""
		candidate := strings.TrimSuffix(name, filepath.Ext(name)) + ".schema.json"
		if _, err := os.Stat(filepath.Join(s.configDir, candidate)); err == nil {
			schemaPath = candidate
		}
		add(name, schemaPath)
	}

	return metas, nil
}
