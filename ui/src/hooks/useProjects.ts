import { useQuery, type UseQueryOptions } from '@tanstack/react-query'
import { listProjects, type ProjectsResponse } from '@/api/projects'

export const projectKeys = {
  all: ['projects'] as const,
  list: () => [...projectKeys.all, 'list'] as const,
}

export function useProjects(
  options?: Omit<UseQueryOptions<ProjectsResponse>, 'queryKey' | 'queryFn'>
) {
  return useQuery({
    queryKey: projectKeys.list(),
    queryFn: listProjects,
    staleTime: 5 * 60 * 1000, // Projects don't change often
    ...options,
  })
}
