import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { ConfigPage } from './Config'
import type { NrvappConfigFileMeta } from '@/types/nrvapp'
import { ApiError } from '@/api/client'

vi.mock('@/hooks/useNrvapp', () => ({
  useConfigFiles: vi.fn(),
}))

vi.mock('@/components/nrvapp/ConfigFileList', () => ({
  ConfigFileList: ({ files }: { files: NrvappConfigFileMeta[] }) => (
    <div data-testid="config-file-list">
      {files.map((f) => (
        <div key={f.path} data-testid="config-file-item">
          {f.path}
        </div>
      ))}
    </div>
  ),
}))

import { useConfigFiles } from '@/hooks/useNrvapp'

function makeFileMeta(overrides: Partial<NrvappConfigFileMeta> = {}): NrvappConfigFileMeta {
  return {
    path: 'customer/config.yaml',
    latest_version: 1,
    has_schema: false,
    updated_at: '2026-01-01T00:00:00Z',
    ...overrides,
  }
}

function setupMocks(data: NrvappConfigFileMeta[] = [], extra = {}) {
  vi.mocked(useConfigFiles).mockReturnValue({
    data,
    isLoading: false,
    error: null,
    ...extra,
  } as unknown as ReturnType<typeof useConfigFiles>)
}

function renderPage() {
  return render(
    <MemoryRouter>
      <ConfigPage />
    </MemoryRouter>
  )
}

beforeEach(() => vi.clearAllMocks())

describe('ConfigPage', () => {
  it('shows loading state', () => {
    setupMocks([], { isLoading: true, data: undefined })
    renderPage()
    expect(screen.getByText('Loading…')).toBeInTheDocument()
  })

  it('shows empty state when no files', () => {
    setupMocks([])
    renderPage()
    expect(screen.getByText('No config files found.')).toBeInTheDocument()
  })

  it('renders ConfigFileList when files present', () => {
    setupMocks([
      makeFileMeta({ path: 'dir/a.yaml' }),
      makeFileMeta({ path: 'dir/b.yaml' }),
    ])
    renderPage()
    expect(screen.getByTestId('config-file-list')).toBeInTheDocument()
    expect(screen.getAllByTestId('config-file-item')).toHaveLength(2)
  })

  it('shows ApiError 400 state with settings link when customer_config_dir not configured', () => {
    const err = new ApiError(400, 'customer_config_dir not set')
    setupMocks([], { error: err, data: undefined })
    renderPage()
    expect(screen.getByText('Customer config directory is not configured.')).toBeInTheDocument()
    expect(screen.getByRole('link', { name: 'Go to Settings' })).toBeInTheDocument()
  })

  it('shows generic error message for non-400 errors', () => {
    const err = new ApiError(500, 'Internal server error')
    setupMocks([], { error: err, data: undefined })
    renderPage()
    expect(screen.getByText('Internal server error')).toBeInTheDocument()
    expect(screen.queryByText('Customer config directory is not configured.')).not.toBeInTheDocument()
  })
})
