import { cn } from '@/lib/utils'
import type { NrvappRange } from '@/types/nrvapp'

interface Props {
  value: NrvappRange
  onChange: (range: NrvappRange) => void
}

const options: { label: string; value: NrvappRange }[] = [
  { label: '7d', value: '7d' },
  { label: '30d', value: '30d' },
]

export function RangeSelector({ value, onChange }: Props) {
  return (
    <div className="inline-flex rounded-md border border-border overflow-hidden">
      {options.map((opt) => (
        <button
          key={opt.value}
          type="button"
          onClick={() => onChange(opt.value)}
          className={cn(
            'px-3 py-1.5 text-sm transition-colors',
            value === opt.value
              ? 'bg-muted text-foreground font-medium'
              : 'text-muted-foreground hover:bg-muted/50'
          )}
        >
          {opt.label}
        </button>
      ))}
    </div>
  )
}
