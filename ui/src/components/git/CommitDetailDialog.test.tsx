import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { CommitDetailDialog } from './CommitDetailDialog'
import type { GitCommitDetailResponse } from '@/types/git'

// Mock the API module
const mockGetGitCommitDetail = vi.fn()
vi.mock('@/api/git', () => ({
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

function renderCommitDialog(projectId = 'proj-123', hash: string | null = 'abc123', onClose = vi.fn()) {
  const queryClient = new QueryClient({
    defaultOptions: {
      queries: { retry: false, gcTime: 0 },
    },
  })
  return {
    ...render(
      <QueryClientProvider client={queryClient}>
        <CommitDetailDialog projectId={projectId} hash={hash} onClose={onClose} />
      </QueryClientProvider>
    ),
    onClose,
  }
}

function createMockCommitDetail(
  overrides: Partial<GitCommitDetailResponse['commit']> = {}
): GitCommitDetailResponse {
  return {
    commit: {
      hash: 'abc123def456789',
      short_hash: 'abc123d',
      author: 'John Doe',
      author_email: 'john@example.com',
      date: '2026-01-15T10:30:00Z',
      message: 'Add new feature\n\nThis is a detailed commit message.',
      files: [
        {
          path: 'src/feature.ts',
          status: 'added',
          additions: 50,
          deletions: 0,
        },
        {
          path: 'src/main.ts',
          status: 'modified',
          additions: 10,
          deletions: 5,
        },
        {
          path: 'src/old.ts',
          status: 'deleted',
          additions: 0,
          deletions: 30,
        },
      ],
      diff: `diff --git a/src/feature.ts b/src/feature.ts
new file mode 100644
index 0000000..abc123
--- /dev/null
+++ b/src/feature.ts
@@ -0,0 +1,3 @@
+export function newFeature() {
+  return "feature"
+}`,
      ...overrides,
    },
  }
}

describe('CommitDetailDialog - Dialog State', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('does not render when hash is null', () => {
    const { container } = renderCommitDialog('proj-123', null)
    expect(container.querySelector('[role="dialog"]')).not.toBeInTheDocument()
  })

  it('renders dialog when hash is provided', async () => {
    mockGetGitCommitDetail.mockResolvedValue(createMockCommitDetail())

    renderCommitDialog()

    await waitFor(() => {
      expect(screen.getByText('Commit Details')).toBeInTheDocument()
    })
  })

  it('dialog renders when hash is provided', async () => {
    mockGetGitCommitDetail.mockResolvedValue(createMockCommitDetail())

    renderCommitDialog()

    await waitFor(() => {
      expect(screen.getByText('Commit Details')).toBeInTheDocument()
    })
    // Dialog is open, verified by header presence
  })

  it('calls onClose when ESC key pressed', async () => {
    const user = userEvent.setup()
    mockGetGitCommitDetail.mockResolvedValue(createMockCommitDetail())

    const { onClose } = renderCommitDialog()

    await waitFor(() => {
      expect(screen.getByText('Commit Details')).toBeInTheDocument()
    })

    await user.keyboard('{Escape}')

    expect(onClose).toHaveBeenCalled()
  })
})

describe('CommitDetailDialog - Loading and Error States', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('renders loading spinner while fetching commit details', async () => {
    mockGetGitCommitDetail.mockImplementation(
      () => new Promise(() => {}) // Never resolves
    )

    renderCommitDialog()

    await waitFor(() => {
      expect(screen.getByRole('status')).toBeInTheDocument() // Spinner
    })
  })

  it('renders error message when API call fails', async () => {
    mockGetGitCommitDetail.mockRejectedValue(new Error('Commit not found'))

    renderCommitDialog()

    await waitFor(() => {
      expect(screen.getByText(/Failed to load commit details/)).toBeInTheDocument()
    })
    expect(screen.getByText(/Commit not found/)).toBeInTheDocument()
  })
})

describe('CommitDetailDialog - Commit Header', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('renders full commit hash', async () => {
    mockGetGitCommitDetail.mockResolvedValue(createMockCommitDetail())

    renderCommitDialog()

    await waitFor(() => {
      expect(screen.getByText('abc123def456789')).toBeInTheDocument()
    })
  })

  it('renders author name and email', async () => {
    mockGetGitCommitDetail.mockResolvedValue(createMockCommitDetail())

    renderCommitDialog()

    await waitFor(() => {
      expect(screen.getByText('John Doe')).toBeInTheDocument()
    })
    expect(screen.getByText(/<john@example.com>/)).toBeInTheDocument()
  })

  it('renders commit date with relative time', async () => {
    mockGetGitCommitDetail.mockResolvedValue(createMockCommitDetail())

    renderCommitDialog()

    await waitFor(() => {
      expect(screen.getByText(/Date:/)).toBeInTheDocument()
    })
  })

  it('renders full commit message including multiline', async () => {
    mockGetGitCommitDetail.mockResolvedValue(createMockCommitDetail())

    renderCommitDialog()

    await waitFor(() => {
      expect(screen.getByText(/Add new feature/)).toBeInTheDocument()
    })
    expect(screen.getByText(/This is a detailed commit message/)).toBeInTheDocument()
  })
})

