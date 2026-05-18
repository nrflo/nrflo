import { Pencil, Trash2, FolderOpen } from 'lucide-react'
import { Button } from '@/components/ui/Button'
import { ProjectForm } from './ProjectForm'
import type { ProjectFormData } from './projectFormUtils'
import type { Project } from '@/api/projects'
import type { ArtifactStorageConfig, CleanupSettings } from '@/api/projectSettings'

interface EditMutation {
  isPending: boolean
  isError: boolean
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  error: any
  artifactError?: string | null
  cleanupError?: string | null
}

interface ProjectListItemProps {
  project: Project
  isEditing: boolean
  isDeleteConfirm: boolean
  currentProject: string
  formData: ProjectFormData
  setFormData: (d: ProjectFormData) => void
  onStartEdit: () => void
  onCancelEdit: () => void
  onSaveEdit: (subforms?: { artifact?: ArtifactStorageConfig; cleanup?: CleanupSettings }) => void
  onDeleteConfirm: () => void
  onCancelDeleteConfirm: () => void
  onDelete: () => void
  editMutation: EditMutation
  isDeletePending: boolean
  projectsCount: number
}

export function ProjectListItem({
  project,
  isEditing,
  isDeleteConfirm,
  currentProject,
  formData,
  setFormData,
  onStartEdit,
  onCancelEdit,
  onSaveEdit,
  onDeleteConfirm,
  onCancelDeleteConfirm,
  onDelete,
  editMutation,
  isDeletePending,
  projectsCount,
}: ProjectListItemProps) {
  return (
    <div
      className={`border rounded-lg p-4 ${
        project.id === currentProject ? 'border-primary bg-primary/5' : ''
      }`}
    >
      {isEditing ? (
        <ProjectForm
          formData={formData}
          setFormData={setFormData}
          onCancel={onCancelEdit}
          onSave={onSaveEdit}
          mutation={editMutation}
          disabledId={project.id}
        />
      ) : isDeleteConfirm ? (
        <div className="flex items-center justify-between">
          <div className="text-sm">
            Are you sure you want to delete{' '}
            <span className="font-semibold">{project.name}</span>?
          </div>
          <div className="flex gap-2">
            <Button variant="ghost" onClick={onCancelDeleteConfirm}>Cancel</Button>
            <Button variant="destructive" onClick={onDelete} disabled={isDeletePending}>
              {isDeletePending ? 'Deleting...' : 'Delete'}
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
                  project.push_after_merge && 'Push after merge: enabled',
                  project.claude_safety_hook && 'Safety hook: enabled',
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
            <Button variant="ghost" size="icon" onClick={onStartEdit}>
              <Pencil className="h-4 w-4" />
            </Button>
            <Button
              variant="ghost"
              size="icon"
              onClick={onDeleteConfirm}
              disabled={projectsCount === 1}
              title={projectsCount === 1 ? "Can't delete the last project" : 'Delete'}
            >
              <Trash2 className="h-4 w-4" />
            </Button>
          </div>
        </div>
      )}
    </div>
  )
}
