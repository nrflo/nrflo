import { useState, useRef, useEffect, type ReactNode } from 'react'
import { Link } from 'react-router-dom'
import { Input } from '@/components/ui/Input'
import { Spinner } from '@/components/ui/Spinner'
import { cn } from '@/lib/utils'
import { NotConfiguredError } from '@/api/specImport'

interface NotConfiguredInfo {
  missing: string[]
  settingsHref: string
}

interface IssueSearchComboProps<Result> {
  placeholder: string
  search: (q: string) => Promise<Result[]>
  renderItem: (r: Result) => ReactNode
  onSelect: (r: Result) => void
  notConfigured?: NotConfiguredInfo
  onNotConfigured?: (missing: string[]) => void
}

// TODO(test-writer): vitest+RTL tests for IssueSearchCombo — (1) debounce: type "abc", advance 200ms asserts search NOT called, advance 50ms asserts called once; (2) min-chars: type "a", advance 300ms, assert search not called; (3) select: resolved results, click item, assert onSelect; (4) not-configured: pass notConfigured prop, assert inline row with settingsHref link
export function IssueSearchCombo<Result>({
  placeholder,
  search,
  renderItem,
  onSelect,
  notConfigured,
  onNotConfigured,
}: IssueSearchComboProps<Result>) {
  const [query, setQuery] = useState('')
  const [results, setResults] = useState<Result[]>([])
  const [pending, setPending] = useState(false)
  const [open, setOpen] = useState(false)
  const debounceRef = useRef<ReturnType<typeof setTimeout> | null>(null)
  const containerRef = useRef<HTMLDivElement>(null)

  useEffect(() => {
    if (!open) return

    function handleClickOutside(e: MouseEvent) {
      if (containerRef.current && !containerRef.current.contains(e.target as Node)) {
        setOpen(false)
      }
    }

    function handleEscape(e: KeyboardEvent) {
      if (e.key === 'Escape') setOpen(false)
    }

    document.addEventListener('mousedown', handleClickOutside)
    document.addEventListener('keydown', handleEscape)
    return () => {
      document.removeEventListener('mousedown', handleClickOutside)
      document.removeEventListener('keydown', handleEscape)
    }
  }, [open])

  function handleQueryChange(value: string) {
    setQuery(value)
    setOpen(false)
    setResults([])

    if (debounceRef.current) clearTimeout(debounceRef.current)

    if (value.trim().length < 2) return

    debounceRef.current = setTimeout(async () => {
      setPending(true)
      try {
        const data = await search(value.trim())
        setResults(data)
        setOpen(data.length > 0)
      } catch (e) {
        if (e instanceof NotConfiguredError) {
          onNotConfigured?.(e.missing)
        }
      } finally {
        setPending(false)
      }
    }, 250)
  }

  function handleSelect(result: Result) {
    onSelect(result)
    setOpen(false)
    setResults([])
  }

  const showNotConfigured = notConfigured != null

  return (
    <div ref={containerRef} className="relative w-full">
      <div className="relative">
        <Input
          placeholder={placeholder}
          value={query}
          onChange={(e) => handleQueryChange(e.target.value)}
        />
        {pending && (
          <div className="absolute right-3 top-1/2 -translate-y-1/2">
            <Spinner size="sm" />
          </div>
        )}
      </div>

      {showNotConfigured && (
        <div className="mt-2 rounded-md border border-amber-300 bg-amber-50 dark:bg-amber-950/30 dark:border-amber-700 px-3 py-2 text-sm text-amber-800 dark:text-amber-300">
          Missing environment variables:{' '}
          <span className="font-mono">{notConfigured.missing.join(', ')}</span>.{' '}
          <Link
            to={notConfigured.settingsHref}
            className="underline hover:no-underline font-medium"
          >
            Configure in project settings
          </Link>
        </div>
      )}

      {open && results.length > 0 && (
        <div
          className={cn(
            'absolute left-0 top-full mt-1 w-full rounded-md border border-border bg-background shadow-lg z-50',
            'max-h-64 overflow-y-auto'
          )}
        >
          {results.map((result, i) => (
            <div
              key={i}
              onClick={() => handleSelect(result)}
              className="cursor-pointer px-3 py-2 text-sm hover:bg-muted transition-colors"
            >
              {renderItem(result)}
            </div>
          ))}
        </div>
      )}
    </div>
  )
}
