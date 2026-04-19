import { cn } from '@/lib/utils'
import { ChevronDown, Check } from 'lucide-react'
import { useState, useRef, useEffect, type ReactNode } from 'react'
import { Tooltip } from '@/components/ui/Tooltip'

export interface DropdownOption {
  value: string
  label: string
  disabled?: boolean
  tooltip?: string
}

export interface DropdownOptionGroup {
  label: string
  options: DropdownOption[]
}

function isGrouped(options: DropdownOption[] | DropdownOptionGroup[]): options is DropdownOptionGroup[] {
  return options.length > 0 && 'options' in options[0]
}

function flattenOptions(options: DropdownOption[] | DropdownOptionGroup[]): DropdownOption[] {
  if (!isGrouped(options)) return options
  return options.flatMap((g) => g.options)
}

interface DropdownProps {
  value: string
  onChange: (value: string) => void
  options: DropdownOption[] | DropdownOptionGroup[]
  placeholder?: string
  disabled?: boolean
  className?: string
  icon?: ReactNode
  labelClassName?: string
}

function OptionItem({ option, selected, onChange, onClose }: { option: DropdownOption; selected: string; onChange: (v: string) => void; onClose: () => void }) {
  const isDisabled = option.disabled
  const content = (
    <div
      onClick={() => {
        if (isDisabled) return
        onChange(option.value)
        onClose()
      }}
      aria-disabled={isDisabled || undefined}
      className={cn(
        'flex items-center gap-2 px-3 py-2 text-sm transition-colors',
        isDisabled
          ? 'opacity-50 cursor-not-allowed text-muted-foreground'
          : cn(
              'cursor-pointer',
              option.value === selected ? 'bg-muted text-foreground' : 'text-foreground hover:bg-muted'
            )
      )}
    >
      <Check className={cn('h-3.5 w-3.5 shrink-0', option.value === selected ? 'opacity-100' : 'opacity-0')} />
      <span className="truncate">{option.label}</span>
    </div>
  )
  if (option.tooltip) {
    return (
      <Tooltip text={option.tooltip} placement="right">
        {content}
      </Tooltip>
    )
  }
  return content
}

export function Dropdown({
  value,
  onChange,
  options,
  placeholder = 'Select...',
  disabled = false,
  className,
  icon,
  labelClassName,
}: DropdownProps) {
  const [open, setOpen] = useState(false)
  const ref = useRef<HTMLDivElement>(null)

  const selected = flattenOptions(options).find((o) => o.value === value)

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
        onClick={() => !disabled && setOpen(!open)}
        className={cn(
          'inline-flex items-center gap-2 h-9 px-3 rounded-md text-sm w-full',
          'border border-border bg-background hover:bg-muted transition-colors',
          'focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary focus-visible:ring-offset-2',
          disabled && 'opacity-50 cursor-not-allowed',
          className
        )}
      >
        {icon}
        <span className={cn("truncate flex-1 text-left", labelClassName)}>
          {selected?.label ?? placeholder}
        </span>
        <ChevronDown
          className={cn(
            'h-3.5 w-3.5 text-muted-foreground transition-transform shrink-0',
            open && 'rotate-180'
          )}
        />
      </button>

      {open && (
        <div className="absolute left-0 top-full mt-1 min-w-[180px] w-full rounded-md border border-border bg-background shadow-lg z-50">
          <div className="py-1 max-h-80 overflow-y-auto">
            {isGrouped(options)
              ? options.map((group) => (
                  <div key={group.label}>
                    {group.label && (
                      <div className="px-3 py-1.5 text-xs font-semibold uppercase text-muted-foreground">
                        {group.label}
                      </div>
                    )}
                    {group.options.map((option) => (
                      <OptionItem key={option.value} option={option} selected={value} onChange={onChange} onClose={() => setOpen(false)} />
                    ))}
                  </div>
                ))
              : options.map((option) => (
                  <OptionItem key={option.value} option={option} selected={value} onChange={onChange} onClose={() => setOpen(false)} />
                ))}
          </div>
        </div>
      )}
    </div>
  )
}
