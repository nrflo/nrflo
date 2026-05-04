import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import {
  listConfigFiles,
  getConfigFile,
  putConfigFile,
  getConfigHistory,
  rollbackConfig,
} from '@/api/configFiles'

export const configFileKeys = {
  all: ['config-files'] as const,
  files: ['config-files', 'files'] as const,
  content: (path: string) => ['config-files', 'content', path] as const,
  history: (path: string) => ['config-files', 'history', path] as const,
}

export function useConfigFiles() {
  return useQuery({
    queryKey: configFileKeys.files,
    queryFn: listConfigFiles,
  })
}

export function useConfigFile(path: string) {
  return useQuery({
    queryKey: configFileKeys.content(path),
    queryFn: () => getConfigFile(path),
    enabled: !!path,
  })
}

export function usePutConfigFile() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ path, content }: { path: string; content: string }) =>
      putConfigFile(path, content),
    onSuccess: (_data, { path }) => {
      qc.invalidateQueries({ queryKey: configFileKeys.content(path) })
      qc.invalidateQueries({ queryKey: configFileKeys.history(path) })
      qc.invalidateQueries({ queryKey: configFileKeys.files })
    },
  })
}

export function useConfigHistory(path: string) {
  return useQuery({
    queryKey: configFileKeys.history(path),
    queryFn: () => getConfigHistory(path),
    enabled: !!path,
  })
}

export function useRollbackConfig() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ path, version }: { path: string; version: number }) =>
      rollbackConfig(path, version),
    onSuccess: (_data, { path }) => {
      qc.invalidateQueries({ queryKey: configFileKeys.content(path) })
      qc.invalidateQueries({ queryKey: configFileKeys.history(path) })
      qc.invalidateQueries({ queryKey: configFileKeys.files })
    },
  })
}
