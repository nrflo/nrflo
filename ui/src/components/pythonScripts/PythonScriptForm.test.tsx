import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { PythonScriptForm } from './PythonScriptForm'
import type { ValidationResult } from '@/types/pythonScript'

// Stub CodeMirror editor — jsdom cannot run it
vi.mock('@/components/ui/CodeEditor', () => ({
  CodeEditor: ({ value, onChange, placeholder, readOnly }: {
    value: string
    onChange: (v: string) => void
    placeholder?: string
    readOnly?: boolean
  }) => (
    <textarea
      value={value}
      onChange={(e) => !readOnly && onChange(e.target.value)}
      placeholder={placeholder}
      aria-label="Code editor"
      readOnly={readOnly}
    />
  ),
}))

vi.mock('@/hooks/usePythonScripts', () => ({
  useValidatePythonScript: vi.fn(),
  useReadPythonFile: vi.fn().mockReturnValue({
    data: undefined, isLoading: false, isFetching: false, isError: false, refetch: vi.fn(), error: null,
  }),
  useBrowsePythonDir: vi.fn().mockReturnValue({
    data: undefined, isLoading: false, isError: false, error: null,
  }),
}))

// Stub FilePickerModal so we can trigger onSelect without modal internals
vi.mock('./FilePickerModal', () => ({
  FilePickerModal: ({ open, onSelect }: {
    open: boolean
    onSelect: (p: string) => void
    onClose: () => void
  }) => {
    if (!open) return null
    return <button onClick={() => onSelect('/scripts/foo.py')}>Select foo.py</button>
  },
}))

import { useValidatePythonScript, useReadPythonFile } from '@/hooks/usePythonScripts'

function setupValidate(result: ValidationResult, isPending = false) {
  vi.mocked(useValidatePythonScript).mockReturnValue({
    mutateAsync: vi.fn().mockResolvedValue(result),
    mutate: vi.fn(),
    isPending,
  } as unknown as ReturnType<typeof useValidatePythonScript>)
}

function renderForm(props: Partial<React.ComponentProps<typeof PythonScriptForm>> = {}) {
  return render(
    <PythonScriptForm
      isCreate={true}
      onSubmit={vi.fn()}
      onValidationFailure={vi.fn()}
      onCancel={vi.fn()}
      {...props}
    />
  )
}

beforeEach(() => vi.clearAllMocks())

describe('PythonScriptForm — Check Syntax', () => {
  it('shows Syntax OK badge when code is valid', async () => {
    setupValidate({ ok: true })
    const user = userEvent.setup()
    renderForm()
    await user.click(screen.getByRole('button', { name: /check syntax/i }))
    expect(await screen.findByText(/syntax ok/i)).toBeInTheDocument()
  })

  it('shows line/col error inline when validation fails', async () => {
    setupValidate({ ok: false, error: 'unexpected EOF', line: 5, col: 10 })
    const user = userEvent.setup()
    renderForm()
    await user.click(screen.getByRole('button', { name: /check syntax/i }))
    expect(await screen.findByText('Line 5, col 10: unexpected EOF')).toBeInTheDocument()
  })

  it('shows error text without line/col when only error field is present', async () => {
    setupValidate({ ok: false, error: 'syntax error' })
    const user = userEvent.setup()
    renderForm()
    await user.click(screen.getByRole('button', { name: /check syntax/i }))
    expect(await screen.findByText('syntax error')).toBeInTheDocument()
  })

  it('clears previous result when re-checking', async () => {
    const mutateAsync = vi.fn()
      .mockResolvedValueOnce({ ok: true })
      .mockResolvedValueOnce({ ok: false, error: 'oops' })
    vi.mocked(useValidatePythonScript).mockReturnValue({
      mutateAsync, mutate: vi.fn(), isPending: false,
    } as unknown as ReturnType<typeof useValidatePythonScript>)

    const user = userEvent.setup()
    renderForm()
    await user.click(screen.getByRole('button', { name: /check syntax/i }))
    await screen.findByText(/syntax ok/i)

    await user.click(screen.getByRole('button', { name: /check syntax/i }))
    expect(await screen.findByText('oops')).toBeInTheDocument()
    expect(screen.queryByText(/syntax ok/i)).not.toBeInTheDocument()
  })
})

