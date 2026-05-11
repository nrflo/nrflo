# spec_import package

Leaf package providing context-aware adapters that fetch external specifications and normalize them into `FetchedSpec{RawText, AttachedRefs}` for the spec-normalizer agent.

## Adapter contract

```go
type Adapter interface {
    Source() Source
    Fetch(ctx context.Context, in Input) (FetchedSpec, error)
}
```

`ResolveAdapter(s Source) (Adapter, error)` is the factory. `Input.Env` is an explicit `map[string]string` (not `os.Getenv`) so callers can inject per-project env vars.

## Sources

| Source constant       | Value            | Description                              |
|-----------------------|------------------|------------------------------------------|
| `SourceGitHubIssue`   | `github_issue`   | GitHub issue URL                         |
| `SourceJira`          | `jira`           | Jira issue key or browse URL             |
| `SourceMarkdown`      | `markdown`       | Raw Markdown passthrough (verbatim body) |

## Per-adapter behavior

### GitHubAdapter
- Parses `https://github.com/{owner}/{repo}/issues/{n}` — rejects `/pull/` and malformed paths.
- `Fetch`: GET `/repos/{o}/{r}/issues/{n}` + `/comments?per_page=20`; appends one `KindSource` ref with `Label=owner/repo#n`.
- `Search`: requires `GITHUB_TOKEN`; returns `MissingEnvError` when absent.
- Auth: `Authorization: Bearer ${GITHUB_TOKEN}` when present; public repos work without it in `Fetch`.
- 401 → `ErrAdapterAuth`, 404 → `ErrAdapterNotFound`.

### JiraAdapter
- Parses raw issue key (`PROJ-123`, regex `^[A-Z][A-Z0-9_]+-[0-9]+$`) or `{base}/browse/{KEY}` URL.
- `Fetch` and `Search` both call `checkEnv` first; any of `JIRA_BASE_URL / JIRA_EMAIL / JIRA_API_TOKEN` missing → `MissingEnvError` with no network call.
- Auth: `Authorization: Basic base64(email:token)`.
- `Fetch`: GET `/rest/api/3/issue/{key}`; ADF description → plain text via `adfToText`; appends `KindSource` ref.
- `Search`: POST `/rest/api/3/search/jql`; user query escaped and capped at 64 bytes; returns `[]JiraIssueSummary`.
- 401 → `ErrAdapterAuth`, 404 → `ErrAdapterNotFound`.

### MarkdownAdapter
- `Fetch` returns `FetchedSpec{RawText: in.Body}` verbatim; no network calls.

## Env-var catalog

Defined in `catalog.go` as `Catalog []EnvVarSpec`. Single source of truth for API surfaces.

| Name             | Feature        | Required |
|------------------|----------------|----------|
| `GITHUB_TOKEN`   | `github_issue` | false    |
| `JIRA_BASE_URL`  | `jira`         | true     |
| `JIRA_EMAIL`     | `jira`         | true     |
| `JIRA_API_TOKEN` | `jira`         | true     |

## Error semantics

| Error                | Type                    | Meaning                            |
|----------------------|-------------------------|------------------------------------|
| `ErrAdapterAuth`     | sentinel (`errors.New`) | HTTP 401 from upstream             |
| `ErrAdapterNotFound` | sentinel (`errors.New`) | HTTP 404 from upstream             |
| `MissingEnvError`    | struct                  | Required env vars absent; no I/O   |

Callers use `errors.Is` for sentinels and `errors.As` for `MissingEnvError`.

## ADF walker (adapter_jira_adf.go)

Recursive `adfNode` walker covering: `doc`, `paragraph`, `heading` (H# prefix), `bulletList` (`-`), `orderedList` (`N.`), `listItem`, `codeBlock` (``` fence), `text` with `code` mark (`backticks`), `inlineCard`, `hardBreak`. Unknown node types recurse into `content` to avoid text loss. Stdlib-only.

## HTTP client + timeout convention

- `sharedClient = &http.Client{}` (no global timeout).
- Each Fetch/Search creates a `context.WithTimeout(ctx, 15*time.Second)` and uses `http.NewRequestWithContext`.
- Response bodies capped at 1 MiB via `io.LimitReader`.

## File size discipline

Each `.go` file stays under 300 LOC per root CLAUDE.md rule. Jira HTTP logic and ADF walker are split across `adapter_jira.go` and `adapter_jira_adf.go` from day one.
