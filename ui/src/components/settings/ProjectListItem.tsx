import { Settings, Trash2, FolderOpen } from 'lucide-react'
import { Button } from '@/components/ui/Button'
import type { Project } from '@/api/projects'

interface ProjectListItemProps {
  project: Project
  isDeleteConfirm: boolean
  currentProject: string
  onOpenSettings: () => void
  onDeleteConfirm: () => void
  onCancelDeleteConfirm: () => void
  onDelete: () => void
  isDeletePending: boolean
  projectsCount: number
}

export function ProjectListItem({
  project,
  isDeleteConfirm,
  currentProject,
  onOpenSettings,
  onDeleteConfirm,
  onCancelDeleteConfirm,
  onDelete,
  isDeletePending,
  projectsCount,
}: ProjectListItemProps) {
  return (
    <div
      className={`border rounded-lg p-4 ${
        project.id === currentProject ? 'border-primary bg-primary/5' : ''
      }`}
    >
      {isDeleteConfirm ? (
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
            <Button variant="ghost" size="icon" onClick={onOpenSettings} title="Settings">
              <Settings className="h-4 w-4" />
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
