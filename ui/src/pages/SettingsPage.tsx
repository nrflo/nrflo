import { useState } from 'react'
import { Settings } from 'lucide-react'
import { cn } from '@/lib/utils'
import { GlobalSettingsSection } from '@/components/settings/GlobalSettingsSection'
import { ProjectsSection } from '@/components/settings/ProjectsSection'
import { SystemAgentsSection } from '@/components/settings/SystemAgentsSection'
import { DefaultTemplatesSection } from '@/components/settings/DefaultTemplatesSection'

type SettingsTab = 'general' | 'projects' | 'system-agents' | 'default-templates'

const tabs: { id: SettingsTab; label: string }[] = [
  { id: 'general', label: 'General' },
  { id: 'projects', label: 'Projects' },
  { id: 'system-agents', label: 'System Agents' },
  { id: 'default-templates', label: 'Default Templates' },
]

export function SettingsPage() {
  const [activeTab, setActiveTab] = useState<SettingsTab>('general')

  return (
    <div className="max-w-7xl mx-auto space-y-6">
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-3">
          <Settings className="h-6 w-6 text-muted-foreground" />
          <h1 className="text-2xl font-bold tracking-tight">Settings</h1>
        </div>
      </div>

      <div className="border-b border-border">
        <div className="flex gap-1">
          {tabs.map(({ id, label }) => (
            <button
              key={id}
              onClick={() => setActiveTab(id)}
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
    </div>
  )
}
