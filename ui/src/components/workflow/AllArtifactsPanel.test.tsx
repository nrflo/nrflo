import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import { AllArtifactsPanel } from './AllArtifactsPanel'
import type { Artifact } from '@/types/artifact'

vi.mock('@/hooks/useArtifacts', () => ({
  useArtifacts: vi.fn(),
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
    size_bytes: 512,
    source: 'input',
    created_at: '2026-01-01T00:00:00Z',
    ...overrides,
  }
}

describe('AllArtifactsPanel', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockUseArtifacts.mockReturnValue({ data: [] })
  })

  it('shows empty message when no artifacts', () => {
    render(<AllArtifactsPanel workflowInstanceId="wfi-1" />)
    expect(screen.getByText('No artifacts available')).toBeInTheDocument()
  })

  describe('input artifacts section', () => {
    it('renders "Input Artifacts" heading for input source', () => {
      mockUseArtifacts.mockReturnValue({
        data: [makeArtifact({ id: 'a1', name: 'spec.md', source: 'input' })],
      })
      render(<AllArtifactsPanel workflowInstanceId="wfi-1" />)
      expect(screen.getByText('Input Artifacts')).toBeInTheDocument()
      expect(screen.getByText('spec.md')).toBeInTheDocument()
    })

    it('download link has correct href', () => {
      mockUseArtifacts.mockReturnValue({
        data: [makeArtifact({ id: 'art-xyz', source: 'input' })],
      })
      render(<AllArtifactsPanel workflowInstanceId="wfi-1" />)
      const link = screen.getByRole('link')
      expect(link).toHaveAttribute('href', '/api/v1/artifacts/art-xyz/download')
    })

    it('does not render Input Artifacts section when no input artifacts', () => {
      mockUseArtifacts.mockReturnValue({
        data: [makeArtifact({ source: 'agent', created_by_session: 'sess-1' })],
      })
      render(<AllArtifactsPanel workflowInstanceId="wfi-1" />)
      expect(screen.queryByText('Input Artifacts')).not.toBeInTheDocument()
    })
  })

  describe('agent artifacts grouping', () => {
    it('groups agent artifacts by session under "Agent: <sessionId>"', () => {
      mockUseArtifacts.mockReturnValue({
        data: [
          makeArtifact({ id: 'a1', name: 'out1.txt', source: 'agent', created_by_session: 'sess-abc' }),
          makeArtifact({ id: 'a2', name: 'out2.txt', source: 'agent', created_by_session: 'sess-xyz' }),
        ],
      })
      render(<AllArtifactsPanel workflowInstanceId="wfi-1" />)
      expect(screen.getByText('sess-abc')).toBeInTheDocument()
      expect(screen.getByText('sess-xyz')).toBeInTheDocument()
      expect(screen.getByText('out1.txt')).toBeInTheDocument()
      expect(screen.getByText('out2.txt')).toBeInTheDocument()
    })

    it('multiple artifacts from same session appear under one heading', () => {
      mockUseArtifacts.mockReturnValue({
        data: [
          makeArtifact({ id: 'a1', name: 'first.txt', source: 'agent', created_by_session: 'sess-1' }),
          makeArtifact({ id: 'a2', name: 'second.txt', source: 'agent', created_by_session: 'sess-1' }),
        ],
      })
      render(<AllArtifactsPanel workflowInstanceId="wfi-1" />)
      const headings = screen.getAllByText('sess-1')
      expect(headings).toHaveLength(1)
      expect(screen.getByText('first.txt')).toBeInTheDocument()
      expect(screen.getByText('second.txt')).toBeInTheDocument()
    })

    it('shows both input and agent sections together', () => {
      mockUseArtifacts.mockReturnValue({
        data: [
          makeArtifact({ id: 'a1', name: 'input.md', source: 'input' }),
          makeArtifact({ id: 'a2', name: 'result.json', source: 'agent', created_by_session: 'sess-1' }),
        ],
      })
      render(<AllArtifactsPanel workflowInstanceId="wfi-1" />)
      expect(screen.getByText('Input Artifacts')).toBeInTheDocument()
      expect(screen.getByText('sess-1')).toBeInTheDocument()
      expect(screen.getByText('input.md')).toBeInTheDocument()
      expect(screen.getByText('result.json')).toBeInTheDocument()
    })
  })
})
