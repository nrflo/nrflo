import { describe, it, expect, vi, beforeEach } from 'vitest'
import { screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { ProjectCaptureThinkingEditor } from './ProjectCaptureThinkingEditor'
import * as hooks from '@/hooks/useProjectSettings'
import { renderWithQuery } from '@/test/utils'

vi.mock('@/hooks/useProjectSettings', () => ({
  useCaptureThinking: vi.fn(),
  useSetCaptureThinking: vi.fn(),
}))

const PROJECT_ID = 'proj-1'

function makeMutation(overrides: Partial<{ mutate: ReturnType<typeof vi.fn>; isPending: boolean; isError: boolean; error: Error | null }> = {}) {
  return { mutate: vi.fn(), isPending: false, isError: false, error: null, ...overrides }
}

beforeEach(() => vi.clearAllMocks())

describe('ProjectCaptureThinkingEditor', () => {
  it('renders section heading', () => {
    vi.mocked(hooks.useCaptureThinking).mockReturnValue({ data: { enabled: false, inherited: true } } as any)
    vi.mocked(hooks.useSetCaptureThinking).mockReturnValue(makeMutation() as any)
    renderWithQuery(<ProjectCaptureThinkingEditor projectId={PROJECT_ID} />)
    expect(screen.getByText('Capture Model Thinking')).toBeInTheDocument()
  })

  it('shows "Inherit (global)" when data.inherited is true', () => {
    vi.mocked(hooks.useCaptureThinking).mockReturnValue({ data: { enabled: false, inherited: true } } as any)
    vi.mocked(hooks.useSetCaptureThinking).mockReturnValue(makeMutation() as any)
    renderWithQuery(<ProjectCaptureThinkingEditor projectId={PROJECT_ID} />)
    expect(screen.getByRole('button')).toHaveTextContent('Inherit (global)')
  })

  it('shows "On" when inherited is false and enabled is true', () => {
    vi.mocked(hooks.useCaptureThinking).mockReturnValue({ data: { enabled: true, inherited: false } } as any)
    vi.mocked(hooks.useSetCaptureThinking).mockReturnValue(makeMutation() as any)
    renderWithQuery(<ProjectCaptureThinkingEditor projectId={PROJECT_ID} />)
    expect(screen.getByRole('button')).toHaveTextContent('On')
  })

  it('shows "Off" when inherited is false and enabled is false', () => {
    vi.mocked(hooks.useCaptureThinking).mockReturnValue({ data: { enabled: false, inherited: false } } as any)
    vi.mocked(hooks.useSetCaptureThinking).mockReturnValue(makeMutation() as any)
    renderWithQuery(<ProjectCaptureThinkingEditor projectId={PROJECT_ID} />)
    expect(screen.getByRole('button')).toHaveTextContent('Off')
  })

  it('defaults to "Inherit (global)" when data is undefined (loading)', () => {
    vi.mocked(hooks.useCaptureThinking).mockReturnValue({ data: undefined } as any)
    vi.mocked(hooks.useSetCaptureThinking).mockReturnValue(makeMutation() as any)
    renderWithQuery(<ProjectCaptureThinkingEditor projectId={PROJECT_ID} />)
    expect(screen.getByRole('button')).toHaveTextContent('Inherit (global)')
  })

  it('shows "Global setting: On" helper when inherited and global is enabled', () => {
    vi.mocked(hooks.useCaptureThinking).mockReturnValue({ data: { enabled: true, inherited: true } } as any)
    vi.mocked(hooks.useSetCaptureThinking).mockReturnValue(makeMutation() as any)
    renderWithQuery(<ProjectCaptureThinkingEditor projectId={PROJECT_ID} />)
    expect(screen.getByText('Global setting: On')).toBeInTheDocument()
  })

  it('shows "Global setting: Off" helper when inherited and global is disabled', () => {
    vi.mocked(hooks.useCaptureThinking).mockReturnValue({ data: { enabled: false, inherited: true } } as any)
    vi.mocked(hooks.useSetCaptureThinking).mockReturnValue(makeMutation() as any)
    renderWithQuery(<ProjectCaptureThinkingEditor projectId={PROJECT_ID} />)
    expect(screen.getByText('Global setting: Off')).toBeInTheDocument()
  })

  it('hides helper text when not inherited', () => {
    vi.mocked(hooks.useCaptureThinking).mockReturnValue({ data: { enabled: true, inherited: false } } as any)
    vi.mocked(hooks.useSetCaptureThinking).mockReturnValue(makeMutation() as any)
    renderWithQuery(<ProjectCaptureThinkingEditor projectId={PROJECT_ID} />)
    expect(screen.queryByText(/global setting:/i)).not.toBeInTheDocument()
  })

  it('selecting "On" calls mutate with enabled: true', async () => {
    const mutate = vi.fn()
    vi.mocked(hooks.useCaptureThinking).mockReturnValue({ data: { enabled: false, inherited: true } } as any)
    vi.mocked(hooks.useSetCaptureThinking).mockReturnValue(makeMutation({ mutate }) as any)
    renderWithQuery(<ProjectCaptureThinkingEditor projectId={PROJECT_ID} />)

    const user = userEvent.setup()
    await user.click(screen.getByRole('button'))
    await user.click(screen.getByText('On'))

    expect(mutate).toHaveBeenCalledWith({ projectId: PROJECT_ID, enabled: true })
  })

  it('selecting "Off" calls mutate with enabled: false', async () => {
    const mutate = vi.fn()
    vi.mocked(hooks.useCaptureThinking).mockReturnValue({ data: { enabled: true, inherited: false } } as any)
    vi.mocked(hooks.useSetCaptureThinking).mockReturnValue(makeMutation({ mutate }) as any)
    renderWithQuery(<ProjectCaptureThinkingEditor projectId={PROJECT_ID} />)

    const user = userEvent.setup()
    await user.click(screen.getByRole('button'))
    await user.click(screen.getByText('Off'))

    expect(mutate).toHaveBeenCalledWith({ projectId: PROJECT_ID, enabled: false })
  })

  it('selecting "Inherit (global)" calls mutate with enabled: null', async () => {
    const mutate = vi.fn()
    vi.mocked(hooks.useCaptureThinking).mockReturnValue({ data: { enabled: true, inherited: false } } as any)
    vi.mocked(hooks.useSetCaptureThinking).mockReturnValue(makeMutation({ mutate }) as any)
    renderWithQuery(<ProjectCaptureThinkingEditor projectId={PROJECT_ID} />)

    const user = userEvent.setup()
    await user.click(screen.getByRole('button'))
    await user.click(screen.getByText('Inherit (global)'))

    expect(mutate).toHaveBeenCalledWith({ projectId: PROJECT_ID, enabled: null })
  })

  it('displays mutation error message', () => {
    vi.mocked(hooks.useCaptureThinking).mockReturnValue({ data: { enabled: false, inherited: true } } as any)
    vi.mocked(hooks.useSetCaptureThinking).mockReturnValue(
      makeMutation({ isError: true, error: new Error('save failed') }) as any
    )
    renderWithQuery(<ProjectCaptureThinkingEditor projectId={PROJECT_ID} />)
    expect(screen.getByText('save failed')).toBeInTheDocument()
  })
})
