import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { GitStatusTabContent } from './GitStatusTabContent'
import type { GitCommitsResponse } from '@/types/git'

// Mock the API module
const mockListGitCommits = vi.fn()
const mockGetGitCommitDetail = vi.fn()
vi.mock('@/api/git', () => ({
  listGitCommits: (...args: unknown[]) => mockListGitCommits(...args),
  getGitCommitDetail: (...args: unknown[]) => mockGetGitCommitDetail(...args),
}))

// Mock project store
const mockProjectStore = {
  currentProject: 'test-project',
  projectsLoaded: true,
}
vi.mock('@/stores/projectStore', () => ({
  useProjectStore: (selector: (state: typeof mockProjectStore) => unknown) =>
    selector(mockProjectStore),
}))

function renderGitStatusTab(projectId = 'proj-123') {
  const queryClient = new QueryClient({
    defaultOptions: {
      queries: { retry: false, gcTime: 0 },
    },
  })
  return render(
    <QueryClientProvider client={queryClient}>
      <GitStatusTabContent projectId={projectId} />
    </QueryClientProvider>
  )
}

function createMockCommitsResponse(overrides: Partial<GitCommitsResponse> = {}): GitCommitsResponse {
  return {
    commits: [
      {
        hash: 'abc123def456',
        short_hash: 'abc123d',
        author: 'John Doe',
        author_email: 'john@example.com',
        date: '2026-01-15T10:30:00Z',
        message: 'Add new feature',
      },
      {
        hash: 'def456ghi789',
        short_hash: 'def456g',
        author: 'Jane Smith',
        author_email: 'jane@example.com',
        date: '2026-01-14T14:20:00Z',
        message: 'Fix bug in login flow',
      },
    ],
    total: 2,
    page: 1,
    per_page: 20,
    ...overrides,
  }
}

describe('GitStatusTabContent - Loading and Error States', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('renders loading spinner while fetching commits', async () => {
    mockListGitCommits.mockImplementation(
      () => new Promise(() => {}) // Never resolves
    )

    renderGitStatusTab()

    expect(screen.getByRole('status')).toBeInTheDocument() // Spinner
  })

  it('renders error state when API call fails', async () => {
    mockListGitCommits.mockRejectedValue(new Error('Network error'))

    renderGitStatusTab()

    await waitFor(() => {
      expect(screen.getByText(/Failed to load commits/)).toBeInTheDocument()
    })
    expect(screen.getByText(/Network error/)).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /retry/i })).toBeInTheDocument()
  })

  it('renders helpful message for 400 error (no git repo)', async () => {
    mockListGitCommits.mockRejectedValue(new Error('Request failed with status code 400'))

    renderGitStatusTab()

    await waitFor(() => {
      expect(screen.getByText('No git repository configured for this project')).toBeInTheDocument()
    })
  })

  it('renders empty state when no commits found', async () => {
    mockListGitCommits.mockResolvedValue(createMockCommitsResponse({ commits: [], total: 0 }))

    renderGitStatusTab()

    await waitFor(() => {
      expect(screen.getByText('No commits found')).toBeInTheDocument()
    })
  })

  it('renders "No project selected" when projectId is empty', () => {
    renderGitStatusTab('')

    expect(screen.getByText('No project selected')).toBeInTheDocument()
  })
})

