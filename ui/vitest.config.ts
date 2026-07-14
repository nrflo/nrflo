import { defineConfig } from 'vitest/config'
import react from '@vitejs/plugin-react'
import path from 'path'

const nodeTests = [
  'src/api/!(client).test.ts',
  'src/lib/**/*.test.ts',
  'src/components/workflow/{AgentLogDetail.formatTime,normalizeApiMessages}.test.ts',
  'src/components/workflow/PhaseGraph/layout*.test.ts',
  'src/components/workflow/Trace/{timeScale,useTraceZoom}.test.ts',
]

export default defineConfig({
  plugins: [react()],
  resolve: {
    alias: {
      '@': path.resolve(__dirname, './src'),
    },
  },
  test: {
    globals: true,
    pool: 'threads',
    projects: [
      {
        extends: true,
        test: {
          name: 'node',
          include: nodeTests,
          environment: 'node',
          setupFiles: ['./src/test/setup.node.ts'],
        },
      },
      {
        extends: true,
        test: {
          name: 'dom',
          include: ['src/**/*.{test,spec}.{ts,tsx}'],
          exclude: nodeTests,
          environment: 'jsdom',
          setupFiles: ['./src/test/setup.ts'],
        },
      },
    ],
  },
})
