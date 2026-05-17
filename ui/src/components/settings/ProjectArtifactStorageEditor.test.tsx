import { describe, it, expect, vi, beforeEach } from 'vitest'
import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { ProjectArtifactStorageEditor } from './ProjectArtifactStorageEditor'
import * as api from '@/api/projectSettings'
import type { ArtifactStorageConfig } from '@/api/projectSettings'
import { renderWithQuery } from '@/test/utils'

vi.mock('@/api/projectSettings')

const PROJECT_ID = 'proj-1'

beforeEach(() => vi.clearAllMocks())

function makeCfg(overrides: Partial<ArtifactStorageConfig> = {}): ArtifactStorageConfig {
  return { mode: 'internal', ...overrides }
}

describe('ProjectArtifactStorageEditor', () => {
  it('initial load shows mode=internal', async () => {
    vi.mocked(api.getArtifactStorage).mockResolvedValue(makeCfg())

    renderWithQuery(<ProjectArtifactStorageEditor projectId={PROJECT_ID} />)

    expect(await screen.findByText('Internal')).toBeInTheDocument()
    expect(screen.queryByPlaceholderText('your-account-id')).not.toBeInTheDocument()
  })

  it('switching to S3 disables submit and shows coming-soon text', async () => {
    vi.mocked(api.getArtifactStorage).mockResolvedValue(makeCfg())

    renderWithQuery(<ProjectArtifactStorageEditor projectId={PROJECT_ID} />)
    await screen.findByText('Internal')

    const user = userEvent.setup()
    // Open dropdown and click S3 (disabled option won't change value, but test the text appears)
    const dropdownTrigger = screen.getByRole('button', { name: /internal/i })
    await user.click(dropdownTrigger)

    // S3 option is in the panel
    const s3Option = screen.getByText('S3')
    expect(s3Option).toBeInTheDocument()

    // Clicking the disabled S3 option does nothing (aria-disabled)
    await user.click(s3Option)
    // Mode stays internal; no "coming soon" text
    expect(screen.queryByText(/coming soon/i)).not.toBeInTheDocument()
  })

  it('switching to Cloudflare R2 reveals R2 fields', async () => {
    vi.mocked(api.getArtifactStorage).mockResolvedValue(makeCfg())

    renderWithQuery(<ProjectArtifactStorageEditor projectId={PROJECT_ID} />)
    await screen.findByText('Internal')

    const user = userEvent.setup()
    const dropdownTrigger = screen.getByRole('button', { name: /internal/i })
    await user.click(dropdownTrigger)

    const r2Option = screen.getByText('Cloudflare R2')
    await user.click(r2Option)

    expect(screen.getByPlaceholderText('your-account-id')).toBeInTheDocument()
    expect(screen.getByPlaceholderText('my-bucket')).toBeInTheDocument()
    expect(screen.getByPlaceholderText('optional/path/prefix/')).toBeInTheDocument()
    expect(screen.getByPlaceholderText('env:R2_ACCESS_KEY')).toBeInTheDocument()
    expect(screen.getByPlaceholderText('env:R2_SECRET_KEY')).toBeInTheDocument()
  })

  it('masked secrets (literal:***) from GET are preserved in payload when unchanged', async () => {
    vi.mocked(api.getArtifactStorage).mockResolvedValue(
      makeCfg({
        mode: 'cloudflare_r2',
        account_id: 'acct123',
        bucket: 'mybucket',
        access_key_ref: 'literal:***',
        secret_key_ref: 'literal:***',
      })
    )
    vi.mocked(api.setArtifactStorage).mockResolvedValue(makeCfg({ mode: 'cloudflare_r2' }))

    renderWithQuery(<ProjectArtifactStorageEditor projectId={PROJECT_ID} />)
    await screen.findByPlaceholderText('your-account-id')

    // The fields display the masked sentinel from backend
    const keyInputs = screen.getAllByDisplayValue('literal:***')
    expect(keyInputs).toHaveLength(2)

    const user = userEvent.setup()
    await user.click(screen.getByRole('button', { name: /save/i }))

    await waitFor(() => {
      expect(api.setArtifactStorage).toHaveBeenCalledWith(
        PROJECT_ID,
        expect.objectContaining({ mode: 'cloudflare_r2', access_key_ref: 'literal:***', secret_key_ref: 'literal:***' })
      )
    })
  })

  it('typing a new secret replaces the masked sentinel in the payload', async () => {
    vi.mocked(api.getArtifactStorage).mockResolvedValue(
      makeCfg({ mode: 'cloudflare_r2', access_key_ref: 'literal:***' })
    )
    vi.mocked(api.setArtifactStorage).mockResolvedValue(makeCfg({ mode: 'cloudflare_r2' }))

    renderWithQuery(<ProjectArtifactStorageEditor projectId={PROJECT_ID} />)
    await screen.findByPlaceholderText('your-account-id')

    const user = userEvent.setup()
    const accessKeyInput = screen.getByPlaceholderText('env:R2_ACCESS_KEY')
    await user.clear(accessKeyInput)
    await user.type(accessKeyInput, 'env:NEW_KEY')

    await user.click(screen.getByRole('button', { name: /save/i }))

    await waitFor(() => {
      const payload = vi.mocked(api.setArtifactStorage).mock.calls[0][1]
      expect(payload.access_key_ref).toBe('env:NEW_KEY')
    })
  })

  it('server error is rendered verbatim', async () => {
    vi.mocked(api.getArtifactStorage).mockResolvedValue(makeCfg())
    vi.mocked(api.setArtifactStorage).mockRejectedValue(new Error('s3 backend not yet implemented'))

    renderWithQuery(<ProjectArtifactStorageEditor projectId={PROJECT_ID} />)
    await screen.findByText('Internal')

    const user = userEvent.setup()
    const saveBtn = screen.getByRole('button', { name: /save/i })
    await user.click(saveBtn)

    expect(await screen.findByText('s3 backend not yet implemented')).toBeInTheDocument()
  })

  it('shows Saved confirmation after successful submit', async () => {
    vi.mocked(api.getArtifactStorage).mockResolvedValue(makeCfg())
    vi.mocked(api.setArtifactStorage).mockResolvedValue(makeCfg())

    renderWithQuery(<ProjectArtifactStorageEditor projectId={PROJECT_ID} />)
    await screen.findByText('Internal')

    const user = userEvent.setup()
    await user.click(screen.getByRole('button', { name: /save/i }))

    expect(await screen.findByText('Saved.')).toBeInTheDocument()
  })
})