describe('GitStatusTabContent - Commit List Rendering', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('renders commit list with all commit details', async () => {
    mockListGitCommits.mockResolvedValue(createMockCommitsResponse())

    renderGitStatusTab()

    await waitFor(() => {
      expect(screen.getByText('abc123d')).toBeInTheDocument()
    })

    // Check first commit
    expect(screen.getByText('abc123d')).toBeInTheDocument()
    expect(screen.getByText('Add new feature')).toBeInTheDocument()
    expect(screen.getByText('John Doe')).toBeInTheDocument()

    // Check second commit
    expect(screen.getByText('def456g')).toBeInTheDocument()
    expect(screen.getByText('Fix bug in login flow')).toBeInTheDocument()
    expect(screen.getByText('Jane Smith')).toBeInTheDocument()
  })

  it('displays commit count', async () => {
    mockListGitCommits.mockResolvedValue(createMockCommitsResponse({ total: 42 }))

    renderGitStatusTab()

    await waitFor(() => {
      expect(screen.getByText('42 commits total')).toBeInTheDocument()
    })
  })

  it('displays singular "commit" for count of 1', async () => {
    mockListGitCommits.mockResolvedValue(
      createMockCommitsResponse({
        commits: [
          {
            hash: 'abc123',
            short_hash: 'abc123',
            author: 'Test',
            author_email: 'test@example.com',
            date: '2026-01-15T10:30:00Z',
            message: 'Single commit',
          },
        ],
        total: 1,
      })
    )

    renderGitStatusTab()

    await waitFor(() => {
      expect(screen.getByText('1 commit total')).toBeInTheDocument()
    })
  })

  it('truncates long commit messages', async () => {
    const longMessage = 'First line\nSecond line\nThird line'
    mockListGitCommits.mockResolvedValue(
      createMockCommitsResponse({
        commits: [
          {
            hash: 'abc123',
            short_hash: 'abc123',
            author: 'Test',
            author_email: 'test@example.com',
            date: '2026-01-15T10:30:00Z',
            message: longMessage,
          },
        ],
        total: 1,
      })
    )

    renderGitStatusTab()

    await waitFor(() => {
      expect(screen.getByText('First line')).toBeInTheDocument()
    })
    // Should only show first line
    expect(screen.queryByText('Second line')).not.toBeInTheDocument()
  })
})

describe('GitStatusTabContent - Pagination', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('displays pagination controls with correct page info', async () => {
    mockListGitCommits.mockResolvedValue(createMockCommitsResponse({ total: 100, page: 1, per_page: 20 }))

    renderGitStatusTab()

    await waitFor(() => {
      expect(screen.getByText('Page 1 of 5')).toBeInTheDocument()
    })
  })

  it('disables previous button on first page', async () => {
    mockListGitCommits.mockResolvedValue(createMockCommitsResponse({ total: 100, page: 1 }))

    const { container } = renderGitStatusTab()

    await waitFor(() => {
      const prevButton = container.querySelector('.lucide-chevron-left')?.closest('button')
      expect(prevButton).toBeDisabled()
    })
  })

  it('disables next button on last page', async () => {
    mockListGitCommits.mockResolvedValue(createMockCommitsResponse({ total: 20, page: 1, per_page: 20 }))

    const { container } = renderGitStatusTab()

    await waitFor(() => {
      const nextButton = container.querySelector('.lucide-chevron-right')?.closest('button')
      expect(nextButton).toBeDisabled()
    })
  })

  it('advances to next page when next button clicked', async () => {
    const user = userEvent.setup()
    mockListGitCommits.mockResolvedValue(createMockCommitsResponse({ total: 100, page: 1 }))

    const { container } = renderGitStatusTab()

    await waitFor(() => {
      expect(screen.getByText('Page 1 of 5')).toBeInTheDocument()
    })

    const nextButton = container.querySelector('.lucide-chevron-right')?.closest('button')
    await user.click(nextButton!)

    await waitFor(() => {
      expect(mockListGitCommits).toHaveBeenCalledWith('proj-123', 2, 20)
    })
  })

  it('goes back to previous page when prev button clicked', async () => {
    const user = userEvent.setup()
    // Start on page 2
    mockListGitCommits.mockResolvedValue(createMockCommitsResponse({ total: 100, page: 2 }))

    const { container } = renderGitStatusTab()

    // First advance to page 2
    await waitFor(() => {
      expect(screen.getByText('Page 1 of 5')).toBeInTheDocument()
    })

    const nextButton = container.querySelector('.lucide-chevron-right')?.closest('button')
    await user.click(nextButton!)

    await waitFor(() => {
      expect(mockListGitCommits).toHaveBeenCalledWith('proj-123', 2, 20)
    })

    // Then go back
    const prevButton = container.querySelector('.lucide-chevron-left')?.closest('button')
    await user.click(prevButton!)

    await waitFor(() => {
      expect(mockListGitCommits).toHaveBeenCalledWith('proj-123', 1, 20)
    })
  })
})

