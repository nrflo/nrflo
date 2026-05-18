import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, within } from '@testing-library/react'
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

// Stub PythonScriptForm — avoids CodeMirror/validation complexity at page level
vi.mock('@/components/pythonScripts/PythonScriptForm', () => ({
  PythonScriptForm: ({ onSubmit, onValidationFailure, onCancel, initial }: {
    onSubmit: (d: object) => void
    onValidationFailure: (r: object, d: object) => void
    onCancel: () => void
    initial?: { name: string }
  }) => (
    <div data-testid="python-script-form">
      {initial?.name && <span data-testid="form-initial-name">{initial.name}</span>}
      <button onClick={() => onSubmit({ name: 'new-script', code: 'print()', description: '' })}>
        Submit Form
      </button>
      <button onClick={() =>
        onValidationFailure(
          { ok: false, error: 'invalid syntax', line: 3, col: 5 },
          { name: 'new-script', code: 'bad', description: '' }
        )
      }>
        Trigger Error
      </button>
      <button onClick={onCancel}>Cancel Form</button>
    </div>
  ),
}))

// Stub PythonToolForm
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
    id: 'script-1',
    project_id: 'proj-1',
    kind: 'agent',
    name: 'data-processor',
    description: 'Processes data',
    code: 'print("hello")',
    file_path: '',
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-15T00:00:00Z',
    ...overrides,
  }
}

function renderPage(initialUrl = '/python-scripts') {
  return render(
    <MemoryRouter initialEntries={[initialUrl]}>
      <PythonScriptsPage />
    </MemoryRouter>
  )
}

function setupMocks(
  scripts: PythonScript[] = [],
  opts: { isLoading?: boolean; error?: Error | null } = {}
) {
  vi.mocked(usePythonScripts).mockReturnValue({
    data: scripts,
    isLoading: opts.isLoading ?? false,
    error: opts.error ?? null,
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

beforeEach(() => {
  vi.clearAllMocks()
  mockUseIsAdmin.mockReturnValue(true)
})

describe('PythonScriptsPage — list rendering', () => {
  it('shows loading state', () => {
    setupMocks([], { isLoading: true })
    renderPage()
    expect(screen.getByText('Loading…')).toBeInTheDocument()
  })

  it('shows empty state when no scripts', () => {
    setupMocks([])
    renderPage()
    expect(screen.getByText('No agent scripts yet.')).toBeInTheDocument()
  })

  it('renders script name and description', () => {
    setupMocks([makeScript()])
    renderPage()
    expect(screen.getByText('data-processor')).toBeInTheDocument()
    expect(screen.getByText('Processes data')).toBeInTheDocument()
  })

  it('renders multiple scripts', () => {
    setupMocks([
      makeScript({ id: 's1', name: 'script-alpha' }),
      makeScript({ id: 's2', name: 'script-beta' }),
    ])
    renderPage()
    expect(screen.getByText('script-alpha')).toBeInTheDocument()
    expect(screen.getByText('script-beta')).toBeInTheDocument()
  })
})

describe('PythonScriptsPage — create flow', () => {
  it('opens create dialog on New Script click', async () => {
    const user = userEvent.setup()
    setupMocks([])
    renderPage()
    await user.click(screen.getByRole('button', { name: /new script/i }))
    expect(screen.getByTestId('python-script-form')).toBeInTheDocument()
    expect(screen.getByRole('heading', { name: /new agent script/i })).toBeInTheDocument()
  })

  it('calls createMutation.mutate on form submit', async () => {
    const user = userEvent.setup()
    setupMocks([])
    const mutate = vi.fn()
    vi.mocked(useCreatePythonScript).mockReturnValue({ mutate, isPending: false } as unknown as ReturnType<typeof useCreatePythonScript>)
    renderPage()
    await user.click(screen.getByRole('button', { name: /new script/i }))
    await user.click(screen.getByRole('button', { name: 'Submit Form' }))
    expect(mutate).toHaveBeenCalledWith(
      expect.objectContaining({ name: 'new-script' }),
      expect.any(Object)
    )
  })

  it('closes dialog when Cancel Form is clicked', async () => {
    const user = userEvent.setup()
    setupMocks([])
    renderPage()
    await user.click(screen.getByRole('button', { name: /new script/i }))
    await user.click(screen.getByRole('button', { name: 'Cancel Form' }))
    expect(screen.queryByTestId('python-script-form')).not.toBeInTheDocument()
  })
})

describe('PythonScriptsPage — edit flow', () => {
  it('opens edit dialog with initial script data', async () => {
    const user = userEvent.setup()
    setupMocks([makeScript({ name: 'my-script' })])
    renderPage()

    const row = screen.getByText('my-script').closest('div.border')!
    const [editBtn] = within(row).getAllByRole('button')
    await user.click(editBtn)

    expect(screen.getByRole('heading', { name: /edit agent script/i })).toBeInTheDocument()
    expect(screen.getByTestId('form-initial-name')).toHaveTextContent('my-script')
  })

  it('calls updateMutation.mutate on form submit', async () => {
    const user = userEvent.setup()
    setupMocks([makeScript()])
    const mutate = vi.fn()
    vi.mocked(useUpdatePythonScript).mockReturnValue({ mutate, isPending: false } as unknown as ReturnType<typeof useUpdatePythonScript>)
    renderPage()

    const row = screen.getByText('data-processor').closest('div.border')!
    const [editBtn] = within(row).getAllByRole('button')
    await user.click(editBtn)
    await user.click(screen.getByRole('button', { name: 'Submit Form' }))

    expect(mutate).toHaveBeenCalledWith(
      expect.objectContaining({ id: 'script-1' }),
      expect.any(Object)
    )
  })
})

describe('PythonScriptsPage — delete flow', () => {
  it('opens delete confirm dialog with script name in message', async () => {
    const user = userEvent.setup()
    setupMocks([makeScript({ name: 'to-delete' })])
    renderPage()

    const row = screen.getByText('to-delete').closest('div.border')!
    const buttons = within(row).getAllByRole('button')
    await user.click(buttons[1]) // second button = delete

    expect(screen.getByRole('heading', { name: /delete agent script/i })).toBeInTheDocument()
    expect(screen.getByText(/Agent definitions referencing/)).toBeInTheDocument()
  })

  it('calls deleteMutation.mutate on confirm', async () => {
    const user = userEvent.setup()
    setupMocks([makeScript({ id: 'script-99' })])
    const mutate = vi.fn()
    vi.mocked(useDeletePythonScript).mockReturnValue({ mutate, isPending: false } as unknown as ReturnType<typeof useDeletePythonScript>)
    renderPage()

    const row = screen.getByText('data-processor').closest('div.border')!
    const buttons = within(row).getAllByRole('button')
    await user.click(buttons[1])

    const dialog = screen.getByRole('heading', { name: /delete agent script/i }).closest('[role="dialog"]') ?? document.body
    await user.click(within(dialog as HTMLElement).getByRole('button', { name: /delete/i }))

    expect(mutate).toHaveBeenCalledWith('script-99', expect.any(Object))
  })

  it('closes delete dialog on Cancel', async () => {
    const user = userEvent.setup()
    setupMocks([makeScript()])
    renderPage()

    const row = screen.getByText('data-processor').closest('div.border')!
    await user.click(within(row).getAllByRole('button')[1])
    expect(screen.getByRole('heading', { name: /delete agent script/i })).toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: 'Cancel' }))
    expect(screen.queryByRole('heading', { name: /delete agent script/i })).not.toBeInTheDocument()
  })
})

