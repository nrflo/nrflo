import '@testing-library/jest-dom/vitest'
import { cleanup } from '@testing-library/react'
import { afterEach, vi } from 'vitest'

// Cleanup after each test
afterEach(() => {
  cleanup()
})

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
