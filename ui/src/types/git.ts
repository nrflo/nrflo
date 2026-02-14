export interface GitCommit {
  hash: string
  short_hash: string
  author: string
  author_email: string
  date: string
  message: string
}

export interface GitChangedFile {
  path: string
  status: string
  additions: number
  deletions: number
}

export interface GitCommitDetail extends GitCommit {
  files: GitChangedFile[]
  diff: string
}

export interface GitCommitsResponse {
  commits: GitCommit[]
  total: number
  page: number
  per_page: number
}

export interface GitCommitDetailResponse {
  commit: GitCommitDetail
}
