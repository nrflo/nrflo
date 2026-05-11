import { useState } from 'react'
import { Input } from '@/components/ui/Input'
import { IssueSearchCombo } from './IssueSearchCombo'
import { searchGitHubIssues, type GitHubIssueSummary } from '@/api/specImport'
import { useProjectStore } from '@/stores/projectStore'

interface GitHubSearchComboProps {
  url: string
  onUrlChange: (url: string) => void
}

export function GitHubSearchCombo({ url, onUrlChange }: GitHubSearchComboProps) {
  const [repo, setRepo] = useState('')
  const [notConfigured, setNotConfigured] = useState<
    { missing: string[]; settingsHref: string } | undefined
  >()
  const currentProject = useProjectStore((s) => s.currentProject)

  function handleSelect(issue: GitHubIssueSummary) {
    onUrlChange(issue.html_url)
  }

  function handleNotConfigured(missing: string[]) {
    setNotConfigured({
      missing,
      settingsHref: `/projects/${currentProject}/edit#env-vars`,
    })
  }

  return (
    <div className="space-y-2">
      <div>
        <label className="text-sm text-muted-foreground mb-1 block">
          Repository (optional, e.g. owner/repo)
        </label>
        <Input
          placeholder="owner/repo"
          value={repo}
          onChange={(e) => setRepo(e.target.value)}
          className="w-full"
        />
      </div>

      <div>
        <label className="text-sm text-muted-foreground mb-1 block">
          Search issues or paste URL
        </label>
        <IssueSearchCombo<GitHubIssueSummary>
          placeholder="Search GitHub issues…"
          search={(q) => searchGitHubIssues(q, repo || undefined)}
          renderItem={(issue) => (
            <span>
              #{issue.number} {issue.title}{' '}
              <span className="text-muted-foreground">({issue.state})</span>
            </span>
          )}
          onSelect={handleSelect}
          notConfigured={notConfigured}
          onNotConfigured={handleNotConfigured}
        />
      </div>

      <div>
        <label className="text-sm text-muted-foreground mb-1 block">
          Issue URL
        </label>
        <Input
          placeholder="https://github.com/owner/repo/issues/123"
          value={url}
          onChange={(e) => onUrlChange(e.target.value)}
          className="w-full"
        />
      </div>
    </div>
  )
}
