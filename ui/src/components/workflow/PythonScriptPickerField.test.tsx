import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { PythonScriptPickerField } from './PythonScriptPickerField'
import type { PythonScript } from '@/types/pythonScript'

vi.mock('@/hooks/usePythonScripts', () => ({
  usePythonScripts: vi.fn(),
}))

import { usePythonScripts } from '@/hooks/usePythonScripts'

function makeScript(overrides: Partial<PythonScript> = {}): PythonScript {
  return {
    id: 'script-1',
    project_id: 'proj-1',
    name: 'my-script',
    description: '',
    code: 'print()',
    created_at: '',
    updated_at: '',
    ...overrides,
  }
}

function renderPicker(props: Partial<React.ComponentProps<typeof PythonScriptPickerField>> = {}) {
  return render(
    <MemoryRouter>
      <PythonScriptPickerField value="" onChange={vi.fn()} {...props} />
    </MemoryRouter>
  )
}

beforeEach(() => vi.clearAllMocks())

describe('PythonScriptPickerField', () => {
  it('shows loading text while scripts are loading', () => {
    vi.mocked(usePythonScripts).mockReturnValue({ data: [], isLoading: true } as ReturnType<typeof usePythonScripts>)
    renderPicker()
    expect(screen.getByText('Loading scripts…')).toBeInTheDocument()
  })

  it('shows empty state with link when no scripts exist', () => {
    vi.mocked(usePythonScripts).mockReturnValue({ data: [], isLoading: false } as ReturnType<typeof usePythonScripts>)
    renderPicker()
    expect(screen.getByText(/No scripts yet/)).toBeInTheDocument()
    const link = screen.getByRole('link', { name: /create one on the Python Scripts page/i })
    expect(link).toHaveAttribute('href', '/python-scripts')
  })

  it('renders Dropdown placeholder when scripts exist', () => {
    vi.mocked(usePythonScripts).mockReturnValue({
      data: [makeScript({ id: 's1', name: 'alpha' })],
      isLoading: false,
    } as ReturnType<typeof usePythonScripts>)
    renderPicker()
    expect(screen.getByText('Select a Python script…')).toBeInTheDocument()
  })

  it('shows script name without description as plain label', () => {
    vi.mocked(usePythonScripts).mockReturnValue({
      data: [makeScript({ id: 's1', name: 'no-desc', description: '' })],
      isLoading: false,
    } as ReturnType<typeof usePythonScripts>)
    renderPicker({ value: 's1' })
    // The selected script label should appear in the dropdown trigger
    expect(screen.getByText('no-desc')).toBeInTheDocument()
  })

  it('shows script name with description in label', () => {
    vi.mocked(usePythonScripts).mockReturnValue({
      data: [makeScript({ id: 's1', name: 'my-script', description: 'Does something' })],
      isLoading: false,
    } as ReturnType<typeof usePythonScripts>)
    renderPicker({ value: 's1' })
    expect(screen.getByText('my-script — Does something')).toBeInTheDocument()
  })

  it('truncates description at 60 characters with ellipsis', () => {
    const longDesc = 'A'.repeat(65)
    vi.mocked(usePythonScripts).mockReturnValue({
      data: [makeScript({ id: 's1', name: 'x', description: longDesc })],
      isLoading: false,
    } as ReturnType<typeof usePythonScripts>)
    renderPicker({ value: 's1' })
    expect(screen.getByText(`x — ${'A'.repeat(60)}…`)).toBeInTheDocument()
  })
})
