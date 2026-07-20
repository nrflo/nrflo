import { useState, useEffect } from 'react'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { Input } from '@/components/ui/Input'
import { Tooltip } from '@/components/ui/Tooltip'
import { Info } from 'lucide-react'
import { updateGlobalSettings, settingsKeys, type GlobalSettings } from '@/api/settings'
import { parseOptionalInt } from './AgentForm'

type WatcherKey =
  | 'context_budget_fraction'
  | 'context_budget_default'
  | 'context_decay_turns'
  | 'cache_ttl_sec'
  | 'min_epoch_interval_calls'
  | 'proactive_restart_threshold_default'
  | 'proactive_restart_min_interval_sec'
  | 'proactive_restart_max_per_session'
  | 'proactive_restart_boundary_window_turns'
  | 'proactive_restart_console_pct'

interface WatcherKnob {
  key: WatcherKey
  label: string
  placeholder: string
  tooltip: string
  isFraction?: boolean
}

const WATCHER_KNOBS: WatcherKnob[] = [
  {
    key: 'context_budget_fraction',
    label: 'Context budget fraction',
    placeholder: '0.65',
    tooltip: 'Fraction of a model\'s context_length used to derive its context budget when a per-model value is unavailable. 0-1 decimal.',
    isFraction: true,
  },
  {
    key: 'context_budget_default',
    label: 'Context budget default (tokens)',
    placeholder: '',
    tooltip: 'Fallback token budget used when neither a per-model context_length nor the fraction can be resolved at spawn time.',
  },
  {
    key: 'context_decay_turns',
    label: 'Context decay turns',
    placeholder: '',
    tooltip: 'Number of turns over which stale context weighting decays in the watcher.',
  },
  {
    key: 'cache_ttl_sec',
    label: 'Cache TTL (sec)',
    placeholder: '',
    tooltip: 'How long the watcher caches per-model context metadata before refetching.',
  },
  {
    key: 'min_epoch_interval_calls',
    label: 'Min epoch interval (calls)',
    placeholder: '',
    tooltip: 'Minimum number of provider calls between watcher epoch evaluations.',
  },
  {
    key: 'proactive_restart_threshold_default',
    label: 'Proactive restart threshold (default)',
    placeholder: '',
    tooltip: 'Default context-usage threshold that triggers a proactive restart.',
  },
  {
    key: 'proactive_restart_min_interval_sec',
    label: 'Proactive restart min interval (sec)',
    placeholder: '',
    tooltip: 'Minimum time between proactive restarts for a given session.',
  },
  {
    key: 'proactive_restart_max_per_session',
    label: 'Proactive restart max per session',
    placeholder: '',
    tooltip: 'Maximum number of proactive restarts allowed within a single session.',
  },
  {
    key: 'proactive_restart_boundary_window_turns',
    label: 'Proactive restart boundary window (turns)',
    placeholder: '',
    tooltip: 'Turn window around a phase boundary within which a proactive restart is allowed.',
  },
  {
    key: 'proactive_restart_console_pct',
    label: 'Proactive restart console (%)',
    placeholder: '',
    tooltip: 'Percentage threshold used for proactive restarts in console sessions.',
  },
]

function toDisplay(val: number | null): string {
  return val != null ? String(val) : ''
}

export function WatcherTuningSettings({ settings }: { settings: GlobalSettings }) {
  const queryClient = useQueryClient()

  const [values, setValues] = useState<Record<WatcherKey, string>>(() => {
    const initial = {} as Record<WatcherKey, string>
    for (const knob of WATCHER_KNOBS) initial[knob.key] = toDisplay(settings[knob.key] ?? null)
    return initial
  })

  useEffect(() => {
    const next = {} as Record<WatcherKey, string>
    for (const knob of WATCHER_KNOBS) next[knob.key] = toDisplay(settings[knob.key] ?? null)
    setValues(next)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [WATCHER_KNOBS.map((k) => settings[k.key]).join('|')])

  const mutation = useMutation({
    mutationFn: (data: Partial<GlobalSettings>) => updateGlobalSettings(data),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: settingsKeys.all }),
  })

  const parseValue = (knob: WatcherKnob, raw: string): number | null => {
    if (knob.isFraction) {
      const trimmed = raw.trim()
      if (trimmed === '') return null
      const n = parseFloat(trimmed)
      return isNaN(n) ? null : n
    }
    return parseOptionalInt(raw)
  }

  const submit = (knob: WatcherKnob) => {
    const raw = values[knob.key]
    const parsed = parseValue(knob, raw)
    if (parsed !== null && parsed < 0) {
      setValues((prev) => ({ ...prev, [knob.key]: toDisplay(settings[knob.key] ?? null) }))
      return
    }
    if (parsed !== (settings[knob.key] ?? null)) {
      mutation.mutate({ [knob.key]: parsed } as Partial<GlobalSettings>)
    }
  }

  return (
    <>
      {WATCHER_KNOBS.map((knob) => (
        <div key={knob.key}>
          <div className="border-t border-border" />
          <div className="flex items-center justify-between">
            <div className="flex items-center gap-1.5">
              <div className="text-sm font-medium">{knob.label}</div>
              <Tooltip placement="right" className="max-w-sm" text={knob.tooltip}>
                <Info className="h-3.5 w-3.5 text-muted-foreground" />
              </Tooltip>
            </div>
            <Input
              type="text"
              value={values[knob.key]}
              onChange={(e) => setValues((prev) => ({ ...prev, [knob.key]: e.target.value }))}
              onBlur={() => submit(knob)}
              onKeyDown={(e) => { if (e.key === 'Enter') submit(knob) }}
              disabled={mutation.isPending}
              placeholder={knob.placeholder}
              className="w-24"
            />
          </div>
        </div>
      ))}
    </>
  )
}
