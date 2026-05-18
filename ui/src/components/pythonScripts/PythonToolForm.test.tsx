import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { PythonToolForm } from './PythonToolForm'
import type { PythonScript } from '@/types/pythonScript'

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

vi.mock('./FilePickerModal', () => ({
  FilePickerModal: ({ open, onSelect }: {
    open: boolean
    onSelect: (p: string) => void
    onClose: () => void
  }) => {
    if (!open) return null
    return <button onClick={() => onSelect('/tools/my_tool.py')}>Select my_tool.py</button>
  },
}))

vi.mock('@/hooks/usePythonScripts', () => ({
  useReadPythonFile: vi.fn().mockReturnValue({
    data: undefined, isLoading: false, isFetching: false, isError: false, refetch: vi.fn(), error: null,
  }),
}))

// Fully-valid edit-mode base — use spread overrides to test a single invalid field
const VALID_INITIAL: PythonScript = {
  id: 'x', project_id: 'p', kind: 'tool',
  name: 'existing-tool', description: '',
  tool_description: 'A tool', input_schema: '{}', timeout_sec: 30,
  code: 'pass', file_path: '', created_at: '', updated_at: '',
}

function renderForm(props: Partial<React.ComponentProps<typeof PythonToolForm>> = {}) {
  return render(
    <PythonToolForm
      isCreate={true}
      onSubmit={vi.fn()}
      onValidationFailure={vi.fn()}
      onCancel={vi.fn()}
      {...props}
    />
  )
}

// Type name + toolDesc + valid JSON in schema so validateLocalFields passes those three checks.
// Uses 'null' (valid JSON, no userEvent special chars) for schema.
async function fillBase(user: ReturnType<typeof userEvent.setup>) {
  await user.type(screen.getByPlaceholderText(/e\.g\., search-web/), 'my-tool')
  await user.type(screen.getByPlaceholderText(/describe what this tool does for the llm/i), 'A tool')
  // Index 0 = schema CodeEditor (always first in DOM order)
  await user.type(screen.getAllByLabelText(/code editor/i)[0], 'null')
}

beforeEach(() => vi.clearAllMocks())

describe('PythonToolForm — required fields', () => {
  it('does not call onSubmit when name is empty', async () => {
    const user = userEvent.setup()
    const onSubmit = vi.fn()
    renderForm({ onSubmit })
    await user.type(screen.getByPlaceholderText(/describe what this tool does for the llm/i), 'A tool')
    await user.type(screen.getAllByLabelText(/code editor/i)[0], 'null')
    await user.click(screen.getByRole('button', { name: /^create$/i }))
    expect(onSubmit).not.toHaveBeenCalled()
  })

  it('does not call onSubmit when tool_description is empty', async () => {
    const user = userEvent.setup()
    const onSubmit = vi.fn()
    renderForm({ onSubmit })
    await user.type(screen.getByPlaceholderText(/e\.g\., search-web/), 'my-tool')
    await user.type(screen.getAllByLabelText(/code editor/i)[0], 'null')
    await user.click(screen.getByRole('button', { name: /^create$/i }))
    expect(onSubmit).not.toHaveBeenCalled()
  })
})

describe('PythonToolForm — input_schema validation', () => {
  it('shows Invalid JSON error and does not call onSubmit', async () => {
    const user = userEvent.setup()
    const onSubmit = vi.fn()
    renderForm({ onSubmit })
    await user.type(screen.getByPlaceholderText(/e\.g\., search-web/), 'my-tool')
    await user.type(screen.getByPlaceholderText(/describe what this tool does for the llm/i), 'A tool')
    await user.type(screen.getAllByLabelText(/code editor/i)[0], 'not-valid-json')
    await user.click(screen.getByRole('button', { name: /^create$/i }))
    expect(screen.getByText('Invalid JSON')).toBeInTheDocument()
    expect(onSubmit).not.toHaveBeenCalled()
  })

  it('clears Invalid JSON error when schema onChange fires', async () => {
    const user = userEvent.setup()
    renderForm()
    // Fill required fields so handleSubmit is actually called
    await user.type(screen.getByPlaceholderText(/e\.g\., search-web/), 'my-tool')
    await user.type(screen.getByPlaceholderText(/describe what this tool does for the llm/i), 'A tool')
    const schemaEditor = screen.getAllByLabelText(/code editor/i)[0]
    await user.type(schemaEditor, 'bad-json')
    await user.click(screen.getByRole('button', { name: /^create$/i }))
    expect(screen.getByText('Invalid JSON')).toBeInTheDocument()
    // Fixing the schema calls setSchemaError(null) on every onChange
    await user.clear(schemaEditor)
    await user.type(schemaEditor, 'null')
    expect(screen.queryByText('Invalid JSON')).not.toBeInTheDocument()
  })
})

