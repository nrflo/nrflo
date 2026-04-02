import { useQuery } from '@tanstack/react-query'
import { fetchErrors, type FetchErrorsParams } from '@/api/errors'
import { useProjectStore } from '@/stores/projectStore'

export const errorKeys = {
  all: ['errors'] as const,
  lists: () => [...errorKeys.all, 'list'] as const,
  list: (params?: FetchErrorsParams) => [...errorKeys.lists(), params] as const,
}

export function useErrors(params?: FetchErrorsParams) {
  const project = useProjectStore((s) => s.currentProject)
  const projectsLoaded = useProjectStore((s) => s.projectsLoaded)
  return useQuery({
    queryKey: [...errorKeys.list(params), project],
    queryFn: () => fetchErrors(params),
    enabled: projectsLoaded,
  })
}