describe('PythonScriptForm — Submit', () => {
  it('calls onSubmit when code is valid', async () => {
    setupValidate({ ok: true })
    const onSubmit = vi.fn()
    const user = userEvent.setup()
    renderForm({ onSubmit })

    await user.type(screen.getByPlaceholderText(/e\.g\., data-processor/), 'my-script')
    await user.click(screen.getByRole('button', { name: /create/i }))

    await screen.findByText(/syntax ok/i)
    expect(onSubmit).toHaveBeenCalledWith(expect.objectContaining({ name: 'my-script' }))
  })

  it('calls onValidationFailure (not onSubmit) when code is invalid', async () => {
    setupValidate({ ok: false, error: 'bad syntax', line: 1, col: 1 })
    const onSubmit = vi.fn()
    const onValidationFailure = vi.fn()
    const user = userEvent.setup()
    renderForm({ onSubmit, onValidationFailure })

    await user.type(screen.getByPlaceholderText(/e\.g\., data-processor/), 'my-script')
    await user.click(screen.getByRole('button', { name: /create/i }))

    await screen.findByText('Line 1, col 1: bad syntax')
    expect(onValidationFailure).toHaveBeenCalledWith(
      expect.objectContaining({ ok: false, line: 1 }),
      expect.objectContaining({ name: 'my-script' })
    )
    expect(onSubmit).not.toHaveBeenCalled()
  })

  it('does not call mutateAsync when name is empty', async () => {
    const mutateAsync = vi.fn().mockResolvedValue({ ok: true })
    vi.mocked(useValidatePythonScript).mockReturnValue({
      mutateAsync, mutate: vi.fn(), isPending: false,
    } as unknown as ReturnType<typeof useValidatePythonScript>)

    const user = userEvent.setup()
    renderForm()
    // Submit without filling in name
    await user.click(screen.getByRole('button', { name: /create/i }))
    expect(mutateAsync).not.toHaveBeenCalled()
  })

  it('calls onCancel when Cancel is clicked', () => {
    setupValidate({ ok: true })
    const onCancel = vi.fn()
    renderForm({ onCancel })
    screen.getByRole('button', { name: /cancel/i }).click()
    expect(onCancel).toHaveBeenCalled()
  })

  it('shows Save button label in edit mode', () => {
    setupValidate({ ok: true })
    renderForm({ isCreate: false })
    expect(screen.getByRole('button', { name: /^save$/i })).toBeInTheDocument()
  })

  it('pre-fills name when initial prop is provided', () => {
    setupValidate({ ok: true })
    renderForm({
      isCreate: false,
      initial: {
        id: 'x', project_id: 'p', name: 'existing-script', description: 'desc',
        code: 'pass', file_path: '', created_at: '', updated_at: '',
      },
    })
    expect(screen.getByDisplayValue('existing-script')).toBeInTheDocument()
  })
})

const INITIAL_WITH_PATH = {
  id: 'x', project_id: 'p', name: 'myscript', description: '',
  code: '', file_path: '/scripts/foo.py', created_at: '', updated_at: '',
}

