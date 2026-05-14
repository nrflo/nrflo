import { useState } from 'react'
import { IssueSearchCombo } from './IssueSearchCombo'
import { searchJiraIssues, type JiraIssueSummary } from '@/api/specImport'
import { useProjectStore } from '@/stores/projectStore'

// Matches bare Jira keys like "PROJ-123" or full URLs
const JIRA_KEY_RE = /^[A-Z][A-Z0-9_]+-[0-9]+$/
const URL_RE = /^https?:\/\//

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

  // Skip search when input looks like a bare key or full URL
  function shouldBypassSearch(q: string): boolean {
    return JIRA_KEY_RE.test(q.trim()) || URL_RE.test(q.trim())
  }

  async function doSearch(q: string): Promise<JiraIssueSummary[]> {
    if (shouldBypassSearch(q)) return []
    return searchJiraIssues(q)
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
      notConfigured={notConfigured}
      onNotConfigured={handleNotConfigured}
    />
  )
}
