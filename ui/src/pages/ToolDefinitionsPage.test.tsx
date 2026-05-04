import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { ToolDefinitionsPage } from './ToolDefinitionsPage'
import type { ToolDefinition } from '@/types/toolDefinition'

const mockUseIsAdmin = vi.fn().mockReturnValue(true)

vi.mock('@/stores/authStore', () => ({
  useIsAdmin: () => mockUseIsAdmin(),
}))

vi.mock('@/hooks/useToolDefinitions', () => ({
  useToolDefinitions: vi.fn(),
  useCreateToolDefinition: vi.fn(),
  useUpdateToolDefinition: vi.fn(),
  useDeleteToolDefinition: vi.fn(),
}))

vi.mock('@/hooks/useProjects', () => ({
  useProjects: vi.fn(),
}))

import {
  useToolDefinitions,
  useCreateToolDefinition,
  useUpdateToolDefinition,
  useDeleteToolDefinition,
} from '@/hooks/useToolDefinitions'
import { useProjects } from '@/hooks/useProjects'

function makeToolDef(overrides: Partial<ToolDefinition> = {}): ToolDefinition {
  return {
    id: 'tool-1',
    name: 'fetch-weather',
    description: 'Gets weather data',
    input_schema: '{"type":"object"}',
    endpoint: 'https://api.example.com/weather',
    auth_method: 'none',
    timeout_sec: 30,
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:00:00Z',
    ...overrides,
  }
}

const mockMutate = vi.fn()

function setupMocks(toolDefs: ToolDefinition[] = []) {
  vi.mocked(useToolDefinitions).mockReturnValue({ data: toolDefs, isLoading: false, error: null } as unknown as ReturnType<typeof useToolDefinitions>)
  vi.mocked(useCreateToolDefinition).mockReturnValue({ mutate: mockMutate, isPending: false } as unknown as ReturnType<typeof useCreateToolDefinition>)
  vi.mocked(useUpdateToolDefinition).mockReturnValue({ mutate: vi.fn(), isPending: false } as unknown as ReturnType<typeof useUpdateToolDefinition>)
  vi.mocked(useDeleteToolDefinition).mockReturnValue({ mutate: mockMutate, isPending: false } as unknown as ReturnType<typeof useDeleteToolDefinition>)
  vi.mocked(useProjects).mockReturnValue({ data: { projects: [{ id: 'proj-a', name: 'Alpha Project' }] } } as unknown as ReturnType<typeof useProjects>)
}

beforeEach(() => {
  vi.clearAllMocks()
  mockUseIsAdmin.mockReturnValue(true)
})

