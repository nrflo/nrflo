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

// resolvePaths checks each candidate against the working tree rooted at
// root: an absolute or slash-containing candidate must Stat inside root
// after containment-checking; a bare basename is canonicalized ONLY when
// it matches exactly one file under root via the lazily built basename
// index — 0 or >1 matches stay unverified. Never guesses, never fuzzy
// matches, never picks the first of several candidates. An empty root
// marks every candidate unverified.
func resolvePaths(root string, cands []string) (verified []string, unverified []string) {
	if root == "" || len(cands) == 0 {
		return nil, cands
	}
	evalRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return nil, cands
	}

	var index map[string][]string
	for _, cand := range cands {
		if resolved, ok := resolveOnePath(evalRoot, cand, &index); ok {
			verified = append(verified, resolved)
		} else {
			unverified = append(unverified, cand)
		}
	}
	return verified, unverified
}

func resolveOnePath(root, cand string, index *map[string][]string) (string, bool) {
	if cand == "" {
		return "", false
	}

	if filepath.IsAbs(cand) {
		clean := filepath.Clean(cand)
		if !withinRoot(root, clean) || !statOK(clean) {
			return "", false
		}
		if rel, err := filepath.Rel(root, clean); err == nil {
			return rel, true
		}
		return "", false
	}

	if strings.ContainsAny(cand, "/\\") {
		joined := filepath.Clean(filepath.Join(root, cand))
		if !withinRoot(root, joined) || !statOK(joined) {
			return "", false
		}
		return filepath.Clean(cand), true
	}

	// Bare basename: consult the lazily built index, canonicalize only on a
	// unique match — the epic's regression case (LogMessage.ts mentioned
	// while only LogMessage.tsx exists) must land here as unverified.
	if *index == nil {
		*index = buildBasenameIndex(root)
	}
	matches := (*index)[cand]
	if len(matches) == 1 {
		return matches[0], true
	}
	return "", false
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
