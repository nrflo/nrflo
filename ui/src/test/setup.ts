import '@testing-library/jest-dom/vitest'
import { vi } from 'vitest'

// Mock window.location
Object.defineProperty(window, 'location', {
  writable: true,
  value: {
    protocol: 'http:',
    host: 'localhost:5175',
  },
})

// Mock environment variables
vi.stubEnv('VITE_API_URL', 'http://localhost:6587')

// Default-stub the available-tools query (mounted by the agent tools picker) so
// components don't require a QueryClient in unit tests. Tests asserting on tool
// content override this with their own vi.mock.
vi.mock('@/hooks/useAvailableTools', () => ({
  useAvailableTools: () => ({ data: [], isLoading: false }),
  availableToolKeys: { all: ['available-tools'], list: () => ['available-tools', 'list'] },
}))