describe('GitStatusTabContent - Refresh Functionality', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('renders refresh button', async () => {
    mockListGitCommits.mockResolvedValue(createMockCommitsResponse())

    renderGitStatusTab()

    await waitFor(() => {
      expect(screen.getByRole('button', { name: /refresh/i })).toBeInTheDocument()
    })
  })

  it('refetches data when refresh button clicked', async () => {
    const user = userEvent.setup()
    mockListGitCommits.mockResolvedValue(createMockCommitsResponse())

    renderGitStatusTab()

    await waitFor(() => {
      expect(mockListGitCommits).toHaveBeenCalledTimes(1)
    })

    const refreshButton = screen.getByRole('button', { name: /refresh/i })
    await user.click(refreshButton)

    await waitFor(() => {
      expect(mockListGitCommits).toHaveBeenCalledTimes(2)
    })
  })

  it('disables refresh button while fetching', async () => {
    let resolvePromise: () => void
    const promise = new Promise<GitCommitsResponse>((resolve) => {
      resolvePromise = () => resolve(createMockCommitsResponse())
    })
    mockListGitCommits.mockReturnValue(promise)

    const user = userEvent.setup()
    renderGitStatusTab()

    await waitFor(() => {
      expect(screen.getByRole('button', { name: /refresh/i })).toBeInTheDocument()
    })

    const refreshButton = screen.getByRole('button', { name: /refresh/i })
    expect(refreshButton).toBeDisabled()

    resolvePromise!()
    await waitFor(() => {
      expect(refreshButton).not.toBeDisabled()
    })
  })
})

describe('GitStatusTabContent - Commit Detail Dialog', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('opens commit detail dialog when commit row clicked', async () => {
    const user = userEvent.setup()
    mockListGitCommits.mockResolvedValue(createMockCommitsResponse())
    mockGetGitCommitDetail.mockResolvedValue({
      commit: {
        hash: 'abc123def456',
        short_hash: 'abc123d',
        author: 'John Doe',
        author_email: 'john@example.com',
        date: '2026-01-15T10:30:00Z',
        message: 'Add new feature',
        files: [],
        diff: '',
      },
    })

    renderGitStatusTab()

    await waitFor(() => {
      expect(screen.getByText('abc123d')).toBeInTheDocument()
    })

    const commitRow = screen.getByText('Add new feature').closest('button')
    await user.click(commitRow!)

    await waitFor(() => {
      expect(screen.getByText('Commit Details')).toBeInTheDocument()
    })
  })
})

describe('GitStatusTabContent - Edge Cases', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('handles total pages = 1 correctly', async () => {
    mockListGitCommits.mockResolvedValue(createMockCommitsResponse({ total: 10, page: 1, per_page: 20 }))

    renderGitStatusTab()

    await waitFor(() => {
      expect(screen.getByText('Page 1 of 1')).toBeInTheDocument()
    })
  })

  it('retry button refetches after error', async () => {
    const user = userEvent.setup()
    mockListGitCommits.mockRejectedValueOnce(new Error('Network error'))
    mockListGitCommits.mockResolvedValueOnce(createMockCommitsResponse())

    renderGitStatusTab()

    await waitFor(() => {
      expect(screen.getByText(/Failed to load commits/)).toBeInTheDocument()
    })

    const retryButton = screen.getByRole('button', { name: /retry/i })
    await user.click(retryButton)

    await waitFor(() => {
      expect(screen.getByText('abc123d')).toBeInTheDocument()
    })
  })
})
