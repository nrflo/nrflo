import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import {
  getArtifactStorage,
  setArtifactStorage,
  getCleanup,
  setCleanup,
  getObserver,
  setObserver,
  type ArtifactStorageConfig,
  type CleanupSettings,
  type ObserverSettings,
} from '@/api/projectSettings'

export const projectSettingsKeys = {
  all: ['project-settings'] as const,
  artifactStorage: (projectId: string) => [...projectSettingsKeys.all, 'artifact-storage', projectId] as const,
  cleanup: (projectId: string) => [...projectSettingsKeys.all, 'cleanup', projectId] as const,
  observer: (projectId: string) => [...projectSettingsKeys.all, 'observer', projectId] as const,
}

export function useArtifactStorage(projectId: string) {
  return useQuery({
    queryKey: projectSettingsKeys.artifactStorage(projectId),
    queryFn: () => getArtifactStorage(projectId),
    enabled: !!projectId,
  })
}

export function useSetArtifactStorage() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ projectId, cfg }: { projectId: string; cfg: ArtifactStorageConfig }) =>
      setArtifactStorage(projectId, cfg),
    onSuccess: (_data, { projectId }) => {
      qc.invalidateQueries({ queryKey: projectSettingsKeys.artifactStorage(projectId) })
    },
  })
}

export function useCleanup(projectId: string) {
  return useQuery({
    queryKey: projectSettingsKeys.cleanup(projectId),
    queryFn: () => getCleanup(projectId),
    enabled: !!projectId,
  })
}

export function useSetCleanup() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ projectId, cfg }: { projectId: string; cfg: CleanupSettings }) =>
      setCleanup(projectId, cfg),
    onSuccess: (_data, { projectId }) => {
      qc.invalidateQueries({ queryKey: projectSettingsKeys.cleanup(projectId) })
    },
  })
}

export function useObserver(projectId: string) {
  return useQuery({
    queryKey: projectSettingsKeys.observer(projectId),
    queryFn: () => getObserver(projectId),
    enabled: !!projectId,
  })
}

export function useSetObserver() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ projectId, cfg }: { projectId: string; cfg: Partial<ObserverSettings> }) =>
      setObserver(projectId, cfg),
    onSuccess: (_data, { projectId }) => {
      qc.invalidateQueries({ queryKey: projectSettingsKeys.observer(projectId) })
    },
  })
}
