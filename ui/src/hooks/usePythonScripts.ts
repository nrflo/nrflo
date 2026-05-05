import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import {
  listPythonScripts,
  getPythonScript,
  createPythonScript,
  updatePythonScript,
  deletePythonScript,
  validatePythonScript,
} from '@/api/pythonScripts'
import type { PythonScriptCreateRequest, PythonScriptUpdateRequest } from '@/types/pythonScript'

export const pythonScriptKeys = {
  all: ['python-scripts'] as const,
  list: () => [...pythonScriptKeys.all, 'list'] as const,
  detail: (id: string) => [...pythonScriptKeys.all, 'detail', id] as const,
}

export function usePythonScripts() {
  return useQuery({
    queryKey: pythonScriptKeys.list(),
    queryFn: listPythonScripts,
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
    mutationFn: (data: PythonScriptCreateRequest) => createPythonScript(data),
    onSuccess: () => qc.invalidateQueries({ queryKey: pythonScriptKeys.all }),
  })
}

export function useUpdatePythonScript() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ id, data }: { id: string; data: PythonScriptUpdateRequest }) =>
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
