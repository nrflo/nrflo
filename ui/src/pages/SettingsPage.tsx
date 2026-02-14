import { useState } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { Settings, Plus, Pencil, Trash2, X, Check, FolderOpen } from 'lucide-react'
import { Button } from '@/components/ui/Button'
import { Card, CardContent, CardHeader, CardTitle, CardDescription } from '@/components/ui/Card'
import { Input } from '@/components/ui/Input'
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

// Query key for projects
const projectKeys = {
  all: ['projects'] as const,
  list: () => [...projectKeys.all, 'list'] as const,
}

interface ProjectFormData {
  id: string
  name: string
  root_path: string
  default_workflow: string
  default_branch: string
}

const emptyForm: ProjectFormData = {
  id: '',
  name: '',
  root_path: '',
  default_workflow: '',
  default_branch: '',
}

export function SettingsPage() {
  const queryClient = useQueryClient()
  const { currentProject, setCurrentProject, loadProjects } = useProjectStore()

  const [isCreating, setIsCreating] = useState(false)
  const [editingId, setEditingId] = useState<string | null>(null)
  const [formData, setFormData] = useState<ProjectFormData>(emptyForm)
  const [deleteConfirm, setDeleteConfirm] = useState<string | null>(null)

  // Fetch projects
  const { data, isLoading, error } = useQuery({
    queryKey: projectKeys.list(),
    queryFn: listProjects,
  })

  const projects = data?.projects || []

  // Create mutation
  const createMutation = useMutation({
    mutationFn: (data: CreateProjectRequest) => createProject(data),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: projectKeys.list() })
      loadProjects()
      setIsCreating(false)
      setFormData(emptyForm)
    },
  })

  // Update mutation
  const updateMutation = useMutation({
    mutationFn: ({ id, data }: { id: string; data: UpdateProjectRequest }) =>
      updateProject(id, data),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: projectKeys.list() })
      loadProjects()
      setEditingId(null)
      setFormData(emptyForm)
    },
  })

  // Delete mutation
  const deleteMutation = useMutation({
    mutationFn: (id: string) => deleteProject(id),
    onSuccess: (_, deletedId) => {
      queryClient.invalidateQueries({ queryKey: projectKeys.list() })
      loadProjects()
      setDeleteConfirm(null)
      // If we deleted the current project, switch to another
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
    setFormData(emptyForm)
  }

  const handleStartEdit = (project: Project) => {
    setEditingId(project.id)
    setIsCreating(false)
    setFormData({
      id: project.id,
      name: project.name,
      root_path: project.root_path || '',
      default_workflow: project.default_workflow || '',
      default_branch: project.default_branch || '',
    })
  }

  const handleCancel = () => {
    setIsCreating(false)
    setEditingId(null)
    setFormData(emptyForm)
  }

  const handleSaveCreate = () => {
    if (!formData.id.trim()) return
    createMutation.mutate({
      id: formData.id.trim(),
      name: formData.name.trim() || formData.id.trim(),
      root_path: formData.root_path.trim() || undefined,
      default_workflow: formData.default_workflow.trim() || undefined,
      default_branch: formData.default_branch.trim() || undefined,
    })
  }

  const handleSaveEdit = () => {
    if (!editingId) return
    updateMutation.mutate({
      id: editingId,
      data: {
        name: formData.name.trim() || undefined,
        root_path: formData.root_path.trim() || undefined,
        default_workflow: formData.default_workflow.trim() || undefined,
        default_branch: formData.default_branch.trim() || undefined,
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
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-3">
          <Settings className="h-6 w-6 text-muted-foreground" />
          <h1 className="text-2xl font-bold tracking-tight">Settings</h1>
        </div>
      </div>

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
            {/* Create form */}
            {isCreating && (
              <div className="border border-primary rounded-lg p-4 space-y-3 bg-muted/30">
                <div className="grid grid-cols-2 gap-3">
                  <div>
                    <label className="text-sm font-medium text-muted-foreground">
                      ID <span className="text-destructive">*</span>
                    </label>
                    <Input
                      value={formData.id}
                      onChange={(e) => setFormData({ ...formData, id: e.target.value })}
                      placeholder="project-id"
                    />
                  </div>
                  <div>
                    <label className="text-sm font-medium text-muted-foreground">Name</label>
                    <Input
                      value={formData.name}
                      onChange={(e) => setFormData({ ...formData, name: e.target.value })}
                      placeholder="Project Name"
                    />
                  </div>
                </div>
                <div className="grid grid-cols-2 gap-3">
                  <div>
                    <label className="text-sm font-medium text-muted-foreground">Root Path</label>
                    <Input
                      value={formData.root_path}
                      onChange={(e) => setFormData({ ...formData, root_path: e.target.value })}
                      placeholder="/path/to/project"
                    />
                  </div>
                  <div>
                    <label className="text-sm font-medium text-muted-foreground">
                      Default Workflow
                    </label>
                    <Input
                      value={formData.default_workflow}
                      onChange={(e) =>
                        setFormData({ ...formData, default_workflow: e.target.value })
                      }
                      placeholder="implementation"
                    />
                  </div>
                </div>
                <div className="grid grid-cols-2 gap-3">
                  <div>
                    <label className="text-sm font-medium text-muted-foreground">
                      Default Branch
                    </label>
                    <Input
                      value={formData.default_branch}
                      onChange={(e) =>
                        setFormData({ ...formData, default_branch: e.target.value })
                      }
                      placeholder="main"
                    />
                  </div>
                </div>
                <div className="flex gap-2 justify-end">
                  <Button variant="ghost" onClick={handleCancel}>
                    Cancel
                  </Button>
                  <Button
                    onClick={handleSaveCreate}
                    disabled={!formData.id.trim() || createMutation.isPending}
                  >
                    {createMutation.isPending ? 'Creating...' : 'Create'}
                  </Button>
                </div>
                {createMutation.isError && (
                  <p className="text-sm text-destructive">
                    Error: {createMutation.error.message}
                  </p>
                )}
              </div>
            )}

            {/* Project list */}
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
                    // Edit form
                    <div className="space-y-3">
                      <div className="grid grid-cols-2 gap-3">
                        <div>
                          <label className="text-sm font-medium text-muted-foreground">ID</label>
                          <Input value={project.id} disabled className="bg-muted" />
                        </div>
                        <div>
                          <label className="text-sm font-medium text-muted-foreground">Name</label>
                          <Input
                            value={formData.name}
                            onChange={(e) => setFormData({ ...formData, name: e.target.value })}
                            placeholder="Project Name"
                          />
                        </div>
                      </div>
                      <div className="grid grid-cols-2 gap-3">
                        <div>
                          <label className="text-sm font-medium text-muted-foreground">
                            Root Path
                          </label>
                          <Input
                            value={formData.root_path}
                            onChange={(e) =>
                              setFormData({ ...formData, root_path: e.target.value })
                            }
                            placeholder="/path/to/project"
                          />
                        </div>
                        <div>
                          <label className="text-sm font-medium text-muted-foreground">
                            Default Workflow
                          </label>
                          <Input
                            value={formData.default_workflow}
                            onChange={(e) =>
                              setFormData({ ...formData, default_workflow: e.target.value })
                            }
                            placeholder="implementation"
                          />
                        </div>
                      </div>
                      <div className="grid grid-cols-2 gap-3">
                        <div>
                          <label className="text-sm font-medium text-muted-foreground">
                            Default Branch
                          </label>
                          <Input
                            value={formData.default_branch}
                            onChange={(e) =>
                              setFormData({ ...formData, default_branch: e.target.value })
                            }
                            placeholder="main"
                          />
                        </div>
                      </div>
                      <div className="flex gap-2 justify-end">
                        <Button variant="ghost" onClick={handleCancel}>
                          <X className="h-4 w-4 mr-1" />
                          Cancel
                        </Button>
                        <Button onClick={handleSaveEdit} disabled={updateMutation.isPending}>
                          <Check className="h-4 w-4 mr-1" />
                          {updateMutation.isPending ? 'Saving...' : 'Save'}
                        </Button>
                      </div>
                      {updateMutation.isError && (
                        <p className="text-sm text-destructive">
                          Error: {updateMutation.error.message}
                        </p>
                      )}
                    </div>
                  ) : deleteConfirm === project.id ? (
                    // Delete confirmation
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
                    // Display mode
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
                              project.default_workflow && `Workflow: ${project.default_workflow}`,
                              project.default_branch && `Branch: ${project.default_branch}`,
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
    </div>
  )
}
