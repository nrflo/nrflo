package service

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
)

// ErrCommitNotFound is returned when a commit hash does not exist in the repo.
var ErrCommitNotFound = errors.New("commit not found")

// GitService executes git commands against a repository.
type GitService struct{}

// GitCommit represents a single commit in the log.
type GitCommit struct {
	Hash        string `json:"hash"`
	ShortHash   string `json:"short_hash"`
	Author      string `json:"author"`
	AuthorEmail string `json:"author_email"`
	Date        string `json:"date"`
	Message     string `json:"message"`
}

// GitCommitDetail includes the full commit body, changed files, and diff.
type GitCommitDetail struct {
	GitCommit
	Files []GitChangedFile `json:"files"`
	Diff  string           `json:"diff"`
}

// GitChangedFile represents a file changed in a commit.
type GitChangedFile struct {
	Path      string `json:"path"`
	Status    string `json:"status"`
	Additions int    `json:"additions"`
	Deletions int    `json:"deletions"`
}

var hashRegex = regexp.MustCompile(`^[0-9a-fA-F]{4,40}$`)

// sep is a unit separator unlikely to appear in commit messages.
const sep = "\x1f"

func validateBranch(branch string) error {
	if branch == "" {
		return fmt.Errorf("branch name is empty")
	}
	if strings.ContainsAny(branch, " ;|`$&(){}") || strings.HasPrefix(branch, "-") {
		return fmt.Errorf("invalid branch name: %s", branch)
	}
	return nil
}

func validateHash(hash string) error {
	if !hashRegex.MatchString(hash) {
		return fmt.Errorf("invalid commit hash: %s", hash)
	}
	return nil
}

func validateRepoPath(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("repo path does not exist: %s", path)
	}
	if !info.IsDir() {
		return fmt.Errorf("repo path is not a directory: %s", path)
	}
	gitDir := path + "/.git"
	if _, err := os.Stat(gitDir); err != nil {
		return fmt.Errorf("not a git repository: %s", path)
	}
	return nil
}

func runGit(repoPath string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = repoPath
	out, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return "", fmt.Errorf("git %s: %s", args[0], strings.TrimSpace(string(exitErr.Stderr)))
		}
		return "", fmt.Errorf("git %s: %w", args[0], err)
	}
	return string(out), nil
}

// ListCommits returns paginated commits from the given branch.
// Returns commits, total count, and any error.
func (s *GitService) ListCommits(repoPath, branch string, page, perPage int) ([]GitCommit, int, error) {
	if err := validateRepoPath(repoPath); err != nil {
		return nil, 0, err
	}
	if err := validateBranch(branch); err != nil {
		return nil, 0, err
	}

	// Get total count
	countOut, err := runGit(repoPath, "rev-list", "--count", branch)
	if err != nil {
		return nil, 0, fmt.Errorf("counting commits: %w", err)
	}
	total, err := strconv.Atoi(strings.TrimSpace(countOut))
	if err != nil {
		return nil, 0, fmt.Errorf("parsing commit count: %w", err)
	}

	offset := (page - 1) * perPage
	format := strings.Join([]string{"%H", "%h", "%an", "%ae", "%aI", "%s"}, sep)

	logOut, err := runGit(repoPath,
		"log",
		"--format="+format,
		"--max-count="+strconv.Itoa(perPage),
		"--skip="+strconv.Itoa(offset),
		branch,
	)
	if err != nil {
		return nil, 0, fmt.Errorf("listing commits: %w", err)
	}

	var commits []GitCommit
	lines := strings.Split(strings.TrimSpace(logOut), "\n")
	for _, line := range lines {
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, sep, 6)
		if len(parts) != 6 {
			continue
		}
		commits = append(commits, GitCommit{
			Hash:        parts[0],
			ShortHash:   parts[1],
			Author:      parts[2],
			AuthorEmail: parts[3],
			Date:        parts[4],
			Message:     parts[5],
		})
	}

	return commits, total, nil
}

// GetCommitDetail returns full details for a single commit.
func (s *GitService) GetCommitDetail(repoPath, hash string) (*GitCommitDetail, error) {
	if err := validateRepoPath(repoPath); err != nil {
		return nil, err
	}
	if err := validateHash(hash); err != nil {
		return nil, err
	}

	// Get commit metadata (use %x00 to separate body from stats)
	format := strings.Join([]string{"%H", "%h", "%an", "%ae", "%aI", "%B"}, sep)
	showOut, err := runGit(repoPath, "show", "--no-patch", "--format="+format, hash)
	if err != nil {
		if strings.Contains(err.Error(), "unknown revision") || strings.Contains(err.Error(), "bad object") {
			return nil, ErrCommitNotFound
		}
		return nil, fmt.Errorf("getting commit detail: %w", err)
	}

	showOut = strings.TrimSpace(showOut)
	parts := strings.SplitN(showOut, sep, 6)
	if len(parts) != 6 {
		return nil, fmt.Errorf("unexpected git show output")
	}

	detail := &GitCommitDetail{
		GitCommit: GitCommit{
			Hash:        parts[0],
			ShortHash:   parts[1],
			Author:      parts[2],
			AuthorEmail: parts[3],
			Date:        parts[4],
			Message:     strings.TrimSpace(parts[5]),
		},
	}

	// Get per-file stats
	numstatOut, err := runGit(repoPath, "diff-tree", "--no-commit-id", "-r", "--numstat", hash)
	if err != nil {
		return nil, fmt.Errorf("getting file stats: %w", err)
	}

	detail.Files = parseNumstat(numstatOut)

	// Get status info to determine added/modified/deleted/renamed
	statusOut, err := runGit(repoPath, "diff-tree", "--no-commit-id", "-r", "--name-status", hash)
	if err != nil {
		return nil, fmt.Errorf("getting file statuses: %w", err)
	}

	statusMap := parseNameStatus(statusOut)
	for i, f := range detail.Files {
		if status, ok := statusMap[f.Path]; ok {
			detail.Files[i].Status = status
		}
	}

	// Get full diff
	diffOut, err := runGit(repoPath, "diff-tree", "-p", hash)
	if err != nil {
		return nil, fmt.Errorf("getting diff: %w", err)
	}
	detail.Diff = diffOut

	return detail, nil
}

func parseNumstat(output string) []GitChangedFile {
	var files []GitChangedFile
	for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "\t", 3)
		if len(parts) != 3 {
			continue
		}
		adds, _ := strconv.Atoi(parts[0])
		dels, _ := strconv.Atoi(parts[1])
		files = append(files, GitChangedFile{
			Path:      parts[2],
			Status:    "modified", // default, overridden by name-status
			Additions: adds,
			Deletions: dels,
		})
	}
	return files
}

func parseNameStatus(output string) map[string]string {
	result := make(map[string]string)
	for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "\t", 3)
		if len(parts) < 2 {
			continue
		}
		code := parts[0]
		path := parts[len(parts)-1] // for renames, use the new path
		switch {
		case strings.HasPrefix(code, "A"):
			result[path] = "added"
		case strings.HasPrefix(code, "D"):
			result[path] = "deleted"
		case strings.HasPrefix(code, "R"):
			result[path] = "renamed"
		default:
			result[path] = "modified"
		}
	}
	return result
}
