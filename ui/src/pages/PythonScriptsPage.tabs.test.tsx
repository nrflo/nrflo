import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router-dom'
import { PythonScriptsPage } from './PythonScriptsPage'
import type { PythonScript } from '@/types/pythonScript'

const mockUseIsAdmin = vi.fn().mockReturnValue(true)
vi.mock('@/stores/authStore', () => ({
  useIsAdmin: () => mockUseIsAdmin(),
}))

vi.mock('@/hooks/usePythonScripts', () => ({
  usePythonScripts: vi.fn(),
  useCreatePythonScript: vi.fn(),
  useUpdatePythonScript: vi.fn(),
  useDeletePythonScript: vi.fn(),
}))

vi.mock('@/components/pythonScripts/PythonScriptForm', () => ({
  PythonScriptForm: ({ onSubmit, onValidationFailure, onCancel }: {
    onSubmit: (d: object) => void
    onValidationFailure: (r: object, d: object) => void
    onCancel: () => void
  }) => (
    <div data-testid="python-script-form">
      <button onClick={() => onSubmit({ name: 'agent-script', code: 'pass', description: '' })}>
        Submit Script Form
      </button>
      <button onClick={() => onValidationFailure({ ok: false, error: 'e' }, { name: 'x', code: 'bad', description: '' })}>
        Trigger Script Error
      </button>
      <button onClick={onCancel}>Cancel Script Form</button>
    </div>
  ),
}))

vi.mock('@/components/pythonScripts/PythonToolForm', () => ({
  PythonToolForm: ({ onSubmit, onCancel }: {
    onSubmit: (d: object) => void
    onCancel: () => void
  }) => (
    <div data-testid="python-tool-form">
      <button onClick={() => onSubmit({ kind: 'tool', name: 'my-tool', tool_description: 'desc', input_schema: '{}', timeout_sec: 30, code: 'pass' })}>
        Submit Tool Form
      </button>
      <button onClick={onCancel}>Cancel Tool Form</button>
    </div>
  ),
}))

import {
  usePythonScripts,
  useCreatePythonScript,
  useUpdatePythonScript,
  useDeletePythonScript,
} from '@/hooks/usePythonScripts'

function makeScript(overrides: Partial<PythonScript> = {}): PythonScript {
  return {
    id: 'script-1', project_id: 'proj-1', kind: 'agent',
    name: 'data-processor', description: 'Processes data',
    code: 'print("hello")', file_path: '',
    created_at: '2026-01-01T00:00:00Z', updated_at: '2026-01-15T00:00:00Z',
    ...overrides,
  }
}

function setupMocks(scripts: PythonScript[] = []) {
  vi.mocked(usePythonScripts).mockReturnValue({
    data: scripts, isLoading: false, error: null,
  } as ReturnType<typeof usePythonScripts>)
  vi.mocked(useCreatePythonScript).mockReturnValue({
    mutate: vi.fn(), isPending: false,
  } as unknown as ReturnType<typeof useCreatePythonScript>)
  vi.mocked(useUpdatePythonScript).mockReturnValue({
    mutate: vi.fn(), isPending: false,
  } as unknown as ReturnType<typeof useUpdatePythonScript>)
  vi.mocked(useDeletePythonScript).mockReturnValue({
    mutate: vi.fn(), isPending: false,
  } as unknown as ReturnType<typeof useDeletePythonScript>)
}

function renderPage(initialUrl = '/python-scripts') {
  return render(
    <MemoryRouter initialEntries={[initialUrl]}>
      <PythonScriptsPage />
    </MemoryRouter>
  )
}

beforeEach(() => {
  vi.clearAllMocks()
  mockUseIsAdmin.mockReturnValue(true)
})

describe('PythonScriptsPage — tab switching', () => {
  it('default route calls usePythonScripts with "agent"', () => {
    setupMocks([])
    renderPage()
    expect(vi.mocked(usePythonScripts)).toHaveBeenCalledWith('agent')
  })

  it('default route shows Agents tab with correct empty state', () => {
    setupMocks([])
    renderPage()
    expect(screen.getByText('No agent scripts yet.')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /new script/i })).toBeInTheDocument()
  })

  it('clicking Tools tab calls usePythonScripts with "tool"', async () => {
    const user = userEvent.setup()
    setupMocks([])
    renderPage()
    await user.click(screen.getByRole('button', { name: 'Tools' }))
    expect(vi.mocked(usePythonScripts)).toHaveBeenLastCalledWith('tool')
  })

  it('clicking Tools tab shows correct empty state and New Tool button', async () => {
    const user = userEvent.setup()
    setupMocks([])
    renderPage()
    await user.click(screen.getByRole('button', { name: 'Tools' }))
    expect(screen.getByText('No Python tools yet.')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /new tool/i })).toBeInTheDocument()
  })

  it('deep-link ?kind=tool calls usePythonScripts with "tool"', () => {
    setupMocks([])
    renderPage('/python-scripts?kind=tool')
    expect(vi.mocked(usePythonScripts)).toHaveBeenCalledWith('tool')
  })

  it('deep-link ?kind=tool renders tool rows and correct empty label', () => {
    setupMocks([makeScript({ kind: 'tool', name: 'my-tool' })])
    renderPage('/python-scripts?kind=tool')
    expect(screen.getByText('my-tool')).toBeInTheDocument()
    expect(screen.queryByText('No Python tools yet.')).not.toBeInTheDocument()
  })

  it('clicking New on Tools tab mounts PythonToolForm not PythonScriptForm', async () => {
    const user = userEvent.setup()
    setupMocks([])
    renderPage('/python-scripts?kind=tool')
    await user.click(screen.getByRole('button', { name: /new tool/i }))
    expect(screen.getByTestId('python-tool-form')).toBeInTheDocument()
    expect(screen.queryByTestId('python-script-form')).not.toBeInTheDocument()
  })

  it('clicking New on Agents tab mounts PythonScriptForm not PythonToolForm', async () => {
    const user = userEvent.setup()
    setupMocks([])
    renderPage()
    await user.click(screen.getByRole('button', { name: /new script/i }))
    expect(screen.getByTestId('python-script-form')).toBeInTheDocument()
    expect(screen.queryByTestId('python-tool-form')).not.toBeInTheDocument()
  })

  it('switching from Agents to Tools tab closes any open form', async () => {
    const user = userEvent.setup()
    setupMocks([])
    renderPage()
    // Open agent form
    await user.click(screen.getByRole('button', { name: /new script/i }))
    expect(screen.getByTestId('python-script-form')).toBeInTheDocument()
    // Switch to Tools tab — setKind closes the form
    await user.click(screen.getByRole('button', { name: 'Tools' }))
    expect(screen.queryByTestId('python-script-form')).not.toBeInTheDocument()
  })
})
