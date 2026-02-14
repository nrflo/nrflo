import { describe, it, expect } from 'vitest'
import { readFileSync, readdirSync, statSync } from 'fs'
import { join } from 'path'

/**
 * Guard test to prevent components from directly instantiating WebSocket.
 * Only WebSocketProvider (via useWebSocket hook) should create sockets.
 */

const SRC_DIR = join(__dirname, '..')
const ALLOWLIST = [
  // Test files can instantiate mock WebSockets
  '.test.ts',
  '.test.tsx',
  // This guard file itself
  'single-socket-guard.test.ts',
  // useWebSocket hook is the ONLY file allowed to create WebSocket
  'hooks/useWebSocket.ts',
]

function getAllFiles(dir: string, files: string[] = []): string[] {
  const entries = readdirSync(dir)

  for (const entry of entries) {
    const fullPath = join(dir, entry)
    const stat = statSync(fullPath)

    if (stat.isDirectory()) {
      if (!['node_modules', 'dist', '.git', 'coverage'].includes(entry)) {
        getAllFiles(fullPath, files)
      }
    } else if (stat.isFile() && (entry.endsWith('.ts') || entry.endsWith('.tsx'))) {
      files.push(fullPath)
    }
  }

  return files
}

describe('Single-Socket Guard', () => {
  it('should only instantiate WebSocket in useWebSocket hook', () => {
    const allFiles = getAllFiles(SRC_DIR)
    const violations: { file: string; lines: string[] }[] = []

    for (const file of allFiles) {
      // Skip allowlisted files
      if (ALLOWLIST.some(pattern => file.includes(pattern))) {
        continue
      }

      const content = readFileSync(file, 'utf-8')
      const lines = content.split('\n')

      const matchingLines: string[] = []
      lines.forEach((line, idx) => {
        // Skip comments and imports
        const trimmed = line.trim()
        if (trimmed.startsWith('//') || trimmed.startsWith('*') || trimmed.startsWith('import')) {
          return
        }

        // Look for 'new WebSocket(' pattern
        if (/new\s+WebSocket\s*\(/.test(line)) {
          matchingLines.push(`Line ${idx + 1}: ${line.trim()}`)
        }
      })

      if (matchingLines.length > 0) {
        violations.push({
          file: file.replace(SRC_DIR, 'ui/src'),
          lines: matchingLines,
        })
      }
    }

    if (violations.length > 0) {
      const report = violations
        .map(v => `${v.file}:\n  ${v.lines.join('\n  ')}`)
        .join('\n\n')

      expect.fail(
        `Found direct WebSocket instantiation outside useWebSocket hook (violates M2):\n\n${report}\n\nUse WebSocketProvider + useWebSocketSubscription instead.`
      )
    }

    expect(violations.length).toBe(0)
  })

  it('verifies that useWebSocket hook can create WebSocket', () => {
    const useWebSocketFile = join(SRC_DIR, 'hooks', 'useWebSocket.ts')
    const content = readFileSync(useWebSocketFile, 'utf-8')

    // useWebSocket.ts should contain 'new WebSocket('
    expect(content).toMatch(/new\s+WebSocket\s*\(/)
  })

  it('verifies that components use useWebSocketSubscription not useWebSocket', () => {
    const violations: { file: string; lines: string[] }[] = []
    const allFiles = getAllFiles(SRC_DIR)

    // Components and pages should NOT directly import/use useWebSocket
    const componentFiles = allFiles.filter(
      f =>
        (f.includes('/components/') || f.includes('/pages/')) &&
        !f.includes('.test.') &&
        !f.includes('WebSocketProvider')
    )

    for (const file of componentFiles) {
      const content = readFileSync(file, 'utf-8')
      const lines = content.split('\n')

      const matchingLines: string[] = []
      lines.forEach((line, idx) => {
        // Look for direct useWebSocket import/usage (not useWebSocketSubscription)
        if (
          /import.*useWebSocket.*from.*useWebSocket/.test(line) ||
          /const.*=.*useWebSocket\(/.test(line)
        ) {
          // Make sure it's not useWebSocketSubscription
          if (!line.includes('useWebSocketSubscription')) {
            matchingLines.push(`Line ${idx + 1}: ${line.trim()}`)
          }
        }
      })

      if (matchingLines.length > 0) {
        violations.push({
          file: file.replace(SRC_DIR, 'ui/src'),
          lines: matchingLines,
        })
      }
    }

    if (violations.length > 0) {
      const report = violations
        .map(v => `${v.file}:\n  ${v.lines.join('\n  ')}`)
        .join('\n\n')

      expect.fail(
        `Components should use useWebSocketSubscription, not useWebSocket directly:\n\n${report}`
      )
    }

    expect(violations.length).toBe(0)
  })
})
