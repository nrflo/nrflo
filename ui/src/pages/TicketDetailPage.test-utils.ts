import { render } from '@testing-library/react'
import { MemoryRouter, Routes, Route } from 'react-router-dom'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { createElement } from 'react'
import { TicketDetailPage } from './TicketDetailPage'
import type { TicketWithDeps } from '@/types/ticket'
import type { WorkflowResponse, AgentSessionsResponse } from '@/types/workflow'

export const sampleTicket: TicketWithDeps = {
  id: 'TICKET-1',
  title: 'Test ticket',
  description: 'A test ticket',
  status: 'in_progress',
  priority: 2,
  issue_type: 'feature',
  created_at: '2026-01-01T00:00:00Z',
  updated_at: '2026-01-01T00:00:00Z',
  closed_at: null,
  created_by: 'ui',
  close_reason: null,
  blockers: [],
  blocks: [],
}

// Workflow with an active phase (agents running)
export const workflowWithActivePhase: WorkflowResponse = {
  ticket_id: 'TICKET-1',
  has_workflow: true,
  state: {
    workflow: 'feature',
    version: 4,
    current_phase: 'implementation',
    category: 'full',
    phase_order: ['investigation', 'implementation', 'verification'],
    phases: {
      investigation: { status: 'completed', result: 'pass' },
      implementation: { status: 'in_progress' },
    },
    active_agents: {
      'implementor:claude:sonnet': {
        agent_id: 'a1',
        agent_type: 'implementor',
        phase: 'implementation',
        model_id: 'claude-sonnet-4-5',
        cli: 'claude',
        pid: 12345,
        started_at: '2026-01-01T00:00:00Z',
      },
    },
  },
  workflows: ['feature'],
  all_workflows: {},
}

// Workflow with no active phase
export const workflowNoActivePhase: WorkflowResponse = {
  ticket_id: 'TICKET-1',
  has_workflow: true,
  state: {
    workflow: 'feature',
    version: 4,
    current_phase: 'implementation',
    phase_order: ['investigation', 'implementation'],
    phases: {
      investigation: { status: 'completed', result: 'pass' },
      implementation: { status: 'completed', result: 'pass' },
    },
    active_agents: {},
  },
  workflows: ['feature'],
  all_workflows: {},
}

// Orchestrated workflow (running via Auto) with active phase
export const workflowOrchestrated: WorkflowResponse = {
  ticket_id: 'TICKET-1',
  has_workflow: true,
  state: {
    workflow: 'feature',
    version: 4,
    current_phase: 'implementation',
    category: 'full',
    phase_order: ['investigation', 'implementation', 'verification'],
    phases: {
      investigation: { status: 'completed', result: 'pass' },
      implementation: { status: 'in_progress' },
    },
    active_agents: {
      'implementor:claude:sonnet': {
        agent_id: 'a1',
        agent_type: 'implementor',
        phase: 'implementation',
        model_id: 'claude-sonnet-4-5',
        cli: 'claude',
        pid: 12345,
        started_at: '2026-01-01T00:00:00Z',
      },
    },
    findings: {
      _orchestration: { status: 'running' },
    },
  },
  workflows: ['feature'],
  all_workflows: {},
}

// Orchestrated workflow with no active agents yet (between phases)
export const workflowOrchestratedNoAgents: WorkflowResponse = {
  ticket_id: 'TICKET-1',
  has_workflow: true,
  state: {
    workflow: 'feature',
    version: 4,
    current_phase: 'implementation',
    category: 'full',
    phase_order: ['investigation', 'implementation', 'verification'],
    phases: {
      investigation: { status: 'completed', result: 'pass' },
    },
    active_agents: {},
    findings: {
      _orchestration: { status: 'running' },
    },
  },
  workflows: ['feature'],
  all_workflows: {},
}

// Workflow with multiple workflows (dropdown selector visible)
export const workflowMultiple: WorkflowResponse = {
  ticket_id: 'TICKET-1',
  has_workflow: true,
  state: {
    workflow: 'feature',
    version: 4,
    current_phase: 'implementation',
    category: 'full',
    phase_order: ['investigation', 'implementation', 'verification'],
    phases: {
      investigation: { status: 'completed', result: 'pass' },
      implementation: { status: 'in_progress' },
    },
    active_agents: {
      'implementor:claude:sonnet': {
        agent_id: 'a1',
        agent_type: 'implementor',
        phase: 'implementation',
        model_id: 'claude-sonnet-4-5',
        cli: 'claude',
        pid: 12345,
        started_at: '2026-01-01T00:00:00Z',
      },
    },
  },
  workflows: ['feature', 'bugfix'],
  all_workflows: {
    feature: {
      workflow: 'feature',
      version: 4,
      current_phase: 'implementation',
      category: 'full',
      phase_order: ['investigation', 'implementation', 'verification'],
      phases: {
        investigation: { status: 'completed', result: 'pass' },
        implementation: { status: 'in_progress' },
      },
      active_agents: {
        'implementor:claude:sonnet': {
          agent_id: 'a1',
          agent_type: 'implementor',
          phase: 'implementation',
          model_id: 'claude-sonnet-4-5',
          cli: 'claude',
          pid: 12345,
          started_at: '2026-01-01T00:00:00Z',
        },
      },
    },
    bugfix: {
      workflow: 'bugfix',
      version: 4,
      current_phase: 'investigation',
      phase_order: ['investigation', 'implementation'],
      phases: {},
      active_agents: {},
    },
  },
}

// Completed workflow with all three stats
export const workflowCompleted: WorkflowResponse = {
  ticket_id: 'TICKET-1',
  has_workflow: true,
  state: {
    workflow: 'feature',
    version: 4,
    status: 'completed',
    completed_at: '2026-01-01T01:30:00Z',
    total_duration_sec: 5400,
    total_tokens_used: 230000,
    current_phase: 'verification',
    phase_order: ['investigation', 'implementation', 'verification'],
    phases: {
      investigation: { status: 'completed', result: 'pass' },
      implementation: { status: 'completed', result: 'pass' },
      verification: { status: 'completed', result: 'pass' },
    },
    active_agents: {},
  },
  workflows: ['feature'],
  all_workflows: {},
}

// Completed workflow with zero tokens (no agents had context_left)
export const workflowCompletedZeroTokens: WorkflowResponse = {
  ticket_id: 'TICKET-1',
  has_workflow: true,
  state: {
    workflow: 'feature',
    version: 4,
    status: 'completed',
    completed_at: '2026-01-01T00:05:00Z',
    total_duration_sec: 300,
    total_tokens_used: 0,
    current_phase: 'investigation',
    phase_order: ['investigation'],
    phases: {
      investigation: { status: 'completed', result: 'pass' },
    },
    active_agents: {},
  },
  workflows: ['feature'],
  all_workflows: {},
}

export const emptySessions: AgentSessionsResponse = {
  ticket_id: 'TICKET-1',
  sessions: [],
}

export function renderPage(ticketId = 'TICKET-1') {
  const queryClient = new QueryClient({
    defaultOptions: {
      queries: { retry: false },
      mutations: { retry: false },
    },
  })
  return render(
    createElement(
      QueryClientProvider,
      { client: queryClient },
      createElement(
        MemoryRouter,
        { initialEntries: [`/tickets/${encodeURIComponent(ticketId)}`] },
        createElement(
          Routes,
          null,
          createElement(Route, { path: '/tickets/:id', element: createElement(TicketDetailPage) })
        )
      )
    )
  )
}
