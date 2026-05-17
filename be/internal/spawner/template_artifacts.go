package spawner

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"be/internal/logger"
	"be/internal/repo"
)

var artifactsAllPattern = regexp.MustCompile(`#\{ARTIFACTS\}`)
var artifactNamedPattern = regexp.MustCompile(`#\{ARTIFACT:([^}]+)\}`)

// expandArtifacts replaces #{ARTIFACTS} and #{ARTIFACT:name} patterns.
// #{ARTIFACTS} → tab-separated "name\t<absPath>" lines, or placeholder when empty.
// #{ARTIFACT:name} → absolute path of the materialized artifact, or "" + warning when missing.
func (s *Spawner) expandArtifacts(template, projectID, wfiID string) (string, error) {
	if !artifactsAllPattern.MatchString(template) && !artifactNamedPattern.MatchString(template) {
		return template, nil
	}

	ctx := context.Background()

	if wfiID == "" {
		template = artifactsAllPattern.ReplaceAllString(template, "_No artifacts available for this workflow._")
		template = artifactNamedPattern.ReplaceAllString(template, "")
		return template, nil
	}

	pool := s.pool()
	if pool == nil {
		return template, fmt.Errorf("no database pool for artifact expansion")
	}

	artifactRepo := repo.NewArtifactRepo(pool, s.config.Clock)
	artifacts, err := artifactRepo.List(wfiID)
	if err != nil {
		template = artifactsAllPattern.ReplaceAllString(template, "_No artifacts available for this workflow._")
		template = artifactNamedPattern.ReplaceAllString(template, "")
		return template, err
	}

	nameToPath := make(map[string]string, len(artifacts))

	if len(artifacts) > 0 && s.config.ArtifactSvc != nil {
		storage, storageErr := s.config.ArtifactSvc.GetStorage(ctx, projectID)
		if storageErr == nil {
			stageDir, stageErr := EnsureStageDir(projectID, wfiID)
			if stageErr == nil {
				for _, a := range artifacts {
					path, matErr := Materialize(ctx, a, stageDir, storage)
					if matErr == nil {
						nameToPath[a.Name] = path
					} else {
						logger.Warn(ctx, "artifact materialize failed in template", "name", a.Name, "error", matErr)
					}
				}
			}
		}
	}

	template = artifactsAllPattern.ReplaceAllStringFunc(template, func(_ string) string {
		if len(artifacts) == 0 {
			return "_No artifacts available for this workflow._"
		}
		lines := make([]string, 0, len(artifacts))
		for _, a := range artifacts {
			path := nameToPath[a.Name]
			if path == "" {
				path = fmt.Sprintf("[not materialized: %s]", a.Name)
			}
			lines = append(lines, a.Name+"\t"+path)
		}
		return strings.Join(lines, "\n")
	})

	template = artifactNamedPattern.ReplaceAllStringFunc(template, func(match string) string {
		sub := artifactNamedPattern.FindStringSubmatch(match)
		if len(sub) < 2 {
			return ""
		}
		name := sub[1]
		path, ok := nameToPath[name]
		if !ok {
			logger.Warn(ctx, "artifact not found in template", "name", name)
			return ""
		}
		return path
	})

	return template, nil
}
