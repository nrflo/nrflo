import { useState } from 'react'
import { useSearchParams } from 'react-router-dom'
import { AlertTriangle, Settings, X } from 'lucide-react'
import { cn } from '@/lib/utils'
import { useProjectStore } from '@/stores/projectStore'
import { GlobalSettingsSection } from '@/components/settings/GlobalSettingsSection'
import { ProjectsSection } from '@/components/settings/ProjectsSection'
import { SystemAgentsSection } from '@/components/settings/SystemAgentsSection'
import { DefaultTemplatesSection } from '@/components/settings/DefaultTemplatesSection'
import { CLIModelsSection } from '@/components/settings/CLIModelsSection'
import { LogsSection } from '@/components/settings/LogsSection'

type SettingsTab = 'general' | 'projects' | 'system-agents' | 'default-templates' | 'cli-models' | 'logs'

const tabs: { id: SettingsTab; label: string }[] = [
  { id: 'general', label: 'General' },
  { id: 'projects', label: 'Projects' },
  { id: 'system-agents', label: 'System Agents' },
  { id: 'default-templates', label: 'Default Templates' },
  { id: 'cli-models', label: 'CLI Models' },
  { id: 'logs', label: 'Logs' },
]

const tabIds = new Set<string>(tabs.map((t) => t.id))

function isValidTab(value: string | null): value is SettingsTab {
  return value !== null && tabIds.has(value)
}

function NoProjectsBanner() {
  const projects = useProjectStore((s) => s.projects)
  const [dismissed, setDismissed] = useState(false)

  if (projects.length > 0 || dismissed) return null

  return (
    <div className="flex items-center gap-2 rounded-lg border border-amber-300 bg-amber-50 px-4 py-3 text-sm text-amber-700 dark:border-amber-700 dark:bg-amber-950/30 dark:text-amber-400">
      <AlertTriangle className="h-4 w-4 shrink-0" />
      <span className="font-medium">No projects configured. Create a project to get started.</span>
      <button onClick={() => setDismissed(true)} className="ml-auto shrink-0 hover:opacity-70" aria-label="Dismiss">
        <X className="h-4 w-4" />
      </button>
    </div>
  )
}

export function SettingsPage() {
  const [searchParams, setSearchParams] = useSearchParams()
  const tabParam = searchParams.get('tab')
  const activeTab: SettingsTab = isValidTab(tabParam) ? tabParam : 'general'

  const handleTabClick = (id: SettingsTab) => {
    setSearchParams({ tab: id }, { replace: true })
  }

  return (
    <div className="max-w-7xl mx-auto space-y-6">
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-3">
          <Settings className="h-6 w-6 text-muted-foreground" />
          <h1 className="text-2xl font-bold tracking-tight">Settings</h1>
        </div>
      </div>

      <div className="rounded-lg border border-border bg-card p-4 space-y-2">
        <h2 className="text-xl font-semibold">New Reality Flow</h2>
        <p className="text-muted-foreground text-sm">
          The main supported CLI is <strong>Claude Code</strong>. The security hook configured in
          Project Settings enforces only Claude CLI tool calls. Claude CLI runs in YOLO mode — if
          that concerns you, add a per-project safety hook to your Claude settings.
        </p>
        <p className="text-muted-foreground text-sm">
          Codex runs in a sandboxed environment (bypasses permissions but sandboxed) — it can do
          research and save findings for later processing by Claude CLI agents. OpenCode support is
          experimental.
        </p>
        <pre className="bg-muted rounded p-3 text-xs font-mono overflow-x-auto whitespace-pre">
{`Claude Code
  claude --print --verbose --dangerously-skip-permissions --output-format stream-json --disallowed-tools "AskUserQuestion,EnterPlanMode,ExitPlanMode" --model <model> --session-id <uuid> [--settings <json>]

OpenCode
  opencode run --format json --model <model> [--variant <effort>] "<prompt>"

Codex
  codex exec --json --model <model> -c 'model_reasoning_effort="<effort>"' --dangerously-bypass-approvals-and-sandbox`}
        </pre>
      </div>

      <NoProjectsBanner />

      <div className="border-b border-border">
        <div className="flex gap-1">
          {tabs.map(({ id, label }) => (
            <button
              key={id}
              onClick={() => handleTabClick(id)}
              className={cn(
                'flex items-center gap-2 px-4 py-2 text-sm font-medium border-b-2 transition-colors',
                activeTab === id
                  ? 'border-primary text-primary'
                  : 'border-transparent text-muted-foreground hover:text-foreground'
              )}
            >
              {label}
            </button>
          ))}
        </div>
      </div>

      {activeTab === 'general' && <GlobalSettingsSection />}
      {activeTab === 'projects' && <ProjectsSection />}
      {activeTab === 'system-agents' && <SystemAgentsSection />}
      {activeTab === 'default-templates' && <DefaultTemplatesSection />}
      {activeTab === 'cli-models' && <CLIModelsSection />}
      {activeTab === 'logs' && <LogsSection initialFilter={searchParams.get('filter') || undefined} />}
    </div>
  )
}
