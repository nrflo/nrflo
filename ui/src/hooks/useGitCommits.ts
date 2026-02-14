import { useQuery, type UseQueryOptions } from '@tanstack/react-query'
import { useProjectStore } from '@/stores/projectStore'
import { listGitCommits, getGitCommitDetail } from '@/api/git'
import type { GitCommitsResponse, GitCommitDetailResponse } from '@/types/git'

export const gitKeys = {
  all: ['git-commits'] as const,
  lists: () => [...gitKeys.all, 'list'] as const,
  list: (projectId: string, page: number, perPage: number) =>
    [...gitKeys.lists(), projectId, page, perPage] as const,
  details: () => [...gitKeys.all, 'detail'] as const,
  detail: (projectId: string, hash: string) =>
    [...gitKeys.details(), projectId, hash] as const,
}

export function useGitCommits(
  projectId: string,
  page = 1,
  perPage = 20,
  options?: Omit<UseQueryOptions<GitCommitsResponse>, 'queryKey' | 'queryFn'>
) {
  const project = useProjectStore((s) => s.currentProject)
  const projectsLoaded = useProjectStore((s) => s.projectsLoaded)

  return useQuery({
    queryKey: [...gitKeys.list(projectId, page, perPage), project],
    queryFn: () => listGitCommits(projectId, page, perPage),
    enabled: projectsLoaded && !!projectId && (options?.enabled ?? true),
    ...options,
  })
}

export function useGitCommitDetail(
  projectId: string,
  hash: string | null,
  options?: Omit<UseQueryOptions<GitCommitDetailResponse>, 'queryKey' | 'queryFn'>
) {
  const project = useProjectStore((s) => s.currentProject)
  const projectsLoaded = useProjectStore((s) => s.projectsLoaded)

  return useQuery({
    queryKey: [...gitKeys.detail(projectId, hash ?? ''), project],
    queryFn: () => getGitCommitDetail(projectId, hash!),
    enabled: projectsLoaded && !!projectId && !!hash && (options?.enabled ?? true),
    ...options,
  })
}
