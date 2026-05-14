import { describe, it, expect, vi, beforeEach } from 'vitest'
import { screen, within, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router-dom'
import { ProjectEnvVarsEditor } from './ProjectEnvVarsEditor'
import * as api from '@/api/projectEnvVars'
import * as catalogHook from '@/hooks/useEnvVarCatalog'
import { toast } from 'sonner'
import { renderWithQuery } from '@/test/utils'
import type { ProjectEnvVar } from '@/api/projectEnvVars'
import type { EnvVarCatalogEntry } from '@/api/specImport'

vi.mock('@/api/projectEnvVars')
vi.mock('@/hooks/useEnvVarCatalog')
vi.mock('sonner')

function makeVar(overrides: Partial<ProjectEnvVar> = {}): ProjectEnvVar {
  return {
    name: 'API_KEY',
    value: 'secret',
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:00:00Z',
    ...overrides,
  }
}

function makeEntry(overrides: Partial<EnvVarCatalogEntry> = {}): EnvVarCatalogEntry {
  return {
    name: 'GITHUB_TOKEN',
    feature: 'github',
    description: 'GitHub personal access token',
    required: false,
    ...overrides,
  }
}

const PROJECT_ID = 'proj-1'

const CATALOG: EnvVarCatalogEntry[] = [
  makeEntry({ name: 'GITHUB_TOKEN', feature: 'github', required: true }),
  makeEntry({ name: 'JIRA_TOKEN', feature: 'jira', required: false }),
  makeEntry({ name: 'SLACK_TOKEN', feature: 'slack', required: false }),
  makeEntry({ name: 'LINEAR_KEY', feature: 'linear', required: false }),
]

beforeEach(() => {
  vi.clearAllMocks()
  vi.mocked(catalogHook.useEnvVarCatalog).mockReturnValue({ data: CATALOG, isLoading: false } as any)
  vi.mocked(api.listEnvVars).mockResolvedValue([])
})

describe('ProjectEnvVarsEditor catalog panel', () => {
  it('shows suggested variables when expanded', async () => {
    renderWithQuery(<MemoryRouter><ProjectEnvVarsEditor projectId={PROJECT_ID} /></MemoryRouter>)
    await screen.findByPlaceholderText('VAR_NAME')

    expect(screen.queryByText('GITHUB_TOKEN')).not.toBeInTheDocument()

    await userEvent.setup().click(screen.getByText('Suggested variables'))

    expect(screen.getByText('GITHUB_TOKEN')).toBeInTheDocument()
    expect(screen.getByText('JIRA_TOKEN')).toBeInTheDocument()
  })

  it('click row prefills add-form name input', async () => {
    renderWithQuery(<MemoryRouter><ProjectEnvVarsEditor projectId={PROJECT_ID} /></MemoryRouter>)
    await screen.findByPlaceholderText('VAR_NAME')

    const user = userEvent.setup()
    await user.click(screen.getByText('Suggested variables'))
    await user.click(screen.getByText('GITHUB_TOKEN'))

    expect(screen.getByPlaceholderText('VAR_NAME')).toHaveValue('GITHUB_TOKEN')
  })

  it('copy button writes name to clipboard and shows toast', async () => {
    renderWithQuery(<MemoryRouter><ProjectEnvVarsEditor projectId={PROJECT_ID} /></MemoryRouter>)
    await screen.findByPlaceholderText('VAR_NAME')

    // Call userEvent.setup() first so its clipboard stub is in place, then spy on it
    const user = userEvent.setup()
    const writeTextSpy = vi.spyOn(navigator.clipboard, 'writeText').mockResolvedValue(undefined)

    await user.click(screen.getByText('Suggested variables'))

    const githubRow = screen.getByText('GITHUB_TOKEN').closest('li')!
    const copyButton = within(githubRow).getByRole('button')
    await user.click(copyButton)

    expect(writeTextSpy).toHaveBeenCalledWith('GITHUB_TOKEN')
    await waitFor(() =>
      expect(toast.success).toHaveBeenCalledWith('Copied GITHUB_TOKEN')
    )
  })

  it('already-defined name renders Set badge', async () => {
    vi.mocked(api.listEnvVars).mockResolvedValue([
      makeVar({ name: 'GITHUB_TOKEN', value: 'ghp_xxx' }),
    ])

    renderWithQuery(<MemoryRouter><ProjectEnvVarsEditor projectId={PROJECT_ID} /></MemoryRouter>)
    await screen.findByText('GITHUB_TOKEN') // wait for table row

    await userEvent.setup().click(screen.getByText('Suggested variables'))

    expect(screen.getByText('Set')).toBeInTheDocument()
  })

  it('panel not rendered when catalog is empty', async () => {
    vi.mocked(catalogHook.useEnvVarCatalog).mockReturnValue({ data: [], isLoading: false } as any)

    renderWithQuery(<MemoryRouter><ProjectEnvVarsEditor projectId={PROJECT_ID} /></MemoryRouter>)
    await screen.findByPlaceholderText('VAR_NAME')

    expect(screen.queryByText('Suggested variables')).not.toBeInTheDocument()
  })
})
