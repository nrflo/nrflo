import { X, Check } from 'lucide-react'
import { Button } from '@/components/ui/Button'
import { Input } from '@/components/ui/Input'
import { Toggle } from '@/components/ui/Toggle'
import { Tooltip } from '@/components/ui/Tooltip'

export interface ProjectFormData {
  id: string
  name: string
  root_path: string
  default_branch: string
  use_git_worktrees: boolean
}

export const emptyProjectForm: ProjectFormData = {
  id: '',
  name: '',
  root_path: '',
  default_branch: '',
  use_git_worktrees: false,
}

export function ProjectForm({
  formData,
  setFormData,
  onCancel,
  onSave,
  mutation,
  isCreate,
  disabledId,
}: {
  formData: ProjectFormData
  setFormData: (data: ProjectFormData) => void
  onCancel: () => void
  onSave: () => void
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  mutation: { isPending: boolean; isError: boolean; error: any }
  isCreate?: boolean
  disabledId?: string
}) {
  return (
    <div className={`space-y-3 ${isCreate ? 'border border-primary rounded-lg p-4 bg-muted/30' : ''}`}>
      <div className="grid grid-cols-2 gap-3">
        <div>
          <label className="text-sm font-medium text-muted-foreground">
            ID {isCreate && <span className="text-destructive">*</span>}
          </label>
          {isCreate ? (
            <Input
              value={formData.id}
              onChange={(e) => setFormData({ ...formData, id: e.target.value })}
              placeholder="project-id"
            />
          ) : (
            <Input value={disabledId} disabled className="bg-muted" />
          )}
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
          <label className="text-sm font-medium text-muted-foreground">Default Branch</label>
          <Input
            value={formData.default_branch}
            onChange={(e) => {
              const val = e.target.value
              setFormData({
                ...formData,
                default_branch: val,
                use_git_worktrees: val.trim() ? formData.use_git_worktrees : false,
              })
            }}
            placeholder="main"
          />
        </div>
        <div className="flex items-end pb-1">
          <Tooltip
            text="Git worktrees give each ticket workflow an isolated copy of the repo so agents don't interfere with each other or the main working directory.\n\nApplies to ticket-scoped workflows only. Requires default_branch to be configured.\n\nLifecycle: creates a feature branch + worktree at /tmp → agents work in isolation → on success, merges to default branch and cleans up → on failure, force-removes worktree and discards changes."
            placement="top"
            className="whitespace-normal max-w-sm"
          >
            <Toggle
              checked={formData.use_git_worktrees}
              onChange={(checked) => setFormData({ ...formData, use_git_worktrees: checked })}
              label="Use Git Worktrees"
              disabled={!formData.default_branch.trim()}
            />
          </Tooltip>
        </div>
      </div>
      <div className="flex gap-2 justify-end">
        <Button variant="ghost" onClick={onCancel}>
          {isCreate ? 'Cancel' : <><X className="h-4 w-4 mr-1" />Cancel</>}
        </Button>
        <Button
          onClick={onSave}
          disabled={isCreate ? !formData.id.trim() || mutation.isPending : mutation.isPending}
        >
          {isCreate ? (
            mutation.isPending ? 'Creating...' : 'Create'
          ) : (
            <>{mutation.isPending ? 'Saving...' : <><Check className="h-4 w-4 mr-1" />Save</>}</>
          )}
        </Button>
      </div>
      {mutation.isError && (
        <p className="text-sm text-destructive">
          Error: {mutation.error.message}
        </p>
      )}
    </div>
  )
}