describe('PythonScriptForm — file_path mode', () => {
  const mockRefetch = vi.fn()

  beforeEach(() => {
    setupValidate({ ok: true })
    vi.mocked(useReadPythonFile).mockReturnValue({
      data: undefined, isLoading: false, isFetching: false, isError: false, refetch: mockRefetch, error: null,
    } as unknown as ReturnType<typeof useReadPythonFile>)
  })

  it('shows editable editor and Check Syntax when file_path is empty', () => {
    renderForm()
    expect(screen.getByRole('button', { name: /check syntax/i })).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: /clear/i })).not.toBeInTheDocument()
    expect(screen.getByLabelText(/code editor/i)).not.toHaveAttribute('readonly')
  })

  it('renders read-only mode immediately when initial file_path is set', () => {
    renderForm({ isCreate: false, initial: INITIAL_WITH_PATH })
    expect(screen.getByLabelText(/code editor/i)).toHaveAttribute('readonly')
    expect(screen.queryByRole('button', { name: /check syntax/i })).not.toBeInTheDocument()
    expect(screen.getByRole('button', { name: /clear/i })).toBeInTheDocument()
    expect(screen.getByDisplayValue('/scripts/foo.py')).toBeInTheDocument()
  })

  it('Browse → select switches to read-only mode with path shown', async () => {
    vi.mocked(useReadPythonFile).mockReturnValue({
      data: { content: 'print("hello")', path: '/scripts/foo.py' },
      isLoading: false, isFetching: false, isError: false, refetch: mockRefetch, error: null,
    } as unknown as ReturnType<typeof useReadPythonFile>)
    const user = userEvent.setup()
    renderForm()

    await user.click(screen.getByRole('button', { name: /browse/i }))
    await user.click(screen.getByRole('button', { name: /select foo\.py/i }))

    expect(screen.getByDisplayValue('/scripts/foo.py')).toBeInTheDocument()
    expect(screen.getByLabelText(/code editor/i)).toHaveAttribute('readonly')
    expect(screen.queryByRole('button', { name: /check syntax/i })).not.toBeInTheDocument()
  })

  it('Reload from file calls refetch', async () => {
    renderForm({ isCreate: false, initial: INITIAL_WITH_PATH })
    const user = userEvent.setup()
    await user.click(screen.getByRole('button', { name: /reload from file/i }))
    expect(mockRefetch).toHaveBeenCalledOnce()
  })

  it('Clear restores editable editor and Check Syntax', async () => {
    renderForm({ isCreate: false, initial: INITIAL_WITH_PATH })
    const user = userEvent.setup()
    await user.click(screen.getByRole('button', { name: /clear/i }))

    expect(screen.getByRole('button', { name: /check syntax/i })).toBeInTheDocument()
    expect(screen.getByLabelText(/code editor/i)).not.toHaveAttribute('readonly')
    expect(screen.queryByRole('button', { name: /clear/i })).not.toBeInTheDocument()
  })

  it('submit with file_path skips validation and sends file_path', async () => {
    const onSubmit = vi.fn()
    const mutateAsync = vi.fn().mockResolvedValue({ ok: true })
    vi.mocked(useValidatePythonScript).mockReturnValue({
      mutateAsync, mutate: vi.fn(), isPending: false,
    } as unknown as ReturnType<typeof useValidatePythonScript>)

    renderForm({ isCreate: false, onSubmit, initial: INITIAL_WITH_PATH })
    const user = userEvent.setup()
    await user.click(screen.getByRole('button', { name: /^save$/i }))

    expect(mutateAsync).not.toHaveBeenCalled()
    expect(onSubmit).toHaveBeenCalledWith(expect.objectContaining({ file_path: '/scripts/foo.py' }))
  })

  it('submit after Clear sends empty file_path', async () => {
    const onSubmit = vi.fn()
    const user = userEvent.setup()
    renderForm({ isCreate: false, onSubmit, initial: INITIAL_WITH_PATH })

    await user.click(screen.getByRole('button', { name: /clear/i }))
    await user.click(screen.getByRole('button', { name: /^save$/i }))

    await screen.findByText(/syntax ok/i)
    expect(onSubmit).toHaveBeenCalledWith(expect.objectContaining({ file_path: '' }))
  })

  it('shows error message when file read fails', () => {
    vi.mocked(useReadPythonFile).mockReturnValue({
      data: undefined, isLoading: false, isFetching: false, isError: true, refetch: mockRefetch, error: null,
    } as unknown as ReturnType<typeof useReadPythonFile>)

    renderForm({ isCreate: false, initial: INITIAL_WITH_PATH })
    expect(screen.getByText(/failed to read file/i)).toBeInTheDocument()
  })
})