describe('ToolDefinitionsPage', () => {
  describe('list rendering', () => {
    it('renders tool definitions rows with name and auth_method badge', () => {
      setupMocks([makeToolDef(), makeToolDef({ id: 'tool-2', name: 'send-email', auth_method: 'bearer_env' })])
      render(<ToolDefinitionsPage />)

      expect(screen.getByText('fetch-weather')).toBeInTheDocument()
      expect(screen.getByText('send-email')).toBeInTheDocument()
      expect(screen.getAllByText('none')).toHaveLength(1)
      expect(screen.getByText('bearer_env')).toBeInTheDocument()
    })

    it('shows endpoint URL in the row', () => {
      setupMocks([makeToolDef()])
      render(<ToolDefinitionsPage />)
      expect(screen.getByText('https://api.example.com/weather')).toBeInTheDocument()
    })

    it('resolves project name from project_id', () => {
      setupMocks([makeToolDef({ project_id: 'proj-a' })])
      render(<ToolDefinitionsPage />)
      expect(screen.getByText(/Alpha Project/)).toBeInTheDocument()
    })

    it('shows Global when no project_id', () => {
      setupMocks([makeToolDef()])
      render(<ToolDefinitionsPage />)
      expect(screen.getByText(/Global/)).toBeInTheDocument()
    })

    it('shows empty state message when no tool defs', () => {
      setupMocks([])
      render(<ToolDefinitionsPage />)
      expect(screen.getByText('No tool definitions yet.')).toBeInTheDocument()
    })

    it('shows loading text while loading', () => {
      vi.mocked(useToolDefinitions).mockReturnValue({ data: [], isLoading: true, error: null } as unknown as ReturnType<typeof useToolDefinitions>)
      vi.mocked(useCreateToolDefinition).mockReturnValue({ mutate: vi.fn(), isPending: false } as unknown as ReturnType<typeof useCreateToolDefinition>)
      vi.mocked(useUpdateToolDefinition).mockReturnValue({ mutate: vi.fn(), isPending: false } as unknown as ReturnType<typeof useUpdateToolDefinition>)
      vi.mocked(useDeleteToolDefinition).mockReturnValue({ mutate: vi.fn(), isPending: false } as unknown as ReturnType<typeof useDeleteToolDefinition>)
      vi.mocked(useProjects).mockReturnValue({ data: undefined } as unknown as ReturnType<typeof useProjects>)
      render(<ToolDefinitionsPage />)
      expect(screen.getByText(/Loading/i)).toBeInTheDocument()
    })
  })

  describe('create form', () => {
    it('opens create form when New Tool Definition is clicked', async () => {
      const user = userEvent.setup()
      setupMocks([])
      render(<ToolDefinitionsPage />)

      await user.click(screen.getByRole('button', { name: /New Tool Definition/i }))

      expect(screen.getByPlaceholderText('my-tool-name')).toBeInTheDocument()
      expect(screen.getByPlaceholderText('https://example.com/api/tool')).toBeInTheDocument()
    })

    it('shows inline JSON validation error for invalid input_schema', async () => {
      const user = userEvent.setup()
      setupMocks([])
      render(<ToolDefinitionsPage />)

      await user.click(screen.getByRole('button', { name: /New Tool Definition/i }))

      const schemaTextarea = screen.getByPlaceholderText('{"type":"object","properties":{}}')
      await user.type(schemaTextarea, 'not-valid-json')

      expect(screen.getByText('Invalid JSON')).toBeInTheDocument()
    })

    it('disables Create button when input_schema has JSON error', async () => {
      const user = userEvent.setup()
      setupMocks([])
      render(<ToolDefinitionsPage />)

      await user.click(screen.getByRole('button', { name: /New Tool Definition/i }))
      await user.type(screen.getByPlaceholderText('{"type":"object","properties":{}}'), 'bad-json')

      expect(screen.getByRole('button', { name: /^Create$/i })).toBeDisabled()
    })

    it('clears JSON error when schema textarea is cleared', async () => {
      const user = userEvent.setup()
      setupMocks([])
      render(<ToolDefinitionsPage />)

      await user.click(screen.getByRole('button', { name: /New Tool Definition/i }))
      const schemaTextarea = screen.getByPlaceholderText('{"type":"object","properties":{}}')
      await user.type(schemaTextarea, 'bad')
      expect(screen.getByText('Invalid JSON')).toBeInTheDocument()

      // Empty string bypasses JSON.parse and clears the error
      await user.clear(schemaTextarea)
      expect(screen.queryByText('Invalid JSON')).not.toBeInTheDocument()
    })

    it('cancels create form when Cancel is clicked', async () => {
      const user = userEvent.setup()
      setupMocks([])
      render(<ToolDefinitionsPage />)

      await user.click(screen.getByRole('button', { name: /New Tool Definition/i }))
      expect(screen.getByPlaceholderText('my-tool-name')).toBeInTheDocument()

      await user.click(screen.getByRole('button', { name: /^Cancel$/i }))
      expect(screen.queryByPlaceholderText('my-tool-name')).not.toBeInTheDocument()
    })
  })

  describe('delete flow', () => {
    it('opens ConfirmDialog when delete icon is clicked', async () => {
      const user = userEvent.setup()
      setupMocks([makeToolDef()])
      render(<ToolDefinitionsPage />)

      const row = screen.getByText('fetch-weather').closest('.border') as HTMLElement
      const buttons = within(row).getAllByRole('button')
      await user.click(buttons[buttons.length - 1])

      expect(screen.getByText('Delete Tool Definition')).toBeInTheDocument()
      expect(screen.getByText(/Delete tool definition 'tool-1'/)).toBeInTheDocument()
    })

    it('calls deleteToolDefinition.mutate with tool id on confirm', async () => {
      const user = userEvent.setup()
      const deleteMutateSpy = vi.fn()
      vi.mocked(useDeleteToolDefinition).mockReturnValue({ mutate: deleteMutateSpy, isPending: false } as unknown as ReturnType<typeof useDeleteToolDefinition>)
      setupMocks([makeToolDef()])
      vi.mocked(useDeleteToolDefinition).mockReturnValue({ mutate: deleteMutateSpy, isPending: false } as unknown as ReturnType<typeof useDeleteToolDefinition>)
      render(<ToolDefinitionsPage />)

      const row = screen.getByText('fetch-weather').closest('.border') as HTMLElement
      const buttons = within(row).getAllByRole('button')
      await user.click(buttons[buttons.length - 1])

      await user.click(screen.getByRole('button', { name: /^Delete$/i }))

      expect(deleteMutateSpy).toHaveBeenCalledWith(
        'tool-1',
        expect.objectContaining({ onSuccess: expect.any(Function) })
      )
    })

    it('closes dialog when Cancel is clicked in confirm dialog', async () => {
      const user = userEvent.setup()
      setupMocks([makeToolDef()])
      render(<ToolDefinitionsPage />)

      const row = screen.getByText('fetch-weather').closest('.border') as HTMLElement
      const buttons = within(row).getAllByRole('button')
      await user.click(buttons[buttons.length - 1])

      expect(screen.getByText('Delete Tool Definition')).toBeInTheDocument()
      await user.click(screen.getByRole('button', { name: /^Cancel$/i }))
      expect(screen.queryByText('Delete Tool Definition')).not.toBeInTheDocument()
    })
  })

  describe('viewer role (isAdmin=false)', () => {
    beforeEach(() => {
      mockUseIsAdmin.mockReturnValue(false)
    })

    it('hides New Tool Definition button', () => {
      setupMocks([])
      render(<ToolDefinitionsPage />)
      expect(screen.queryByRole('button', { name: /New Tool Definition/i })).not.toBeInTheDocument()
    })

    it('hides edit and delete buttons for existing definitions', () => {
      setupMocks([makeToolDef()])
      render(<ToolDefinitionsPage />)
      const row = screen.getByText('fetch-weather').closest('.border') as HTMLElement
      expect(within(row).queryAllByRole('button')).toHaveLength(0)
    })

    it('shows ReadOnlyHint banner', () => {
      setupMocks([])
      render(<ToolDefinitionsPage />)
      expect(screen.getByText('Read-only — admin required to make changes.')).toBeInTheDocument()
    })

    it('still renders tool definition rows', () => {
      setupMocks([makeToolDef(), makeToolDef({ id: 'tool-2', name: 'send-sms' })])
      render(<ToolDefinitionsPage />)
      expect(screen.getByText('fetch-weather')).toBeInTheDocument()
      expect(screen.getByText('send-sms')).toBeInTheDocument()
    })
  })
})
