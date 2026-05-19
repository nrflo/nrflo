import { useState } from 'react'
import { X, Check, Info, ShieldCheck } from 'lucide-react'
import { Button } from '@/components/ui/Button'
import { Input } from '@/components/ui/Input'
import { Textarea } from '@/components/ui/Textarea'
import { Toggle } from '@/components/ui/Toggle'
import { Tooltip } from '@/components/ui/Tooltip'
import { SafetyHookCheckDialog } from './SafetyHookCheckDialog'
import { ProjectEnvVarsEditor } from './ProjectEnvVarsEditor'
import { ProjectArtifactStorageEditor } from './ProjectArtifactStorageEditor'
import { ProjectCleanupEditor } from './ProjectCleanupEditor'
import { ProjectObserverEditor } from './ProjectObserverEditor'
import { useProjectSubforms } from './useProjectSubforms'
import type { ArtifactStorageConfig, CleanupSettings, ObserverSettings } from '@/api/projectSettings'
import { type ProjectFormData, emptyProjectForm } from './projectFormUtils'
export { type ProjectFormData, emptyProjectForm, parseSafetyHookConfig, buildSafetyHookJSON } from './projectFormUtils'

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
  onSave: (subforms?: { artifact?: ArtifactStorageConfig; cleanup?: CleanupSettings; observer?: Partial<ObserverSettings> }) => void
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  mutation: { isPending: boolean; isError: boolean; error: any; artifactError?: string | null; cleanupError?: string | null; observerError?: string | null }
  isCreate?: boolean
  disabledId?: string
}) {
  const [checkDialogOpen, setCheckDialogOpen] = useState(false)
  const isEdit = !isCreate && !!disabledId

  const { artifactValue, setArtifactValue, cleanupValue, setCleanupValue, observerValue, setObserverValue, buildChangedSubforms } =
    useProjectSubforms(disabledId ?? '')

  function handleSave() {
    if (isEdit) {
      onSave(buildChangedSubforms())
    } else {
      onSave()
    }
  }

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
          <div className="flex items-center gap-1.5">
            <label className="text-sm font-medium text-muted-foreground">Default Branch</label>
            <Tooltip
              placement="right"
              text="Branch displayed on the Git page. When worktrees are enabled, feature branches are created from this branch."
            >
              <Info className="h-3.5 w-3.5 text-muted-foreground" />
            </Tooltip>
          </div>
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
            text={
              <div className="space-y-2">
                <p>Git worktrees give each ticket workflow an isolated copy of the repo so agents don't interfere with each other or the main working directory.</p>
                <p>Applies to ticket-scoped workflows only. Requires default_branch to be configured.</p>
                <p>Lifecycle: creates a feature branch + worktree at /tmp → agents work in isolation → on success, merges to default branch and cleans up → on failure, force-removes worktree and discards changes.</p>
              </div>
            }
            placement="top"
            className="max-w-sm"
          >
            <Toggle
              checked={formData.use_git_worktrees}
              onChange={(checked) => setFormData({ ...formData, use_git_worktrees: checked })}
              label="Use Git Worktrees"
              disabled={!formData.default_branch.trim()}
            />
          </Tooltip>
        </div>
        <div className="flex items-end pb-1">
          <Tooltip
            text="Automatically push default branch to remote after successful worktree merge"
            placement="top"
            className="max-w-sm"
          >
            <Toggle
              checked={formData.push_after_merge}
              onChange={(checked) => setFormData({ ...formData, push_after_merge: checked })}
              label="Push after merge"
              disabled={!formData.default_branch.trim()}
            />
          </Tooltip>
        </div>
      </div>
      <div className="border-t border-border pt-3 space-y-3">
        <div className="text-sm font-medium text-muted-foreground">Safety Hook</div>
        <Toggle
          checked={formData.safety_hook_enabled}
          onChange={(checked) =>
            setFormData({
              ...formData,
              safety_hook_enabled: checked,
              safety_hook_allowed_rm_paths: checked && !formData.safety_hook_allowed_rm_paths
                ? emptyProjectForm.safety_hook_allowed_rm_paths
                : formData.safety_hook_allowed_rm_paths,
              safety_hook_dangerous_patterns: checked && !formData.safety_hook_dangerous_patterns
                ? emptyProjectForm.safety_hook_dangerous_patterns
                : formData.safety_hook_dangerous_patterns,
            })
          }
          label="Enable safety hook"
        />
        {formData.safety_hook_enabled && (
          <div className="space-y-3 pl-4 border-l-2 border-border">
            <Toggle
              checked={formData.safety_hook_allow_git}
              onChange={(checked) => setFormData({ ...formData, safety_hook_allow_git: checked })}
              label="Allow git operations"
            />
            <div>
              <label className="text-sm font-medium text-muted-foreground">Allowed rm paths (one per line)</label>
              <Textarea
                value={formData.safety_hook_allowed_rm_paths}
                onChange={(e) => setFormData({ ...formData, safety_hook_allowed_rm_paths: e.target.value })}
                rows={4}
                placeholder="node_modules&#10;dist&#10;build"
              />
            </div>
            <div>
              <label className="text-sm font-medium text-muted-foreground">Dangerous patterns (one per line)</label>
              <Textarea
                value={formData.safety_hook_dangerous_patterns}
                onChange={(e) => setFormData({ ...formData, safety_hook_dangerous_patterns: e.target.value })}
                rows={3}
                placeholder="rm -rf /&#10;DROP TABLE"
              />
            </div>
            <Button variant="outline" size="sm" onClick={() => setCheckDialogOpen(true)}>
              <ShieldCheck className="h-4 w-4 mr-1" />Check
            </Button>
            <SafetyHookCheckDialog
              open={checkDialogOpen}
              onClose={() => setCheckDialogOpen(false)}
              formData={formData}
            />
          </div>
        )}
      </div>
      {isEdit && disabledId && (
        <>
          <ProjectEnvVarsEditor projectId={disabledId} />
          <ProjectArtifactStorageEditor
            projectId={disabledId}
            value={artifactValue}
            onChange={setArtifactValue}
            serverError={mutation.artifactError}
          />
          <ProjectCleanupEditor
            projectId={disabledId}
            value={cleanupValue}
            onChange={setCleanupValue}
            serverError={mutation.cleanupError}
          />
          <ProjectObserverEditor
            projectId={disabledId}
            value={observerValue}
            onChange={setObserverValue}
            serverError={mutation.observerError}
          />
        </>
      )}
      <div className="flex gap-2 justify-end">
        <Button variant="ghost" onClick={onCancel}>
          {isCreate ? 'Cancel' : <><X className="h-4 w-4 mr-1" />Cancel</>}
        </Button>
        <Button
          onClick={handleSave}
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
