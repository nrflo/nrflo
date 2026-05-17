package spec_import

import "testing"

func TestCatalog_Length(t *testing.T) {
	if len(Catalog) != 6 {
		t.Errorf("len(Catalog) = %d, want 6", len(Catalog))
	}
}

func TestCatalog_Names(t *testing.T) {
	want := map[string]bool{
		"GITHUB_TOKEN":         true,
		"JIRA_BASE_URL":        true,
		"JIRA_EMAIL":           true,
		"JIRA_API_TOKEN":       true,
		"ANTHROPIC_API_KEY":    true,
		"ANTHROPIC_OAUTH_TOKEN": true,
	}
	seen := map[string]int{}
	for _, e := range Catalog {
		seen[e.Name]++
	}
	for name := range want {
		if seen[name] != 1 {
			t.Errorf("Catalog entry %q: count = %d, want 1", name, seen[name])
		}
	}
}

func TestCatalog_Required(t *testing.T) {
	for _, e := range Catalog {
		switch e.Name {
		case "GITHUB_TOKEN":
			if e.Required {
				t.Errorf("GITHUB_TOKEN.Required = true, want false")
			}
		case "JIRA_BASE_URL", "JIRA_EMAIL", "JIRA_API_TOKEN":
			if !e.Required {
				t.Errorf("%s.Required = false, want true", e.Name)
			}
		}
	}
}

func TestCatalog_Features(t *testing.T) {
	featureOf := map[string]string{}
	for _, e := range Catalog {
		featureOf[e.Name] = e.Feature
	}
	if featureOf["GITHUB_TOKEN"] != string(SourceGitHubIssue) {
		t.Errorf("GITHUB_TOKEN.Feature = %q, want %q", featureOf["GITHUB_TOKEN"], string(SourceGitHubIssue))
	}
	for _, name := range []string{"JIRA_BASE_URL", "JIRA_EMAIL", "JIRA_API_TOKEN"} {
		if featureOf[name] != string(SourceJira) {
			t.Errorf("%s.Feature = %q, want %q", name, featureOf[name], string(SourceJira))
		}
	}
	for _, name := range []string{"ANTHROPIC_API_KEY", "ANTHROPIC_OAUTH_TOKEN"} {
		if featureOf[name] != "anthropic" {
			t.Errorf("%s.Feature = %q, want %q", name, featureOf[name], "anthropic")
		}
	}
}
