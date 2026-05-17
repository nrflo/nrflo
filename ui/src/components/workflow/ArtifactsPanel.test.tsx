import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { within } from '@testing-library/react'
import { ArtifactsPanel } from './ArtifactsPanel'
import type { Artifact } from '@/types/artifact'

const mockMutate = vi.fn()

vi.mock('@/hooks/useArtifacts', () => ({
  useArtifacts: vi.fn(),
  useDeleteArtifact: vi.fn(() => ({ mutate: mockMutate, isPending: false })),
}))

vi.mock('@/api/artifacts', () => ({
  downloadArtifactURL: (id: string) => `/api/v1/artifacts/${id}/download`,
}))

import { useArtifacts } from '@/hooks/useArtifacts'
const mockUseArtifacts = useArtifacts as ReturnType<typeof vi.fn>

function makeArtifact(overrides: Partial<Artifact> = {}): Artifact {
  return {
    id: 'art-1',
    project_id: 'proj',
    workflow_instance_id: 'wfi-1',
    name: 'report.txt',
    type: 'text',
    size_bytes: 1024,
    source: 'input',
    created_at: '2026-01-01T00:00:00Z',
    ...overrides,
  }
}

describe('ArtifactsPanel', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockUseArtifacts.mockReturnValue({ data: [] })
  })

  describe('empty state', () => {
    it('shows empty message when no artifacts', () => {
      render(<ArtifactsPanel workflowInstanceId="wfi-1" />)
      expect(screen.getByText('No artifacts available')).toBeInTheDocument()
    })

    it('shows empty message when no input artifacts and no sessionId', () => {
      mockUseArtifacts.mockReturnValue({
        data: [makeArtifact({ source: 'agent', created_by_session: 'sess-1' })],
      })
      render(<ArtifactsPanel workflowInstanceId="wfi-1" />)
      expect(screen.getByText('No artifacts available')).toBeInTheDocument()
    })

    it('shows empty message when session filter matches no artifacts', () => {
      mockUseArtifacts.mockReturnValue({
        data: [makeArtifact({ source: 'agent', created_by_session: 'other-sess' })],
      })
      render(<ArtifactsPanel workflowInstanceId="wfi-1" sessionId="my-sess" />)
      expect(screen.getByText('No artifacts available')).toBeInTheDocument()
    })
  })

  describe('populated state', () => {
    it('shows input artifact name and size', () => {
      mockUseArtifacts.mockReturnValue({
        data: [makeArtifact({ name: 'plan.md', size_bytes: 2048 })],
      })
      render(<ArtifactsPanel workflowInstanceId="wfi-1" />)
      expect(screen.getByText('plan.md')).toBeInTheDocument()
      expect(screen.getByText('2.0 KB')).toBeInTheDocument()
    })

    it('shows input artifacts without sessionId filter', () => {
      mockUseArtifacts.mockReturnValue({
        data: [
          makeArtifact({ id: 'a1', name: 'in.txt', source: 'input' }),
          makeArtifact({ id: 'a2', name: 'out.txt', source: 'agent', created_by_session: 's1' }),
        ],
      })
      render(<ArtifactsPanel workflowInstanceId="wfi-1" />)
      expect(screen.getByText('in.txt')).toBeInTheDocument()
      expect(screen.queryByText('out.txt')).not.toBeInTheDocument()
    })

    it('shows input + matching session artifacts when sessionId provided', () => {
      mockUseArtifacts.mockReturnValue({
        data: [
          makeArtifact({ id: 'a1', name: 'in.txt', source: 'input' }),
          makeArtifact({ id: 'a2', name: 'mine.json', source: 'agent', created_by_session: 'sess-1' }),
          makeArtifact({ id: 'a3', name: 'other.txt', source: 'agent', created_by_session: 'sess-2' }),
        ],
      })
      render(<ArtifactsPanel workflowInstanceId="wfi-1" sessionId="sess-1" />)
      expect(screen.getByText('in.txt')).toBeInTheDocument()
      expect(screen.getByText('mine.json')).toBeInTheDocument()
      expect(screen.queryByText('other.txt')).not.toBeInTheDocument()
    })
  })

  describe('download link', () => {
    it('href equals downloadArtifactURL(id)', () => {
      mockUseArtifacts.mockReturnValue({
        data: [makeArtifact({ id: 'art-abc', name: 'file.txt' })],
      })
      render(<ArtifactsPanel workflowInstanceId="wfi-1" />)
      const link = screen.getByRole('link')
      expect(link).toHaveAttribute('href', '/api/v1/artifacts/art-abc/download')
    })
  })

  describe('delete', () => {
    it('clicking trash button opens ConfirmDialog', async () => {
      const user = userEvent.setup()
      mockUseArtifacts.mockReturnValue({
        data: [makeArtifact({ id: 'art-1', name: 'file.txt' })],
      })
      render(<ArtifactsPanel workflowInstanceId="wfi-1" />)

      const buttons = screen.getAllByRole('button')
      await user.click(buttons[buttons.length - 1])

      expect(screen.getByText('Delete Artifact')).toBeInTheDocument()
      expect(screen.getByText(/Delete "file\.txt"\?/)).toBeInTheDocument()
    })

    it('confirming delete calls mutation with artifact id and workflowInstanceId', async () => {
      const user = userEvent.setup()
      mockUseArtifacts.mockReturnValue({
        data: [makeArtifact({ id: 'art-1', name: 'file.txt' })],
      })
      render(<ArtifactsPanel workflowInstanceId="wfi-1" />)

      const buttons = screen.getAllByRole('button')
      await user.click(buttons[buttons.length - 1])

      const overlay = document.querySelector<HTMLElement>('.fixed.inset-0')!
      await user.click(within(overlay).getByRole('button', { name: 'Delete' }))

      expect(mockMutate).toHaveBeenCalledWith({ id: 'art-1', workflowInstanceId: 'wfi-1' })
    })

    it('cancelling closes dialog without calling mutation', async () => {
      const user = userEvent.setup()
      mockUseArtifacts.mockReturnValue({
        data: [makeArtifact({ id: 'art-1', name: 'file.txt' })],
      })
      render(<ArtifactsPanel workflowInstanceId="wfi-1" />)

      const buttons = screen.getAllByRole('button')
      await user.click(buttons[buttons.length - 1])
      expect(screen.getByText('Delete Artifact')).toBeInTheDocument()

      await user.click(screen.getByRole('button', { name: 'Cancel' }))
      expect(screen.queryByText('Delete Artifact')).not.toBeInTheDocument()
      expect(mockMutate).not.toHaveBeenCalled()
    })
  })
})
