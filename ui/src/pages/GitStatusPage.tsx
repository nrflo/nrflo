import { useMemo } from 'react'
import { useProjectStore } from '@/stores/projectStore'
import { GitStatusTabContent } from './GitStatusTabContent'

export function GitStatusPage() {
  const currentProject = useProjectStore((s) => s.currentProject)
  const projects = useProjectStore((s) => s.projects)

  const hasDefaultBranch = useMemo(() => {
    const project = projects.find((p) => p.id === currentProject)
    return !!project?.default_branch
  }, [projects, currentProject])

  if (!currentProject) {
    return (
      <div className="max-w-7xl mx-auto p-6">
        <h1 className="text-2xl font-bold mb-4">Git Status</h1>
        <div className="text-center py-12">
          <p className="text-muted-foreground">No project selected</p>
        </div>
      </div>
    )
  }

  if (!hasDefaultBranch) {
    return (
      <div className="max-w-7xl mx-auto p-6">
        <h1 className="text-2xl font-bold mb-4">Git Status</h1>
        <div className="text-center py-12">
          <p className="text-muted-foreground">
            No default branch configured for this project
          </p>
        </div>
      </div>
    )
  }

  return (
    <div className="max-w-7xl mx-auto p-6 space-y-6">
      <h1 className="text-2xl font-bold">Git Status</h1>
      <GitStatusTabContent projectId={currentProject} />
    </div>
  )
}