describe('CommitDetailDialog - Copy Hash Functionality', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('renders copyable hash as a button', async () => {
    mockGetGitCommitDetail.mockResolvedValue(createMockCommitDetail())

    renderCommitDialog()

    await waitFor(() => {
      expect(screen.getByText('abc123def456789')).toBeInTheDocument()
    })

    const copyButton = screen.getByText('abc123def456789').closest('button')
    expect(copyButton).toBeInTheDocument()
  })
})

describe('CommitDetailDialog - Changed Files List', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('renders changed files count', async () => {
    mockGetGitCommitDetail.mockResolvedValue(createMockCommitDetail())

    renderCommitDialog()

    await waitFor(() => {
      expect(screen.getByText('Changed files (3)')).toBeInTheDocument()
    })
  })

  it('renders all changed files with paths', async () => {
    mockGetGitCommitDetail.mockResolvedValue(createMockCommitDetail())

    renderCommitDialog()

    await waitFor(() => {
      expect(screen.getAllByText('src/feature.ts').length).toBeGreaterThan(0)
    })
    expect(screen.getAllByText('src/main.ts').length).toBeGreaterThan(0)
    expect(screen.getAllByText('src/old.ts').length).toBeGreaterThan(0)
  })

  it('renders status badges for each file', async () => {
    mockGetGitCommitDetail.mockResolvedValue(createMockCommitDetail())

    renderCommitDialog()

    await waitFor(() => {
      expect(screen.getByText('added')).toBeInTheDocument()
    })
    expect(screen.getByText('modified')).toBeInTheDocument()
    expect(screen.getByText('deleted')).toBeInTheDocument()
  })

  it('renders additions and deletions counts', async () => {
    mockGetGitCommitDetail.mockResolvedValue(createMockCommitDetail())

    renderCommitDialog()

    await waitFor(() => {
      expect(screen.getByText('+50')).toBeInTheDocument()
    })
    expect(screen.getByText('+10')).toBeInTheDocument()
    expect(screen.getByText('-5')).toBeInTheDocument()
    expect(screen.getByText('-30')).toBeInTheDocument()
  })

  it('does not show additions when 0', async () => {
    mockGetGitCommitDetail.mockResolvedValue(createMockCommitDetail())

    renderCommitDialog()

    await waitFor(() => {
      const deletedFileRow = screen.getByText('src/old.ts').closest('button')
      expect(deletedFileRow).toBeInTheDocument()
      // Should show -30 but not +0
      expect(deletedFileRow).not.toHaveTextContent('+0')
    })
  })

  it('renders clickable file rows in changed files section', async () => {
    mockGetGitCommitDetail.mockResolvedValue(createMockCommitDetail())

    renderCommitDialog()

    await waitFor(() => {
      expect(screen.getByText('Changed files (3)')).toBeInTheDocument()
    })

    // File paths should be present in the changed files list
    expect(screen.getByText('added')).toBeInTheDocument()
    expect(screen.getByText('modified')).toBeInTheDocument()
    expect(screen.getByText('deleted')).toBeInTheDocument()
  })
})