describe('PythonScriptsPage — save anyway flow', () => {
  it('opens ConfirmDialog with error details on validation failure', async () => {
    const user = userEvent.setup()
    setupMocks([])
    renderPage()
    await user.click(screen.getByRole('button', { name: /new script/i }))
    await user.click(screen.getByRole('button', { name: 'Trigger Error' }))
    expect(screen.getByRole('heading', { name: /validation error/i })).toBeInTheDocument()
    expect(screen.getByText(/Line 3, col 5: invalid syntax/)).toBeInTheDocument()
  })

  it('calls createMutation.mutate when Save anyway is clicked', async () => {
    const user = userEvent.setup()
    setupMocks([])
    const mutate = vi.fn()
    vi.mocked(useCreatePythonScript).mockReturnValue({ mutate, isPending: false } as unknown as ReturnType<typeof useCreatePythonScript>)
    renderPage()
    await user.click(screen.getByRole('button', { name: /new script/i }))
    await user.click(screen.getByRole('button', { name: 'Trigger Error' }))
    await user.click(screen.getByRole('button', { name: /save anyway/i }))
    expect(mutate).toHaveBeenCalledWith(
      expect.objectContaining({ name: 'new-script' }),
      expect.any(Object)
    )
  })

  it('does not call mutation when Cancel is clicked in ConfirmDialog', async () => {
    const user = userEvent.setup()
    setupMocks([])
    const mutate = vi.fn()
    vi.mocked(useCreatePythonScript).mockReturnValue({ mutate, isPending: false } as unknown as ReturnType<typeof useCreatePythonScript>)
    renderPage()
    await user.click(screen.getByRole('button', { name: /new script/i }))
    await user.click(screen.getByRole('button', { name: 'Trigger Error' }))
    await user.click(screen.getByRole('button', { name: 'Cancel' }))
    expect(mutate).not.toHaveBeenCalled()
  })
})

describe('PythonScriptsPage — admin/viewer roles', () => {
  it('hides New Script button for non-admin', () => {
    mockUseIsAdmin.mockReturnValue(false)
    setupMocks([])
    renderPage()
    expect(screen.queryByRole('button', { name: /new script/i })).not.toBeInTheDocument()
  })

  it('shows ReadOnlyHint for non-admin', () => {
    mockUseIsAdmin.mockReturnValue(false)
    setupMocks([])
    renderPage()
    expect(screen.getByText(/read.only/i)).toBeInTheDocument()
  })

  it('hides edit and delete buttons for non-admin', () => {
    mockUseIsAdmin.mockReturnValue(false)
    setupMocks([makeScript()])
    renderPage()
    expect(screen.getByText('data-processor')).toBeInTheDocument()
    const row = screen.getByText('data-processor').closest('div.border')!
    expect(within(row).queryAllByRole('button')).toHaveLength(0)
  })

  it('shows edit and delete buttons for admin', () => {
    setupMocks([makeScript()])
    renderPage()
    const row = screen.getByText('data-processor').closest('div.border')!
    expect(within(row).getAllByRole('button')).toHaveLength(2)
  })
})

