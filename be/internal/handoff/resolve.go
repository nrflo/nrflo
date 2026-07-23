package handoff

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/repo"
)

// maxIndexedFiles bounds the lazily built basename index (buildBasenameIndex)
// so a bare-basename candidate over a huge repo cannot make Compose slow.
const maxIndexedFiles = 40000

// skipDirs mirrors the fs_glob.go/fs_grep.go builtin-tool walk skip-list.
var skipDirs = map[string]bool{
	".git": true, "node_modules": true, "dist": true, "build": true,
	"vendor": true, ".venv": true, "__pycache__": true, "target": true, ".next": true,
}

// PathStatus classifies a single ResolvePathCandidates outcome.
type PathStatus int

// PathStatus values. A bare basename is the only candidate shape that can
// land in PathAmbiguous — absolute and slash-containing candidates are a
// direct Stat, so they can only ever be PathResolved or PathUnresolved.
const (
	PathUnresolved PathStatus = iota
	PathResolved
	PathAmbiguous
)

// PathResult is one candidate's resolution outcome: Candidate is the
// original input verbatim, Resolved is the root-relative canonical path
// (set only when Status==PathResolved).
type PathResult struct {
	Candidate string
	Resolved  string
	Status    PathStatus
}

// ResolvePathCandidates is the per-candidate form of the never-synthesize
// resolver: an absolute or slash-containing candidate must Stat inside root
// after containment-checking (PathResolved or PathUnresolved only); a bare
// basename is canonicalized ONLY when it matches exactly one file under root
// via the lazily built basename index (PathResolved), 0 matches stays
// PathUnresolved, >1 matches is PathAmbiguous. Never guesses, never fuzzy
// matches, never picks the first of several candidates. An empty root
// marks every candidate PathUnresolved.
func ResolvePathCandidates(root string, cands []string) []PathResult {
	if len(cands) == 0 {
		return nil
	}
	if root == "" {
		return unresolvedResults(cands)
	}
	evalRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return unresolvedResults(cands)
	}

	var index map[string][]string
	results := make([]PathResult, len(cands))
	for i, cand := range cands {
		resolved, status := resolveOnePath(evalRoot, cand, &index)
		results[i] = PathResult{Candidate: cand, Resolved: resolved, Status: status}
	}
	return results
}

func unresolvedResults(cands []string) []PathResult {
	results := make([]PathResult, len(cands))
	for i, cand := range cands {
		results[i] = PathResult{Candidate: cand, Status: PathUnresolved}
	}
	return results
}

// resolvePaths checks each candidate against the working tree rooted at
// root, collapsing PathAmbiguous into unverified alongside PathUnresolved —
// a thin wrapper over ResolvePathCandidates kept for the existing verified
// vs unverified callers.
func resolvePaths(root string, cands []string) (verified []string, unverified []string) {
	if root == "" || len(cands) == 0 {
		return nil, cands
	}
	for _, r := range ResolvePathCandidates(root, cands) {
		if r.Status == PathResolved {
			verified = append(verified, r.Resolved)
		} else {
			unverified = append(unverified, r.Candidate)
		}
	}
	return verified, unverified
}

func resolveOnePath(root, cand string, index *map[string][]string) (string, PathStatus) {
	if cand == "" {
		return "", PathUnresolved
	}

	if filepath.IsAbs(cand) {
		clean := filepath.Clean(cand)
		if !withinRoot(root, clean) || !statOK(clean) {
			return "", PathUnresolved
		}
		if rel, err := filepath.Rel(root, clean); err == nil {
			return rel, PathResolved
		}
		return "", PathUnresolved
	}

	if strings.ContainsAny(cand, "/\\") {
		joined := filepath.Clean(filepath.Join(root, cand))
		if !withinRoot(root, joined) || !statOK(joined) {
			return "", PathUnresolved
		}
		return filepath.Clean(cand), PathResolved
	}

	// Bare basename: consult the lazily built index, canonicalize only on a
	// unique match — the epic's regression case (LogMessage.ts mentioned
	// while only LogMessage.tsx exists) must land here as unverified.
	if *index == nil {
		*index = buildBasenameIndex(root)
	}
	matches := (*index)[cand]
	switch len(matches) {
	case 1:
		return matches[0], PathResolved
	case 0:
		return "", PathUnresolved
	default:
		return "", PathAmbiguous
	}
}

func withinRoot(root, p string) bool {
	rel, err := filepath.Rel(root, p)
	if err != nil {
		return false
	}
	if rel == "." {
		return true
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func statOK(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

// buildBasenameIndex walks root once, mapping each file's basename to its
// root-relative path(s), skipping common dependency/build directories and
// capping at maxIndexedFiles. Walk errors are swallowed — a partial index
// degrades to more candidates staying unverified, never a Compose failure.
func buildBasenameIndex(root string) map[string][]string {
	index := map[string][]string{}
	count := 0
	_ = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil //nolint:nilerr
		}
		if d.IsDir() {
			if p != root && skipDirs[d.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		if count >= maxIndexedFiles {
			return filepath.SkipAll
		}
		count++
		rel, relErr := filepath.Rel(root, p)
		if relErr != nil {
			return nil
		}
		base := filepath.Base(rel)
		index[base] = append(index[base], rel)
		return nil
	})
	return index
}

// resolveTickets verifies ticket-ID candidates against repo.TicketRepo,
// capped at maxTicketIDs lookups (already enforced by extraction, guarded
// again here). A hit renders as "id — title (status)"; anything not found
// stays unverified, verbatim as extracted.
func resolveTickets(pool *db.Pool, clk clock.Clock, projectID string, ids []string) (verified []string, unverified []string) {
	if projectID == "" || len(ids) == 0 {
		return nil, ids
	}
	ticketRepo := repo.NewTicketRepo(pool, clk)
	for i, id := range ids {
		if i >= maxTicketIDs {
			unverified = append(unverified, ids[i:]...)
			break
		}
		t, err := ticketRepo.Get(projectID, id)
		if err != nil || t == nil {
			unverified = append(unverified, id)
			continue
		}
		verified = append(verified, id+" — "+t.Title+" ("+string(t.Status)+")")
	}
	return verified, unverified
}
