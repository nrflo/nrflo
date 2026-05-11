package spec_import

// EnvVarSpec describes a well-known env var used by spec_import adapters.
type EnvVarSpec struct {
	Name        string
	Feature     string
	Description string
	Required    bool
}

// Catalog is the authoritative list of env vars consumed by this package.
// Callers (e.g. API handlers) can surface this list to guide users.
var Catalog = []EnvVarSpec{
	{
		Name:        "GITHUB_TOKEN",
		Feature:     string(SourceGitHubIssue),
		Description: "Personal access token or fine-grained token for GitHub API. Public repos work without it; required for private repos and Search.",
		Required:    false,
	},
	{
		Name:        "JIRA_BASE_URL",
		Feature:     string(SourceJira),
		Description: "Base URL of the Jira instance, e.g. https://yourorg.atlassian.net",
		Required:    true,
	},
	{
		Name:        "JIRA_EMAIL",
		Feature:     string(SourceJira),
		Description: "Email address associated with the Jira API token.",
		Required:    true,
	},
	{
		Name:        "JIRA_API_TOKEN",
		Feature:     string(SourceJira),
		Description: "Jira API token generated at https://id.atlassian.com/manage-profile/security/api-tokens",
		Required:    true,
	},
}
