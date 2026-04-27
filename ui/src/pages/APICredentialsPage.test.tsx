import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { APICredentialsPage } from './APICredentialsPage'
import type { APICredential } from '@/types/apiCredential'

vi.mock('@/hooks/useAPICredentials', () => ({
  useAPICredentials: vi.fn(),
  useCreateAPICredential: vi.fn(),
  useUpdateAPICredential: vi.fn(),
  useDeleteAPICredential: vi.fn(),
}))

vi.mock('@/hooks/useProjects', () => ({
  useProjects: vi.fn(),
}))

import {
  useAPICredentials,
  useCreateAPICredential,
  useUpdateAPICredential,
  useDeleteAPICredential,
} from '@/hooks/useAPICredentials'
import { useProjects } from '@/hooks/useProjects'

function makeCredential(overrides: Partial<APICredential> = {}): APICredential {
  return {
    id: 'cred-1',
    provider: 'anthropic',
    secret_ref: 'env:ANTHROPIC_API_KEY',
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:00:00Z',
    ...overrides,
  }
}

function setupMocks(credentials: APICredential[] = []) {
  vi.mocked(useAPICredentials).mockReturnValue({ data: credentials, isLoading: false, error: null } as unknown as ReturnType<typeof useAPICredentials>)
  vi.mocked(useCreateAPICredential).mockReturnValue({ mutate: vi.fn(), isPending: false } as unknown as ReturnType<typeof useCreateAPICredential>)
  vi.mocked(useUpdateAPICredential).mockReturnValue({ mutate: vi.fn(), isPending: false } as unknown as ReturnType<typeof useUpdateAPICredential>)
  vi.mocked(useDeleteAPICredential).mockReturnValue({ mutate: vi.fn(), isPending: false } as unknown as ReturnType<typeof useDeleteAPICredential>)
  vi.mocked(useProjects).mockReturnValue({
    data: { projects: [{ id: 'proj-a', name: 'Alpha Project' }] },
  } as unknown as ReturnType<typeof useProjects>)
}

beforeEach(() => {
  vi.clearAllMocks()
})

