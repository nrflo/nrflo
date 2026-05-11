import { useQuery } from '@tanstack/react-query'
import { getEnvVarCatalog } from '@/api/specImport'

export function useEnvVarCatalog() {
  return useQuery({
    queryKey: ['env-var-catalog'],
    queryFn: getEnvVarCatalog,
    staleTime: Infinity,
    gcTime: Infinity,
  })
}
