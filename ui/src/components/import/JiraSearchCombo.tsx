import { useState } from 'react'
import { IssueSearchCombo } from './IssueSearchCombo'
import { searchJiraIssues, type JiraIssueSummary } from '@/api/specImport'
import { useProjectStore } from '@/stores/projectStore'

// /browse/PROJ-123 inside a Jira URL.
const BROWSE_KEY_RE = /\/browse\/([A-Z][A-Z0-9_]+-[0-9]+)/

interface JiraSearchComboProps {
  value: string
  onChange: (value: string) => void
}

export function JiraSearchCombo({ value: _value, onChange }: JiraSearchComboProps) {
  const [notConfigured, setNotConfigured] = useState<
    { missing: string[]; settingsHref: string } | undefined
  >()
  const currentProject = useProjectStore((s) => s.currentProject)

  function handleSelect(issue: JiraIssueSummary) {
    onChange(issue.url ?? issue.key)
  }

  function handleNotConfigured(missing: string[]) {
    setNotConfigured({
      missing,
      settingsHref: `/settings?tab=projects&project=${encodeURIComponent(currentProject ?? '')}#env-vars`,
    })
  }

  async function doSearch(q: string): Promise<JiraIssueSummary[]> {
    // Pasted browse URL → extract the key so the backend `key = …` branch hits.
    const m = q.match(BROWSE_KEY_RE)
    return searchJiraIssues(m ? m[1] : q)
  }

  return (
    <IssueSearchCombo<JiraIssueSummary>
      placeholder="Search Jira issues or paste key/URL…"
      search={doSearch}
      renderItem={(issue) => (
        <span>
          <span className="font-mono text-xs">{issue.key}</span> — {issue.summary}{' '}
          <span className="text-muted-foreground">({issue.status})</span>
        </span>
      )}
      onSelect={handleSelect}
      formatSelection={(issue) => issue.key}
      notConfigured={notConfigured}
      onNotConfigured={handleNotConfigured}
    />
  )
}
