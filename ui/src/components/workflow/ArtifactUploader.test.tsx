import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { ArtifactUploader } from './ArtifactUploader'
import type { InputArtifactRef } from '@/types/artifact'

const mockUploadArtifact = vi.fn()
const mockCancelUpload = vi.fn()

vi.mock('@/api/artifacts', () => ({
  uploadArtifact: (...args: unknown[]) => mockUploadArtifact(...args),
  cancelUpload: (...args: unknown[]) => mockCancelUpload(...args),
}))

describe('ArtifactUploader', () => {
  const onChange = vi.fn<(refs: InputArtifactRef[], hasPending: boolean) => void>()

  beforeEach(() => {
    vi.clearAllMocks()
    mockCancelUpload.mockResolvedValue(undefined)
  })

  it('renders drop zone with Browse button', () => {
    render(<ArtifactUploader onChange={onChange} />)
    expect(screen.getByText(/drop files here/i)).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /browse/i })).toBeInTheDocument()
  })

  it('calls onChange with hasPending=true while upload is in-flight', async () => {
    mockUploadArtifact.mockReturnValue(new Promise(() => {}))
    const user = userEvent.setup()
    render(<ArtifactUploader onChange={onChange} />)

    const file = new File(['content'], 'pending.txt', { type: 'text/plain' })
    const input = document.querySelector<HTMLInputElement>('input[type="file"]')!
    await user.upload(input, file)

    await waitFor(() => {
      const calls = onChange.mock.calls
      expect(calls.some(([, pending]) => pending === true)).toBe(true)
    })
  })

  it('uploads file and calls onChange with refs on success', async () => {
    mockUploadArtifact.mockResolvedValue({ upload_id: 'uid-1', name: 'test.txt' })
    const user = userEvent.setup()
    render(<ArtifactUploader onChange={onChange} />)

    const file = new File(['content'], 'test.txt', { type: 'text/plain' })
    const input = document.querySelector<HTMLInputElement>('input[type="file"]')!
    await user.upload(input, file)

    expect(mockUploadArtifact).toHaveBeenCalledWith(file)
    await waitFor(() => {
      const lastCall = onChange.mock.calls[onChange.mock.calls.length - 1]
      expect(lastCall[0]).toEqual([{ upload_id: 'uid-1', name: 'test.txt' }])
      expect(lastCall[1]).toBe(false)
    })
    expect(screen.getByText('test.txt')).toBeInTheDocument()
  })

  it('shows "Upload failed" error text when upload throws', async () => {
    mockUploadArtifact.mockRejectedValue(new Error('network error'))
    const user = userEvent.setup()
    render(<ArtifactUploader onChange={onChange} />)

    const file = new File(['x'], 'fail.txt', { type: 'text/plain' })
    const input = document.querySelector<HTMLInputElement>('input[type="file"]')!
    await user.upload(input, file)

    await waitFor(() => {
      expect(screen.getByText('Upload failed')).toBeInTheDocument()
    })
  })

  it('calls cancelUpload and removes file on X click after successful upload', async () => {
    mockUploadArtifact.mockResolvedValue({ upload_id: 'uid-1', name: 'test.txt' })
    const user = userEvent.setup()
    render(<ArtifactUploader onChange={onChange} />)

    const file = new File(['content'], 'test.txt', { type: 'text/plain' })
    const input = document.querySelector<HTMLInputElement>('input[type="file"]')!
    await user.upload(input, file)

    // Wait for upload to complete
    await waitFor(() => {
      const lastCall = onChange.mock.calls[onChange.mock.calls.length - 1]
      expect(lastCall[1]).toBe(false)
    })

    // Find and click the X remove button (only non-Browse button in file list)
    const allButtons = screen.getAllByRole('button')
    const removeBtn = allButtons[allButtons.length - 1]
    await user.click(removeBtn)

    expect(mockCancelUpload).toHaveBeenCalledWith('uid-1')
    await waitFor(() => {
      expect(screen.queryByText('test.txt')).not.toBeInTheDocument()
    })
    const lastCall = onChange.mock.calls[onChange.mock.calls.length - 1]
    expect(lastCall[0]).toEqual([])
  })

  it('does not call cancelUpload when removing a failed upload', async () => {
    mockUploadArtifact.mockRejectedValue(new Error('fail'))
    const user = userEvent.setup()
    render(<ArtifactUploader onChange={onChange} />)

    const file = new File(['x'], 'fail.txt', { type: 'text/plain' })
    const input = document.querySelector<HTMLInputElement>('input[type="file"]')!
    await user.upload(input, file)

    await waitFor(() => screen.getByText('Upload failed'))

    const allButtons = screen.getAllByRole('button')
    await user.click(allButtons[allButtons.length - 1])

    expect(mockCancelUpload).not.toHaveBeenCalled()
    await waitFor(() => {
      expect(screen.queryByText('fail.txt')).not.toBeInTheDocument()
    })
  })
})