describe('APICredentialsPage', () => {
  describe('list rendering', () => {
    it('renders credential with provider and secret_ref', () => {
      setupMocks([makeCredential()])
      render(<APICredentialsPage />)
      expect(screen.getByText('anthropic')).toBeInTheDocument()
      expect(screen.getByText('env:ANTHROPIC_API_KEY')).toBeInTheDocument()
    })

    it('redacts literal: secrets as literal:***', () => {
      setupMocks([makeCredential({ secret_ref: 'literal:sk-real-secret-key' })])
      render(<APICredentialsPage />)
      expect(screen.getByText('literal:***')).toBeInTheDocument()
      expect(screen.queryByText('literal:sk-real-secret-key')).not.toBeInTheDocument()
    })

    it('does not redact env: refs', () => {
      setupMocks([makeCredential({ secret_ref: 'env:MY_KEY' })])
      render(<APICredentialsPage />)
      expect(screen.getByText('env:MY_KEY')).toBeInTheDocument()
    })

    it('does not redact file: refs', () => {
      setupMocks([makeCredential({ secret_ref: 'file:/path/to/key' })])
      render(<APICredentialsPage />)
      expect(screen.getByText('file:/path/to/key')).toBeInTheDocument()
    })

    it('resolves project name from project_id', () => {
      setupMocks([makeCredential({ project_id: 'proj-a' })])
      render(<APICredentialsPage />)
      expect(screen.getByText('Alpha Project')).toBeInTheDocument()
    })

    it('shows Global for credentials without project_id', () => {
      setupMocks([makeCredential()])
      render(<APICredentialsPage />)
      expect(screen.getByText('Global')).toBeInTheDocument()
    })

    it('shows empty state when no credentials', () => {
      setupMocks([])
      render(<APICredentialsPage />)
      expect(screen.getByText('No API credentials configured yet.')).toBeInTheDocument()
    })

    it('renders multiple credentials', () => {
      setupMocks([
        makeCredential({ id: 'c1', secret_ref: 'env:KEY1' }),
        makeCredential({ id: 'c2', secret_ref: 'env:KEY2' }),
      ])
      render(<APICredentialsPage />)
      expect(screen.getByText('env:KEY1')).toBeInTheDocument()
      expect(screen.getByText('env:KEY2')).toBeInTheDocument()
    })
  })

  describe('create form', () => {
    it('opens create form when New Credential is clicked', async () => {
      const user = userEvent.setup()
      setupMocks([])
      render(<APICredentialsPage />)

      await user.click(screen.getByRole('button', { name: /New Credential/i }))

      expect(screen.getByText('Provider')).toBeInTheDocument()
      expect(screen.getByText('Secret Ref')).toBeInTheDocument()
    })

    it('shows anthropic as default provider', async () => {
      const user = userEvent.setup()
      setupMocks([])
      render(<APICredentialsPage />)

      await user.click(screen.getByRole('button', { name: /New Credential/i }))

      const providerLabel = screen.getByText('Provider')
      const providerBtn = providerLabel.parentElement!.querySelector('button[type="button"]') as HTMLButtonElement
      expect(providerBtn.textContent).toContain('Anthropic')
    })

    it('shows secret ref format hint', async () => {
      const user = userEvent.setup()
      setupMocks([])
      render(<APICredentialsPage />)

      await user.click(screen.getByRole('button', { name: /New Credential/i }))

      expect(screen.getByText(/env:VAR_NAME/)).toBeInTheDocument()
    })

    it('cancels create form when Cancel is clicked', async () => {
      const user = userEvent.setup()
      setupMocks([])
      render(<APICredentialsPage />)

      await user.click(screen.getByRole('button', { name: /New Credential/i }))
      expect(screen.getByText('Secret Ref')).toBeInTheDocument()

      await user.click(screen.getByRole('button', { name: /^Cancel$/i }))
      expect(screen.queryByText('Secret Ref')).not.toBeInTheDocument()
    })

    it('calls createAPICredential.mutate on form submit', async () => {
      const user = userEvent.setup()
      const createMutateSpy = vi.fn()
      vi.mocked(useCreateAPICredential).mockReturnValue({ mutate: createMutateSpy, isPending: false } as unknown as ReturnType<typeof useCreateAPICredential>)
      setupMocks([])
      vi.mocked(useCreateAPICredential).mockReturnValue({ mutate: createMutateSpy, isPending: false } as unknown as ReturnType<typeof useCreateAPICredential>)
      render(<APICredentialsPage />)

      await user.click(screen.getByRole('button', { name: /New Credential/i }))

      const secretInput = screen.getByPlaceholderText(/env:ANTHROPIC_API_KEY/i)
      await user.type(secretInput, 'env:MY_TEST_KEY')

      await user.click(screen.getByRole('button', { name: /^Create$/i }))

      expect(createMutateSpy).toHaveBeenCalledWith(
        expect.objectContaining({ provider: 'anthropic', secret_ref: 'env:MY_TEST_KEY' }),
        expect.anything()
      )
    })
  })

  describe('delete flow', () => {
    it('shows delete confirm dialog when delete button is clicked', async () => {
      const user = userEvent.setup()
      setupMocks([makeCredential()])
      render(<APICredentialsPage />)

      const buttons = screen.getAllByRole('button')
      // Trash button is last among the per-row buttons
      const trashBtn = buttons[buttons.length - 1]
      await user.click(trashBtn)

      expect(screen.getByText('Delete API Credential')).toBeInTheDocument()
    })

    it('calls deleteAPICredential.mutate on confirm', async () => {
      const user = userEvent.setup()
      const deleteMutateSpy = vi.fn()
      vi.mocked(useDeleteAPICredential).mockReturnValue({ mutate: deleteMutateSpy, isPending: false } as unknown as ReturnType<typeof useDeleteAPICredential>)
      setupMocks([makeCredential()])
      vi.mocked(useDeleteAPICredential).mockReturnValue({ mutate: deleteMutateSpy, isPending: false } as unknown as ReturnType<typeof useDeleteAPICredential>)
      render(<APICredentialsPage />)

      const buttons = screen.getAllByRole('button')
      await user.click(buttons[buttons.length - 1])
      await user.click(screen.getByRole('button', { name: /^Delete$/i }))

      expect(deleteMutateSpy).toHaveBeenCalledWith(
        'cred-1',
        expect.objectContaining({ onSuccess: expect.any(Function) })
      )
    })
  })
})
