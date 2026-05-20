import { useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { Plus } from 'lucide-react'
import { Button } from '@/components/ui/Button'
import { Card, CardContent, CardHeader, CardTitle, CardDescription } from '@/components/ui/Card'
import { useProjectStore } from '@/stores/projectStore'
import {
  listProjects,
  createProject,
  deleteProject,
  type Project,
  type CreateProjectRequest,
} from '@/api/projects'
import { ProjectForm } from './ProjectForm'
import { ProjectListItem } from './ProjectListItem'
import {
  emptyProjectForm,
  buildSafetyHookJSON,
  type ProjectFormData,
} from './projectFormUtils'

const projectKeys = {
  all: ['projects'] as const,
  list: () => [...projectKeys.all, 'list'] as const,
}

export function ProjectsSection() {
  const queryClient = useQueryClient()
  const { currentProject, setCurrentProject, loadProjects } = useProjectStore()
  const navigate = useNavigate()

  const [isCreating, setIsCreating] = useState(false)
  const [formData, setFormData] = useState<ProjectFormData>(emptyProjectForm)
  const [deleteConfirm, setDeleteConfirm] = useState<string | null>(null)

  const { data, isLoading, error } = useQuery({
    queryKey: projectKeys.list(),
    queryFn: listProjects,
  })

  const projects = data?.projects || []

  const createMutation = useMutation({
    mutationFn: (data: CreateProjectRequest) => createProject(data),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: projectKeys.list() })
      loadProjects()
      setIsCreating(false)
      setFormData(emptyProjectForm)
    },
  })

  const deleteMutation = useMutation({
    mutationFn: (id: string) => deleteProject(id),
    onSuccess: (_, deletedId) => {
      queryClient.invalidateQueries({ queryKey: projectKeys.list() })
      loadProjects()
      setDeleteConfirm(null)
      if (deletedId === currentProject && projects.length > 1) {
        const remaining = projects.filter((p) => p.id !== deletedId)
        if (remaining.length > 0) {
          setCurrentProject(remaining[0].id)
        }
      }
    },
  })

  const handleStartCreate = () => {
    setIsCreating(true)
    setFormData(emptyProjectForm)
  }

  const handleCancel = () => {
    setIsCreating(false)
    setFormData(emptyProjectForm)
  }

  const handleSaveCreate = () => {
    if (!formData.id.trim()) return
    const safetyHook = buildSafetyHookJSON(formData)
    createMutation.mutate({
      id: formData.id.trim(),
      name: formData.name.trim() || formData.id.trim(),
      root_path: formData.root_path.trim() || undefined,
      default_branch: formData.default_branch.trim() || undefined,
      use_git_worktrees: formData.use_git_worktrees,
      claude_safety_hook: safetyHook || undefined,
    })
  }

  const handleOpenSettings = (project: Project) => {
    setCurrentProject(project.id)
    navigate('/project-settings')
  }

  const handleDelete = (id: string) => {
    deleteMutation.mutate(id)
  }

  if (isLoading) {
    return (
      <div className="flex items-center justify-center py-12">
        <div className="text-muted-foreground">Loading projects...</div>
      </div>
    )
  }

  if (error) {
    return (
      <div className="flex items-center justify-center py-12">
        <div className="text-destructive">Error loading projects: {(error as Error).message}</div>
      </div>
    )
  }

  return (
    <Card>
      <CardHeader>
        <div className="flex items-center justify-between">
          <div>
            <CardTitle>Projects</CardTitle>
            <CardDescription>Manage your projects</CardDescription>
          </div>
          <Button onClick={handleStartCreate} disabled={isCreating}>
            <Plus className="h-4 w-4 mr-2" />
            New Project
          </Button>
        </div>
      </CardHeader>
      <CardContent>
        <div className="space-y-3">
          {isCreating && (
            <ProjectForm
              formData={formData}
              setFormData={setFormData}
              onCancel={handleCancel}
              onSave={handleSaveCreate}
              mutation={createMutation}
              isCreate
            />
          )}

          {projects.length === 0 && !isCreating ? (
            <div className="text-center py-8 text-muted-foreground">
              No projects found. Create one to get started.
            </div>
          ) : (
            projects.map((project) => (
              <ProjectListItem
                key={project.id}
                project={project}
                isDeleteConfirm={deleteConfirm === project.id}
                currentProject={currentProject}
                onOpenSettings={() => handleOpenSettings(project)}
                onDeleteConfirm={() => setDeleteConfirm(project.id)}
                onCancelDeleteConfirm={() => setDeleteConfirm(null)}
                onDelete={() => handleDelete(project.id)}
                isDeletePending={deleteMutation.isPending}
                projectsCount={projects.length}
              />
            ))
          )}
        </div>
      </CardContent>
    </Card>
  )
}