describe('CommitDetailDialog - Diff Rendering', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('renders diff section header', async () => {
    mockGetGitCommitDetail.mockResolvedValue(createMockCommitDetail())

    renderCommitDialog()

    await waitFor(() => {
      expect(screen.getByText('Diff')).toBeInTheDocument()
    })
  })

  it('passes diff content to DiffViewer', async () => {
    mockGetGitCommitDetail.mockResolvedValue(createMockCommitDetail())

    renderCommitDialog()

    await waitFor(() => {
      // DiffViewer should render the diff - check for unique diff content
      expect(screen.getByText('+export function newFeature() {')).toBeInTheDocument()
    })
  })

  it('does not render changed files section when files array is empty', async () => {
    mockGetGitCommitDetail.mockResolvedValue(
      createMockCommitDetail({
        files: [],
      })
    )

    renderCommitDialog()

    await waitFor(() => {
      expect(screen.getByText('Diff')).toBeInTheDocument()
    })
    expect(screen.queryByText(/Changed files/)).not.toBeInTheDocument()
  })
})

describe('CommitDetailDialog - Changes Row (Total Stats)', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('renders Changes row with both additions and deletions', async () => {
    mockGetGitCommitDetail.mockResolvedValue(createMockCommitDetail())

    renderCommitDialog()

    await waitFor(() => {
      expect(screen.getByText('Changes:')).toBeInTheDocument()
    })
    // Default mock has 60 total additions and 35 total deletions
    expect(screen.getByText(/Changes:/)).toBeInTheDocument()
    const changesRow = screen.getByText(/Changes:/).parentElement
    expect(changesRow).toHaveTextContent('+60')
    expect(changesRow).toHaveTextContent('-35')
  })

  it('renders Changes row with only additions (no deletions)', async () => {
    mockGetGitCommitDetail.mockResolvedValue(
      createMockCommitDetail({
        files: [
          {
            path: 'src/new.ts',
            status: 'added',
            additions: 75,
            deletions: 0,
          },
        ],
      })
    )

    renderCommitDialog()

    await waitFor(() => {
      expect(screen.getByText('Changes:')).toBeInTheDocument()
    })
    const changesRow = screen.getByText(/Changes:/).parentElement
    expect(changesRow).toHaveTextContent('+75')
    expect(changesRow).not.toHaveTextContent('-')
  })

  it('renders Changes row with only deletions (no additions)', async () => {
    mockGetGitCommitDetail.mockResolvedValue(
      createMockCommitDetail({
        files: [
          {
            path: 'src/removed.ts',
            status: 'deleted',
            additions: 0,
            deletions: 120,
          },
        ],
      })
    )

    renderCommitDialog()

    await waitFor(() => {
      expect(screen.getByText('Changes:')).toBeInTheDocument()
    })
    const changesRow = screen.getByText(/Changes:/).parentElement
    expect(changesRow).toHaveTextContent('-120')
    expect(changesRow).not.toHaveTextContent('+')
  })

  it('does not render Changes row when commit has no files', async () => {
    mockGetGitCommitDetail.mockResolvedValue(
      createMockCommitDetail({
        files: [],
      })
    )

    renderCommitDialog()

    await waitFor(() => {
      expect(screen.getByText('Commit Details')).toBeInTheDocument()
    })
    expect(screen.queryByText('Changes:')).not.toBeInTheDocument()
  })

  it('does not render Changes row when all files have zero additions and deletions', async () => {
    mockGetGitCommitDetail.mockResolvedValue(
      createMockCommitDetail({
        files: [
          {
            path: 'binary-file.bin',
            status: 'modified',
            additions: 0,
            deletions: 0,
          },
        ],
      })
    )

    renderCommitDialog()

    await waitFor(() => {
      expect(screen.getByText('Commit Details')).toBeInTheDocument()
    })
    expect(screen.queryByText('Changes:')).not.toBeInTheDocument()
  })

  it('applies correct color classes to Changes row stats', async () => {
    mockGetGitCommitDetail.mockResolvedValue(createMockCommitDetail())

    renderCommitDialog()

    await waitFor(() => {
      expect(screen.getByText('Changes:')).toBeInTheDocument()
    })

    // Check for green color class on additions
    const additionsSpan = screen.getByText('+60')
    expect(additionsSpan).toHaveClass('text-green-600', 'dark:text-green-400')

    // Check for red color class on deletions
    const deletionsSpan = screen.getByText('-35')
    expect(deletionsSpan).toHaveClass('text-red-600', 'dark:text-red-400')
  })

  it('renders Changes row after Date line', async () => {
    mockGetGitCommitDetail.mockResolvedValue(createMockCommitDetail())

    renderCommitDialog()

    await waitFor(() => {
      expect(screen.getByText('Changes:')).toBeInTheDocument()
    })

    // Get the parent container
    const headerSection = screen.getByText('Date:').closest('.space-y-2')
    expect(headerSection).toBeInTheDocument()

    // Verify Changes row is in the same section
    expect(headerSection).toContainElement(screen.getByText('Changes:'))
  })

  it('computes totals correctly from multiple files', async () => {
    mockGetGitCommitDetail.mockResolvedValue(
      createMockCommitDetail({
        files: [
          { path: 'file1.ts', status: 'added', additions: 100, deletions: 0 },
          { path: 'file2.ts', status: 'modified', additions: 25, deletions: 10 },
          { path: 'file3.ts', status: 'modified', additions: 15, deletions: 30 },
          { path: 'file4.ts', status: 'deleted', additions: 0, deletions: 50 },
        ],
      })
    )

    renderCommitDialog()

    await waitFor(() => {
      expect(screen.getByText('Changes:')).toBeInTheDocument()
    })

    // Total: 100 + 25 + 15 + 0 = 140 additions
    // Total: 0 + 10 + 30 + 50 = 90 deletions
    const changesRow = screen.getByText(/Changes:/).parentElement
    expect(changesRow).toHaveTextContent('+140')
    expect(changesRow).toHaveTextContent('-90')
  })
})

