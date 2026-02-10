import { cn } from '@/lib/utils'
import { FolderOpen, ChevronDown, Check } from 'lucide-react'
import { useState, useRef, useEffect } from 'react'

interface ProjectOption {
  id: string
  name: string
}

interface ProjectSelectProps {
  value: string
  onChange: (value: string) => void
  projects: ProjectOption[]
}

export function ProjectSelect({ value, onChange, projects }: ProjectSelectProps) {
  const [open, setOpen] = useState(false)
  const ref = useRef<HTMLDivElement>(null)

  const selected = projects.find((p) => p.id === value)

  useEffect(() => {
    if (!open) return

    function handleClickOutside(e: MouseEvent) {
      if (ref.current && !ref.current.contains(e.target as Node)) {
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

  return (
    <div ref={ref} className="relative">
      <button
        type="button"
        onClick={() => setOpen(!open)}
        className={cn(
          'inline-flex items-center gap-2 h-9 px-3 rounded-md text-sm',
          'border border-border bg-background hover:bg-muted transition-colors',
          'focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary focus-visible:ring-offset-2'
        )}
      >
        <FolderOpen className="h-4 w-4 text-muted-foreground" />
        <span className="max-w-[120px] truncate">{selected?.name ?? value}</span>
        <ChevronDown
          className={cn(
            'h-3.5 w-3.5 text-muted-foreground transition-transform',
            open && 'rotate-180'
          )}
        />
      </button>

      {open && (
        <div className="absolute right-0 top-full mt-1 min-w-[180px] rounded-md border border-border bg-background shadow-lg z-50">
          <div className="py-1">
            {projects.map((project) => (
              <div
                key={project.id}
                onClick={() => {
                  onChange(project.id)
                  setOpen(false)
                }}
                className={cn(
                  'flex items-center gap-2 px-3 py-2 text-sm cursor-pointer transition-colors',
                  project.id === value
                    ? 'bg-muted text-foreground'
                    : 'text-foreground hover:bg-muted'
                )}
              >
                <Check
                  className={cn(
                    'h-3.5 w-3.5 shrink-0',
                    project.id === value ? 'opacity-100' : 'opacity-0'
                  )}
                />
                <span className="truncate">{project.name}</span>
              </div>
            ))}
          </div>
        </div>
      )}
    </div>
  )
}
