import { useState } from 'react'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { updateProject, type UpdateProjectRequest } from '@/api/projects'
import { useSetArtifactStorage, useSetCleanup, useSetObserver } from './useProjectSettings'
import { buildSafetyHookJSON, type ProjectFormData } from '@/components/settings/projectFormUtils'
import { useProjectStore } from '@/stores/projectStore'
import type { ArtifactStorageConfig, CleanupSettings, ObserverSettings } from '@/api/projectSettings'

const projectListKey = ['projects', 'list'] as const

export function useSaveProjectSettings(projectId: string) {
  const queryClient = useQueryClient()
  const loadProjects = useProjectStore((s) => s.loadProjects)
  const [artifactError, setArtifactError] = useState<string | null>(null)
  const [cleanupError, setCleanupError] = useState<string | null>(null)
  const [observerError, setObserverError] = useState<string | null>(null)

  const updateMutation = useMutation({
    mutationFn: (data: UpdateProjectRequest) => updateProject(projectId, data),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: projectListKey })
      loadProjects()
    },
  })

  const setArtifactMutation = useSetArtifactStorage()
  const setCleanupMutation = useSetCleanup()
  const setObserverMutation = useSetObserver()

  const save = async (
    formData: ProjectFormData,
    subforms?: { artifact?: ArtifactStorageConfig; cleanup?: CleanupSettings; observer?: Partial<ObserverSettings> }
  ) => {
    setArtifactError(null)
    setCleanupError(null)
    setObserverError(null)
    const safetyHook = buildSafetyHookJSON(formData)

    const [, artifactResult, cleanupResult, observerResult] = await Promise.allSettled([
      updateMutation.mutateAsync({
        name: formData.name.trim() || undefined,
        root_path: formData.root_path.trim() || undefined,
        default_branch: formData.default_branch.trim() || undefined,
        use_git_worktrees: formData.use_git_worktrees,
        push_after_merge: formData.push_after_merge,
        claude_safety_hook: safetyHook,
      }),
      subforms?.artifact
        ? setArtifactMutation.mutateAsync({ projectId, cfg: subforms.artifact })
        : Promise.resolve(null),
      subforms?.cleanup
        ? setCleanupMutation.mutateAsync({ projectId, cfg: subforms.cleanup })
        : Promise.resolve(null),
      subforms?.observer
        ? setObserverMutation.mutateAsync({ projectId, cfg: subforms.observer })
        : Promise.resolve(null),
    ])

    if (artifactResult.status === 'rejected') {
      setArtifactError((artifactResult.reason as Error).message)
    }
    if (cleanupResult.status === 'rejected') {
      setCleanupError((cleanupResult.reason as Error).message)
    }
    if (observerResult.status === 'rejected') {
      setObserverError((observerResult.reason as Error).message)
    }
  }

  return {
    save,
    isPending:
      updateMutation.isPending ||
      setArtifactMutation.isPending ||
      setCleanupMutation.isPending ||
      setObserverMutation.isPending,
    isError: updateMutation.isError,
    error: updateMutation.error,
    artifactError,
    cleanupError,
    observerError,
  }
}
