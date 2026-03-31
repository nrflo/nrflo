import { useState } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { Plus, Pencil, Trash2, FolderOpen } from 'lucide-react'
import { Button } from '@/components/ui/Button'
import { Card, CardContent, CardHeader, CardTitle, CardDescription } from '@/components/ui/Card'
import { useProjectStore } from '@/stores/projectStore'
import {
  listProjects,
  createProject,
  updateProject,
  deleteProject,
  type Project,
  type CreateProjectRequest,
  type UpdateProjectRequest,
} from '@/api/projects'
import { ProjectForm, emptyProjectForm, type ProjectFormData } from './ProjectForm'

const projectKeys = {
  all: ['projects'] as const,
  list: () => [...projectKeys.all, 'list'] as const,
}

export function ProjectsSection() {
  const queryClient = useQueryClient()
  const { currentProject, setCurrentProject, loadProjects } = useProjectStore()

  const [isCreating, setIsCreating] = useState(false)
  const [editingId, setEditingId] = useState<string | null>(null)
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

  const updateMutation = useMutation({
    mutationFn: ({ id, data }: { id: string; data: UpdateProjectRequest }) =>
      updateProject(id, data),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: projectKeys.list() })
      loadProjects()
      setEditingId(null)
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
    setEditingId(null)
    setFormData(emptyProjectForm)
  }

  const handleStartEdit = (project: Project) => {
    setEditingId(project.id)
    setIsCreating(false)
    setFormData({
      id: project.id,
      name: project.name,
      root_path: project.root_path || '',
      default_branch: project.default_branch || '',
      use_git_worktrees: project.use_git_worktrees || false,
    })
  }

  const handleCancel = () => {
    setIsCreating(false)
    setEditingId(null)
    setFormData(emptyProjectForm)
  }

  const handleSaveCreate = () => {
    if (!formData.id.trim()) return
    createMutation.mutate({
      id: formData.id.trim(),
      name: formData.name.trim() || formData.id.trim(),
      root_path: formData.root_path.trim() || undefined,
      default_branch: formData.default_branch.trim() || undefined,
      use_git_worktrees: formData.use_git_worktrees,
    })
  }

  const handleSaveEdit = () => {
    if (!editingId) return
    updateMutation.mutate({
      id: editingId,
      data: {
        name: formData.name.trim() || undefined,
        root_path: formData.root_path.trim() || undefined,
        default_branch: formData.default_branch.trim() || undefined,
        use_git_worktrees: formData.use_git_worktrees,
      },
    })
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
              <div
                key={project.id}
                className={`border rounded-lg p-4 ${
                  project.id === currentProject ? 'border-primary bg-primary/5' : ''
                }`}
              >
                {editingId === project.id ? (
                  <ProjectForm
                    formData={formData}
                    setFormData={setFormData}
                    onCancel={handleCancel}
                    onSave={handleSaveEdit}
                    mutation={updateMutation}
                    disabledId={project.id}
                  />
                ) : deleteConfirm === project.id ? (
                  <div className="flex items-center justify-between">
                    <div className="text-sm">
                      Are you sure you want to delete{' '}
                      <span className="font-semibold">{project.name}</span>?
                    </div>
                    <div className="flex gap-2">
                      <Button variant="ghost" onClick={() => setDeleteConfirm(null)}>
                        Cancel
                      </Button>
                      <Button
                        variant="destructive"
                        onClick={() => handleDelete(project.id)}
                        disabled={deleteMutation.isPending}
                      >
                        {deleteMutation.isPending ? 'Deleting...' : 'Delete'}
                      </Button>
                    </div>
                  </div>
                ) : (
                  <div className="flex items-center justify-between">
                    <div className="flex items-center gap-3">
                      <FolderOpen className="h-5 w-5 text-muted-foreground" />
                      <div>
                        <div className="flex items-center gap-2">
                          <span className="font-medium">{project.name}</span>
                          <span className="text-sm text-muted-foreground">({project.id})</span>
                          {project.id === currentProject && (
                            <span className="text-xs bg-primary text-primary-foreground px-2 py-0.5 rounded">
                              Active
                            </span>
                          )}
                        </div>
                        <div className="text-sm text-muted-foreground">
                          {[
                            project.root_path && `Path: ${project.root_path}`,
                            project.default_branch && `Branch: ${project.default_branch}`,
                            project.use_git_worktrees && 'Worktrees: enabled',
                          ]
                            .filter(Boolean)
                            .map((text, i, arr) => (
                              <span key={i}>
                                {text}
                                {i < arr.length - 1 && <span className="mx-2">|</span>}
                              </span>
                            ))}
                        </div>
                      </div>
                    </div>
                    <div className="flex gap-1">
                      <Button
                        variant="ghost"
                        size="icon"
                        onClick={() => handleStartEdit(project)}
                      >
                        <Pencil className="h-4 w-4" />
                      </Button>
                      <Button
                        variant="ghost"
                        size="icon"
                        onClick={() => setDeleteConfirm(project.id)}
                        disabled={projects.length === 1}
                        title={
                          projects.length === 1 ? "Can't delete the last project" : 'Delete'
                        }
                      >
                        <Trash2 className="h-4 w-4" />
                      </Button>
                    </div>
                  </div>
                )}
              </div>
            ))
          )}
        </div>
      </CardContent>
    </Card>
  )
}