describe('PythonToolForm — timeout validation', () => {
  // fireEvent.submit bypasses HTML5 constraint validation to reach the JS path
  it('shows Must be 1–600 error for timeout 0 via JS validation', () => {
    const onSubmit = vi.fn()
    const { container } = renderForm({ isCreate: false, onSubmit, initial: { ...VALID_INITIAL, timeout_sec: 0 } })
    fireEvent.submit(container.querySelector('form')!)
    expect(screen.getByText('Must be 1–600')).toBeInTheDocument()
    expect(onSubmit).not.toHaveBeenCalled()
  })

  it('shows Must be 1–600 error for timeout 601 via JS validation', () => {
    const onSubmit = vi.fn()
    const { container } = renderForm({ isCreate: false, onSubmit, initial: { ...VALID_INITIAL, timeout_sec: 601 } })
    fireEvent.submit(container.querySelector('form')!)
    expect(screen.getByText('Must be 1–600')).toBeInTheDocument()
    expect(onSubmit).not.toHaveBeenCalled()
  })

  it('accepts boundary value 600', async () => {
    const user = userEvent.setup()
    const onSubmit = vi.fn()
    renderForm({ isCreate: false, onSubmit, initial: { ...VALID_INITIAL, timeout_sec: 600 } })
    await user.click(screen.getByRole('button', { name: /^save$/i }))
    expect(onSubmit).toHaveBeenCalled()
  })
})

describe('PythonToolForm — file_path mode', () => {
  it('Browse → select sets file_path and shows Clear button', async () => {
    const user = userEvent.setup()
    renderForm()
    await user.click(screen.getByRole('button', { name: /browse/i }))
    await user.click(screen.getByRole('button', { name: /select my_tool\.py/i }))
    expect(screen.getByDisplayValue('/tools/my_tool.py')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /^clear$/i })).toBeInTheDocument()
  })

  it('with file_path set, onSubmit is called without triggering onValidationFailure', async () => {
    const user = userEvent.setup()
    const onSubmit = vi.fn()
    const onValidationFailure = vi.fn()
    renderForm({ onSubmit, onValidationFailure })

    await user.click(screen.getByRole('button', { name: /browse/i }))
    await user.click(screen.getByRole('button', { name: /select my_tool\.py/i }))

    // Fill other required fields; code is empty but file_path mode skips the code check
    await user.type(screen.getByPlaceholderText(/e\.g\., search-web/), 'my-tool')
    await user.type(screen.getByPlaceholderText(/describe what this tool does for the llm/i), 'A tool')
    await user.type(screen.getAllByLabelText(/code editor/i)[0], 'null')

    await user.click(screen.getByRole('button', { name: /^create$/i }))

    expect(onSubmit).toHaveBeenCalledWith(expect.objectContaining({ file_path: '/tools/my_tool.py' }))
    expect(onValidationFailure).not.toHaveBeenCalled()
  })

  it('Clear restores inline code mode and removes file path', async () => {
    const user = userEvent.setup()
    renderForm()
    await user.click(screen.getByRole('button', { name: /browse/i }))
    await user.click(screen.getByRole('button', { name: /select my_tool\.py/i }))
    expect(screen.getByDisplayValue('/tools/my_tool.py')).toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: /^clear$/i }))
    expect(screen.queryByDisplayValue('/tools/my_tool.py')).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: /^clear$/i })).not.toBeInTheDocument()
  })
})

describe('PythonToolForm — payload kind', () => {
  it('create payload contains kind: tool', async () => {
    const user = userEvent.setup()
    const onSubmit = vi.fn()
    renderForm({ isCreate: true, onSubmit })
    await fillBase(user)
    // Index 1 = inline code CodeEditor; must be non-empty for submit to fire
    await user.type(screen.getAllByLabelText(/code editor/i)[1], 'pass')
    await user.click(screen.getByRole('button', { name: /^create$/i }))
    expect(onSubmit).toHaveBeenCalledWith(expect.objectContaining({ kind: 'tool' }))
  })

  it('edit payload omits kind field', async () => {
    const user = userEvent.setup()
    const onSubmit = vi.fn()
    renderForm({ isCreate: false, onSubmit, initial: VALID_INITIAL })
    await user.click(screen.getByRole('button', { name: /^save$/i }))
    expect(onSubmit).toHaveBeenCalled()
    const arg = onSubmit.mock.calls[0][0] as Record<string, unknown>
    expect(arg).not.toHaveProperty('kind')
  })
})

describe('PythonToolForm — submit button labels', () => {
  it('shows Create in create mode', () => {
    renderForm({ isCreate: true })
    expect(screen.getByRole('button', { name: /^create$/i })).toBeInTheDocument()
  })

  it('shows Save in edit mode', () => {
    renderForm({ isCreate: false })
    expect(screen.getByRole('button', { name: /^save$/i })).toBeInTheDocument()
  })
})
