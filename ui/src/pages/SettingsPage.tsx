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
