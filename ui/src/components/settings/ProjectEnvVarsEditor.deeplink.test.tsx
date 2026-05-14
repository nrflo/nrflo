import { describe, it, expect, vi, beforeEach } from 'vitest'
import { screen } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { ProjectEnvVarsEditor } from './ProjectEnvVarsEditor'
import * as api from '@/api/projectEnvVars'
import * as catalogHook from '@/hooks/useEnvVarCatalog'
import { renderWithQuery } from '@/test/utils'

vi.mock('@/api/projectEnvVars')
vi.mock('@/hooks/useEnvVarCatalog')

const PROJECT_ID = 'proj-1'

beforeEach(() => {
  vi.clearAllMocks()
  vi.mocked(api.listEnvVars).mockResolvedValue([])
  vi.mocked(catalogHook.useEnvVarCatalog).mockReturnValue({ data: [], isLoading: false } as any)
  Element.prototype.scrollIntoView = vi.fn()
})

function renderEditor(path: string) {
  return renderWithQuery(
    <MemoryRouter initialEntries={[path]}>
      <ProjectEnvVarsEditor projectId={PROJECT_ID} />
    </MemoryRouter>
  )
}

describe('ProjectEnvVarsEditor deep-link', () => {
  it('wrapper div has id="env-vars"', async () => {
    renderEditor('/settings?tab=projects&project=proj-1')
    await screen.findByPlaceholderText('VAR_NAME')
    expect(document.getElementById('env-vars')).toBeInTheDocument()
  })

  it('calls scrollIntoView on mount when hash is #env-vars', async () => {
    const scrollMock = vi.fn()
    Element.prototype.scrollIntoView = scrollMock

    renderEditor('/settings?tab=projects&project=proj-1#env-vars')
    await screen.findByPlaceholderText('VAR_NAME')

    expect(scrollMock).toHaveBeenCalledWith({ behavior: 'smooth', block: 'start' })
  })

  it('does not call scrollIntoView when hash is absent', async () => {
    const scrollMock = vi.fn()
    Element.prototype.scrollIntoView = scrollMock

    renderEditor('/settings?tab=projects&project=proj-1')
    await screen.findByPlaceholderText('VAR_NAME')

    expect(scrollMock).not.toHaveBeenCalled()
  })

  it('does not call scrollIntoView for a different hash', async () => {
    const scrollMock = vi.fn()
    Element.prototype.scrollIntoView = scrollMock

    renderEditor('/settings?tab=projects&project=proj-1#other-section')
    await screen.findByPlaceholderText('VAR_NAME')

    expect(scrollMock).not.toHaveBeenCalled()
  })
})
