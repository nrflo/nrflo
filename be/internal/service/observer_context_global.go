package service

import (
	"fmt"
	"strings"
)

// buildGlobalDynamicContext renders project list + per-project counts + recent sessions
// + global health snapshot into a markdown bundle for a global-scope observer.
func buildGlobalDynamicContext(s *ObserverService) (string, error) {
	var b strings.Builder

	b.WriteString("## Global Context\n\n")

	projects, err := s.projectSvc.List()
	if err != nil {
		b.WriteString("_Could not load projects._\n\n")
		return b.String(), nil
	}

	b.WriteString(fmt.Sprintf("**Projects:** %d\n\n", len(projects)))

	if len(projects) > 0 {
		b.WriteString("### Projects\n\n")
		for _, p := range projects {
			b.WriteString(fmt.Sprintf("- **%s** (%s)\n", p.ID, p.Name))

			// Per-project recent sessions count
			sessions, sessErr := s.agentSvc.GetRecentSessions(p.ID, 5)
			if sessErr == nil {
				b.WriteString(fmt.Sprintf("  Recent sessions: %d\n", len(sessions)))
			}
		}
		b.WriteString("\n")
	}

	// Global recent sessions across all projects
	b.WriteString("### Recent Sessions (all projects)\n\n")
	for _, p := range projects {
		sessions, err := s.agentSvc.GetRecentSessions(p.ID, 3)
		if err != nil || len(sessions) == 0 {
			continue
		}
		for _, sess := range sessions {
			b.WriteString(fmt.Sprintf("- [%s] %s phase=%s status=%s\n", p.ID, sess.ID[:8], sess.Phase, sess.Status))
		}
	}
	b.WriteString("\n")

	return b.String(), nil
}
