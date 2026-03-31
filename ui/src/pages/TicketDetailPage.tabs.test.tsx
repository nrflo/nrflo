import { describe, it, expect, vi, beforeEach } from 'vitest'
import { screen, waitFor } from '@testing-library/react'
import * as ticketsApi from '@/api/tickets'
import {
  sampleTicket,
  workflowNoActivePhase,
  emptySessions,
  renderPage,
} from './TicketDetailPage.test-utils'

vi.mock('@/stores/projectStore', () => ({
  useProjectStore: (selector: (s: { currentProject: string; projectsLoaded: boolean }) => unknown) =>
    selector({ currentProject: 'test-project', projectsLoaded: true }),
}))

vi.mock('@/hooks/useWebSocketSubscription', () => ({
  useWebSocketSubscription: () => ({ isConnected: true }),
}))

vi.mock('@/pages/WorkflowTabContent', () => ({
  WorkflowTabContent: () => <div data-testid="workflow-tab-content">WorkflowTabContent</div>,
}))

vi.mock('@/pages/HierarchyTabContent', () => ({
  HierarchyTabContent: () => <div data-testid="hierarchy-tab-content">HierarchyTabContent</div>,
}))

vi.mock('@/pages/DescriptionTabContent', () => ({
  DescriptionTabContent: () => <div data-testid="description-tab-content">DescriptionTabContent</div>,
}))

vi.mock('@/pages/DetailsTabContent', () => ({
  DetailsTabContent: () => <div data-testid="details-tab-content">DetailsTabContent</div>,
}))

vi.mock('@/components/workflow/RunWorkflowDialog', () => ({
  RunWorkflowDialog: () => null,
}))

vi.mock('@/components/workflow/RunEpicWorkflowDialog', () => ({
  RunEpicWorkflowDialog: () => null,
}))

vi.mock('@/hooks/useChains', () => ({
  useChainList: () => ({ data: [] }),
}))

vi.mock('@/api/tickets', async () => {
  const actual = await vi.importActual('@/api/tickets')
  return {
    ...actual,
    getTicket: vi.fn(),
    getWorkflow: vi.fn(),
    getAgentSessions: vi.fn(),
    closeTicket: vi.fn(),
    deleteTicket: vi.fn(),
  }
})

vi.mock('@/api/workflows', () => ({
  runWorkflow: vi.fn(),
  stopWorkflow: vi.fn(),
}))

describe('TicketDetailPage - tab behavior', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.mocked(ticketsApi.getTicket).mockResolvedValue(sampleTicket)
    vi.mocked(ticketsApi.getWorkflow).mockResolvedValue(workflowNoActivePhase)
    vi.mocked(ticketsApi.getAgentSessions).mockResolvedValue(emptySessions)
  })

  describe('tab order', () => {
    it('renders Workflow as the first tab', async () => {
      renderPage()
      await waitFor(() => expect(screen.getByText('Test ticket')).toBeInTheDocument())

      const tabs = screen.getAllByRole('button', { name: /^(Workflow|Hierarchy|Description|Details)$/ })
      expect(tabs[0]).toHaveTextContent('Workflow')
    })

    it('renders tabs in order: Workflow, Hierarchy, Description, Details', async () => {
      renderPage()
      await waitFor(() => expect(screen.getByText('Test ticket')).toBeInTheDocument())

      const tabs = screen.getAllByRole('button', { name: /^(Workflow|Hierarchy|Description|Details)$/ })
      const labels = tabs.map(t => t.textContent?.trim())
      expect(labels).toEqual(['Workflow', 'Hierarchy', 'Description', 'Details'])
    })
  })

  describe('default tab (no query param)', () => {
    it('shows Workflow tab content by default', async () => {
      renderPage()
      await waitFor(() => expect(screen.getByText('Test ticket')).toBeInTheDocument())

      await waitFor(() => {
        expect(screen.getByTestId('workflow-tab-content')).toBeInTheDocument()
      })
      expect(screen.queryByTestId('hierarchy-tab-content')).not.toBeInTheDocument()
    })
  })

  describe('?tab=hierarchy query param', () => {
    it('shows Hierarchy tab content when ?tab=hierarchy', async () => {
      renderPage('TICKET-1', '?tab=hierarchy')
      await waitFor(() => expect(screen.getByText('Test ticket')).toBeInTheDocument())

      await waitFor(() => {
        expect(screen.getByTestId('hierarchy-tab-content')).toBeInTheDocument()
      })
      expect(screen.queryByTestId('workflow-tab-content')).not.toBeInTheDocument()
    })
  })

  describe('?tab=workflow query param', () => {
    it('shows Workflow tab content when ?tab=workflow', async () => {
      renderPage('TICKET-1', '?tab=workflow')
      await waitFor(() => expect(screen.getByText('Test ticket')).toBeInTheDocument())

      await waitFor(() => {
        expect(screen.getByTestId('workflow-tab-content')).toBeInTheDocument()
      })
      expect(screen.queryByTestId('hierarchy-tab-content')).not.toBeInTheDocument()
    })
  })

  describe('invalid tab param', () => {
    it('falls back to Workflow for an unknown tab value', async () => {
      renderPage('TICKET-1', '?tab=bogus')
      await waitFor(() => expect(screen.getByText('Test ticket')).toBeInTheDocument())

      await waitFor(() => {
        expect(screen.getByTestId('workflow-tab-content')).toBeInTheDocument()
      })
      expect(screen.queryByTestId('hierarchy-tab-content')).not.toBeInTheDocument()
    })
  })
})
