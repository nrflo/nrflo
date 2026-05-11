// Package spec_import provides adapters that fetch external specifications
// (GitHub Issues, Jira Issues, raw Markdown) and normalize them into a
// FetchedSpec ready for downstream processing by the spec-normalizer agent.
package spec_import

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"be/internal/model"
)

// sharedClient is used by all adapters. No global timeout — adapters set per-request timeouts via context.
var sharedClient = &http.Client{}

// Source identifies which external source an adapter handles.
type Source string

const (
	SourceGitHubIssue Source = "github_issue"
	SourceJira        Source = "jira"
	SourceMarkdown    Source = "markdown"
)

// Input carries the raw user-supplied spec reference plus an env-var map
// that adapters use instead of os.Getenv so callers can inject per-project vars.
type Input struct {
	// Body is the URL, issue key, or raw Markdown text depending on Source.
	Body string
	// Env provides runtime env vars (e.g. GITHUB_TOKEN, JIRA_BASE_URL).
	Env map[string]string
}

// FetchedSpec is the normalized output of an Adapter.Fetch call.
type FetchedSpec struct {
	// RawText is the full spec text ready for the spec-normalizer agent.
	RawText string
	// AttachedRefs are back-links to be persisted by the repo layer.
	// Only Kind/URL/Label are populated; ID/ProjectID/TicketID/CreatedAt
	// are populated downstream by repo.TicketRefRepo.
	AttachedRefs []model.TicketRef
}

// Adapter is the interface all source adapters must implement.
type Adapter interface {
	// Source returns the source type this adapter handles.
	Source() Source
	// Fetch retrieves and normalizes the spec from the external source.
	Fetch(ctx context.Context, in Input) (FetchedSpec, error)
}

// Sentinel errors returned by adapters.
var (
	// ErrAdapterAuth is returned when the server rejects credentials (HTTP 401).
	ErrAdapterAuth = errors.New("spec_import: authentication failed")
	// ErrAdapterNotFound is returned when the resource does not exist (HTTP 404).
	ErrAdapterNotFound = errors.New("spec_import: resource not found")
)

// MissingEnvError is returned by Fetch/Search when required env vars are absent.
type MissingEnvError struct {
	Source  Source
	Missing []string
}

func (e MissingEnvError) Error() string {
	return fmt.Sprintf("spec_import(%s): missing env vars: %s", e.Source, strings.Join(e.Missing, ", "))
}

// ResolveAdapter returns the Adapter for the given Source.
func ResolveAdapter(s Source) (Adapter, error) {
	switch s {
	case SourceGitHubIssue:
		return &GitHubAdapter{client: sharedClient}, nil
	case SourceJira:
		return &JiraAdapter{client: sharedClient}, nil
	case SourceMarkdown:
		return &MarkdownAdapter{}, nil
	default:
		return nil, fmt.Errorf("spec_import: unknown source %q", s)
	}
}
