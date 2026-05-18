import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import {
  listPythonScripts,
  getPythonScript,
  createPythonScript,
  updatePythonScript,
  deletePythonScript,
  validatePythonScript,
  browseDirectory,
  readPythonFile,
} from '@/api/pythonScripts'
import type { PythonScriptCreateRequest, PythonScriptUpdateRequest, PythonToolCreateRequest, PythonToolUpdateRequest } from '@/types/pythonScript'

export const pythonScriptKeys = {
  all: ['python-scripts'] as const,
  list: (kind?: 'agent' | 'tool') => [...pythonScriptKeys.all, 'list', kind ?? 'all'] as const,
  detail: (id: string) => [...pythonScriptKeys.all, 'detail', id] as const,
}

export function usePythonScripts(kind?: 'agent' | 'tool') {
  return useQuery({
    queryKey: pythonScriptKeys.list(kind),
    queryFn: () => listPythonScripts(kind),
  })
}

export function usePythonScript(id: string) {
  return useQuery({
    queryKey: pythonScriptKeys.detail(id),
    queryFn: () => getPythonScript(id),
    enabled: !!id,
  })
}

export function useCreatePythonScript() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (data: PythonScriptCreateRequest | PythonToolCreateRequest) => createPythonScript(data),
    onSuccess: () => qc.invalidateQueries({ queryKey: pythonScriptKeys.all }),
  })
}

export function useUpdatePythonScript() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ id, data }: { id: string; data: PythonScriptUpdateRequest | PythonToolUpdateRequest }) =>
      updatePythonScript(id, data),
    onSuccess: () => qc.invalidateQueries({ queryKey: pythonScriptKeys.all }),
  })
}

export function useDeletePythonScript() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: deletePythonScript,
    onSuccess: () => qc.invalidateQueries({ queryKey: pythonScriptKeys.all }),
  })
}

export function useValidatePythonScript() {
  return useMutation({
    mutationFn: validatePythonScript,
  })
}

export function useBrowsePythonDir(path?: string) {
  return useQuery({
    queryKey: [...pythonScriptKeys.all, 'browse', path ?? ''] as const,
    queryFn: () => browseDirectory(path),
    staleTime: 0,
  })
}

export function useReadPythonFile(path: string | null) {
  return useQuery({
    queryKey: [...pythonScriptKeys.all, 'file', path ?? ''] as const,
    queryFn: () => readPythonFile(path!),
    enabled: !!path,
  })
}
