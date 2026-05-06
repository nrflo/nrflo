import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { FilePickerModal } from './FilePickerModal'
import type { BrowseEntry, BrowseResponse } from '@/types/pythonScript'

const mockUseBrowse = vi.fn()

vi.mock('@/hooks/usePythonScripts', () => ({
  useBrowsePythonDir: (...args: unknown[]) => mockUseBrowse(...args),
}))

function makeEntry(name: string, is_dir: boolean, is_python = false): BrowseEntry {
  return { name, is_dir, is_python, size: 0, modified_at: '' }
}

function browseMock(path: string, entries: BrowseEntry[]) {
  return { data: { path, entries } as BrowseResponse, isLoading: false, isError: false, error: null }
}

function renderModal(props: Partial<React.ComponentProps<typeof FilePickerModal>> = {}) {
  const onClose = vi.fn()
  const onSelect = vi.fn()
  render(
    <FilePickerModal open={true} onClose={onClose} onSelect={onSelect} {...props} />
  )
  return { onClose, onSelect }
}

beforeEach(() => {
  vi.clearAllMocks()
  mockUseBrowse.mockReturnValue(browseMock('/', []))
})

describe('FilePickerModal — listing', () => {
  it('renders directories before .py files', () => {
    mockUseBrowse.mockReturnValue(browseMock('/home/user', [
      makeEntry('projects', true),
      makeEntry('scripts', true),
      makeEntry('foo.py', false, true),
      makeEntry('bar.py', false, true),
    ]))
    renderModal()

    const items = screen.getAllByRole('button', { name: /projects|scripts|foo\.py|bar\.py/i })
    expect(items[0]).toHaveTextContent('projects')
    expect(items[1]).toHaveTextContent('scripts')
    expect(items[2]).toHaveTextContent('foo.py')
    expect(items[3]).toHaveTextContent('bar.py')
  })

  it('shows current path in header', () => {
    mockUseBrowse.mockReturnValue(browseMock('/home/user', []))
    renderModal()
    expect(screen.getByText('/home/user')).toBeInTheDocument()
  })

  it('shows empty state message when no files or dirs', () => {
    mockUseBrowse.mockReturnValue(browseMock('/home/user', []))
    renderModal()
    expect(screen.getByText(/no files or directories/i)).toBeInTheDocument()
  })

  it('shows loading spinner when isLoading is true', () => {
    mockUseBrowse.mockReturnValue({ data: undefined, isLoading: true, isError: false, error: null })
    renderModal()
    expect(document.querySelector('.animate-spin')).toBeInTheDocument()
  })

  it('shows error message from error object', () => {
    mockUseBrowse.mockReturnValue({
      data: undefined, isLoading: false, isError: true, error: new Error('Permission denied'),
    })
    renderModal()
    expect(screen.getByText('Permission denied')).toBeInTheDocument()
  })

  it('shows fallback error message when error has no message', () => {
    mockUseBrowse.mockReturnValue({ data: undefined, isLoading: false, isError: true, error: null })
    renderModal()
    expect(screen.getByText(/failed to load directory/i)).toBeInTheDocument()
  })
})

describe('FilePickerModal — navigation', () => {
  it('up button is disabled at root path "/"', () => {
    mockUseBrowse.mockReturnValue(browseMock('/', []))
    renderModal()
    expect(screen.getByRole('button', { name: /go to parent directory/i })).toBeDisabled()
  })

  it('up button is disabled when path is empty', () => {
    mockUseBrowse.mockReturnValue(browseMock('', []))
    renderModal()
    expect(screen.getByRole('button', { name: /go to parent directory/i })).toBeDisabled()
  })

  it('up button is enabled when not at root', () => {
    mockUseBrowse.mockReturnValue(browseMock('/home/user', []))
    renderModal()
    expect(screen.getByRole('button', { name: /go to parent directory/i })).not.toBeDisabled()
  })

  it('clicking a directory navigates into it', async () => {
    mockUseBrowse.mockReturnValue(browseMock('/home/user', [
      makeEntry('scripts', true),
    ]))
    const user = userEvent.setup()
    renderModal()

    await user.click(screen.getByRole('button', { name: /scripts/i }))
    expect(mockUseBrowse).toHaveBeenCalledWith('/home/user/scripts')
  })

  it('clicking up navigates to parent directory', async () => {
    mockUseBrowse.mockReturnValue(browseMock('/home/user/scripts', []))
    const user = userEvent.setup()
    renderModal()

    await user.click(screen.getByRole('button', { name: /go to parent directory/i }))
    expect(mockUseBrowse).toHaveBeenCalledWith('/home/user')
  })

  it('clicking up from top-level path goes to "/"', async () => {
    mockUseBrowse.mockReturnValue(browseMock('/home', []))
    const user = userEvent.setup()
    renderModal()

    await user.click(screen.getByRole('button', { name: /go to parent directory/i }))
    expect(mockUseBrowse).toHaveBeenCalledWith('/')
  })
})

describe('FilePickerModal — file selection', () => {
  it('Select button is disabled before any file is clicked', () => {
    mockUseBrowse.mockReturnValue(browseMock('/home/user', [
      makeEntry('foo.py', false, true),
    ]))
    renderModal()
    expect(screen.getByRole('button', { name: /^select$/i })).toBeDisabled()
  })

  it('Select button enabled after clicking a .py file', async () => {
    mockUseBrowse.mockReturnValue(browseMock('/home/user', [
      makeEntry('foo.py', false, true),
    ]))
    const user = userEvent.setup()
    renderModal()

    await user.click(screen.getByRole('button', { name: /foo\.py/i }))
    expect(screen.getByRole('button', { name: /^select$/i })).not.toBeDisabled()
  })

  it('clicking Select calls onSelect with absolute path', async () => {
    mockUseBrowse.mockReturnValue(browseMock('/home/user', [
      makeEntry('foo.py', false, true),
    ]))
    const onSelect = vi.fn()
    const user = userEvent.setup()
    render(<FilePickerModal open={true} onClose={vi.fn()} onSelect={onSelect} />)

    await user.click(screen.getByRole('button', { name: /foo\.py/i }))
    await user.click(screen.getByRole('button', { name: /^select$/i }))

    expect(onSelect).toHaveBeenCalledWith('/home/user/foo.py')
    expect(onSelect).toHaveBeenCalledTimes(1)
  })

  it('Cancel button calls onClose', async () => {
    mockUseBrowse.mockReturnValue(browseMock('/home/user', []))
    const onClose = vi.fn()
    const user = userEvent.setup()
    render(<FilePickerModal open={true} onClose={onClose} onSelect={vi.fn()} />)

    await user.click(screen.getByRole('button', { name: /^cancel$/i }))
    expect(onClose).toHaveBeenCalled()
  })

  it('does not render when open is false', () => {
    mockUseBrowse.mockReturnValue(browseMock('/', []))
    renderModal({ open: false })
    expect(screen.queryByRole('button', { name: /^select$/i })).not.toBeInTheDocument()
  })
})
