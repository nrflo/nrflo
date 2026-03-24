import { Settings } from 'lucide-react'
import { GlobalSettingsSection } from '@/components/settings/GlobalSettingsSection'
import { ProjectsSection } from '@/components/settings/ProjectsSection'
import { SystemAgentsSection } from '@/components/settings/SystemAgentsSection'

export function SettingsPage() {
  return (
    <div className="max-w-7xl mx-auto space-y-6">
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-3">
          <Settings className="h-6 w-6 text-muted-foreground" />
          <h1 className="text-2xl font-bold tracking-tight">Settings</h1>
        </div>
      </div>

      <GlobalSettingsSection />
      <ProjectsSection />
      <SystemAgentsSection />
    </div>
  )
}
