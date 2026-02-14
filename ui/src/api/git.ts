import { apiGet } from './client'
import type { GitCommitsResponse, GitCommitDetailResponse } from '@/types/git'

export async function listGitCommits(
  projectId: string,
  page = 1,
  perPage = 20
): Promise<GitCommitsResponse> {
  const params = new URLSearchParams()
  params.set('page', String(page))
  params.set('per_page', String(perPage))
  return apiGet<GitCommitsResponse>(
    `/api/v1/projects/${encodeURIComponent(projectId)}/git/commits?${params.toString()}`
  )
}

export async function getGitCommitDetail(
  projectId: string,
  hash: string
): Promise<GitCommitDetailResponse> {
  return apiGet<GitCommitDetailResponse>(
    `/api/v1/projects/${encodeURIComponent(projectId)}/git/commits/${encodeURIComponent(hash)}`
  )
}
