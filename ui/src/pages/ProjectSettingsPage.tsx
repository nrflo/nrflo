import { useState, useEffect } from 'react'
import { useProjectStore } from '@/stores/projectStore'
import { Card, CardHeader, CardTitle, CardDescription, CardContent } from '@/components/ui/Card'
import { ProjectForm } from '@/components/settings/ProjectForm'
import { parseSafetyHookConfig, emptyProjectForm, type ProjectFormData } from '@/components/settings/projectFormUtils'
import { useSaveProjectSettings } from '@/hooks/useSaveProjectSettings'
import type { Project } from '@/api/projects'

function projectToFormData(project: Project): ProjectFormData {
  return {
    id: project.id,
    name: project.name,
    root_path: project.root_path || '',
    default_branch: project.default_branch || '',
    use_git_worktrees: project.use_git_worktrees || false,
    push_after_merge: project.push_after_merge || false,
    ...parseSafetyHookConfig(project.claude_safety_hook),
  }
}

export function ProjectSettingsPage() {
  const { currentProject, projects, projectsLoaded } = useProjectStore()
  const project = projects.find((p) => p.id === currentProject)
  const [formData, setFormData] = useState<ProjectFormData>(() =>
    project ? projectToFormData(project) : emptyProjectForm
  )

  useEffect(() => {
    if (project) setFormData(projectToFormData(project))
  }, [project?.id]) // eslint-disable-line react-hooks/exhaustive-deps

  const { save, isPending, isError, error, artifactError, cleanupError, observerError } =
    useSaveProjectSettings(currentProject ?? '')

  const resetToLoaded = () => {
    if (project) setFormData(projectToFormData(project))
  }

  if (!projectsLoaded) {
    return (
      <div className="flex items-center justify-center py-12">
        <div className="text-muted-foreground">Loading project settings...</div>
      </div>
    )
  }

  if (!currentProject || !project) {
    return (
      <div className="flex items-center justify-center py-12">
        <div className="text-muted-foreground">No active project selected.</div>
      </div>
    )
  }

  return (
    <div className="p-6 max-w-[84rem] mx-auto">
      <Card>
        <CardHeader>
          <CardTitle>Project Settings</CardTitle>
          <CardDescription>{project.name}</CardDescription>
        </CardHeader>
        <CardContent>
          <ProjectForm
            formData={formData}
            setFormData={setFormData}
            disabledId={currentProject}
            onSave={(subforms) => save(formData, subforms)}
            onCancel={resetToLoaded}
            mutation={{ isPending, isError, error, artifactError, cleanupError, observerError }}
          />
        </CardContent>
      </Card>
    </div>
  )
}
