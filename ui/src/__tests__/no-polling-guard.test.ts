import { describe, it, expect } from 'vitest'
import { readFileSync, readdirSync, statSync } from 'fs'
import { join } from 'path'

/**
 * Guard test to prevent reintroduction of runtime polling.
 * This test scans source files for refetchInterval to ensure zero polling.
 */

const SRC_DIR = join(__dirname, '..')
const ALLOWLIST = [
  // Test files are allowed to have refetchInterval in test data
  '.test.ts',
  '.test.tsx',
  // This guard file itself
  'no-polling-guard.test.ts',
  // Logs hook polls external log files (not workflow data) — no WebSocket events available
  'useLogs.ts',
]

function getAllFiles(dir: string, files: string[] = []): string[] {
  const entries = readdirSync(dir)

  for (const entry of entries) {
    const fullPath = join(dir, entry)
    const stat = statSync(fullPath)

    if (stat.isDirectory()) {
      // Skip node_modules, dist, .git
      if (!['node_modules', 'dist', '.git', 'coverage'].includes(entry)) {
        getAllFiles(fullPath, files)
      }
    } else if (stat.isFile() && (entry.endsWith('.ts') || entry.endsWith('.tsx'))) {
      files.push(fullPath)
    }
  }

  return files
}

describe('No-Polling Guard', () => {
  it('should not have refetchInterval in production source files', () => {
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
        // Skip comments
        const trimmed = line.trim()
        if (trimmed.startsWith('//') || trimmed.startsWith('*')) {
          return
        }

        if (line.includes('refetchInterval')) {
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
        `Found refetchInterval in production code (polling forbidden per M4):\n\n${report}\n\nRemove all runtime polling.`
      )
    }

    // If we get here, no violations
    expect(violations.length).toBe(0)
  })

  it('verifies that test files can still reference refetchInterval', () => {
    // This test file itself should be allowed
    const thisFile = readFileSync(__filename, 'utf-8')
    expect(thisFile).toContain('refetchInterval')
  })
})