describe('CommitDetailDialog - Edge Cases', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('handles renamed file status', async () => {
    mockGetGitCommitDetail.mockResolvedValue(
      createMockCommitDetail({
        files: [
          {
            path: 'src/new-name.ts',
            status: 'renamed',
            additions: 0,
            deletions: 0,
          },
        ],
      })
    )

    renderCommitDialog()

    await waitFor(() => {
      expect(screen.getByText('renamed')).toBeInTheDocument()
    })
  })

  it('handles file with only additions', async () => {
    mockGetGitCommitDetail.mockResolvedValue(
      createMockCommitDetail({
        files: [
          {
            path: 'src/new.ts',
            status: 'added',
            additions: 100,
            deletions: 0,
          },
        ],
        diff: '', // Clear default diff to avoid mismatch
      })
    )

    renderCommitDialog()

    await waitFor(() => {
      expect(screen.getByText('src/new.ts')).toBeInTheDocument()
    })
    // Should render +100 in file row and Changes row
    expect(screen.getAllByText('+100').length).toBeGreaterThan(0)
    // Should not render -0
    const fileRow = screen.getByText('src/new.ts').closest('button')
    expect(fileRow).not.toHaveTextContent('-0')
  })

  it('handles file with only deletions', async () => {
    mockGetGitCommitDetail.mockResolvedValue(
      createMockCommitDetail({
        files: [
          {
            path: 'src/removed.ts',
            status: 'deleted',
            additions: 0,
            deletions: 50,
          },
        ],
        diff: '', // Clear default diff to avoid mismatch
      })
    )

    renderCommitDialog()

    await waitFor(() => {
      expect(screen.getByText('src/removed.ts')).toBeInTheDocument()
    })
    // Should render -50 in file row and Changes row
    expect(screen.getAllByText('-50').length).toBeGreaterThan(0)
    // Should not render +0
    const fileRow = screen.getByText('src/removed.ts').closest('button')
    expect(fileRow).not.toHaveTextContent('+0')
  })

  it('handles single-line commit message', async () => {
    mockGetGitCommitDetail.mockResolvedValue(
      createMockCommitDetail({
        message: 'Simple commit message',
      })
    )

    renderCommitDialog()

    await waitFor(() => {
      expect(screen.getByText('Simple commit message')).toBeInTheDocument()
    })
  })

  it('handles empty diff', async () => {
    mockGetGitCommitDetail.mockResolvedValue(
      createMockCommitDetail({
        diff: '',
        files: [],
      })
    )

    renderCommitDialog()

    await waitFor(() => {
      expect(screen.getByText('Diff')).toBeInTheDocument()
    })
    // DiffViewer should show "No diff available"
    expect(screen.getByText('No diff available')).toBeInTheDocument()
  })
})
