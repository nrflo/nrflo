import type { APIProviderName } from '@/api/apiModels'
import type { DropdownOption } from '@/components/ui/Dropdown'

export const REASONING_EFFORT_OPTIONS: DropdownOption[] = [
  { value: '', label: 'Default' },
  { value: 'low', label: 'Low' },
  { value: 'medium', label: 'Medium' },
  { value: 'high', label: 'High' },
  { value: 'xhigh', label: 'Extra High (Opus 4.7/4.8 or Sonnet 5 only)' },
  { value: 'max', label: 'Max' },
  { value: 'ultra', label: 'Ultra (Codex GPT-5.6 Sol/Terra only)' },
]

export function buildCLIEffortOptions(cliType: string, mappedModel: string): DropdownOption[] {
  if (cliType === 'claude') {
    const supportsXHigh =
      mappedModel.startsWith('claude-opus-4-7') ||
      mappedModel.startsWith('claude-opus-4-8') ||
      mappedModel.startsWith('claude-sonnet-5')
    return REASONING_EFFORT_OPTIONS.filter((opt) => opt.value !== 'ultra').map((opt) =>
      opt.value === 'xhigh' && !supportsXHigh
        ? { ...opt, disabled: true, tooltip: "'xhigh' is only supported on Opus 4.7/4.8 or Sonnet 5 Claude models" }
        : opt
    )
  }
  const supportsUltra =
    mappedModel.startsWith('gpt-5.6-sol') || mappedModel.startsWith('gpt-5.6-terra')
  return REASONING_EFFORT_OPTIONS.filter((opt) => opt.value !== 'xhigh').map((opt) =>
    opt.value === 'ultra' && !supportsUltra
      ? { ...opt, disabled: true, tooltip: "'ultra' is only supported on Codex GPT-5.6 Sol/Terra models" }
      : opt
  )
}

export function buildAPIEffortOptions(provider: APIProviderName, mappedModel: string): DropdownOption[] {
  if (provider === 'anthropic') {
    const supportsXHigh =
      mappedModel.startsWith('claude-opus-4-7') ||
      mappedModel.startsWith('claude-opus-4-8') ||
      mappedModel.startsWith('claude-sonnet-5')
    return REASONING_EFFORT_OPTIONS.filter((opt) => opt.value !== 'ultra').map((opt) =>
      opt.value === 'xhigh' && !supportsXHigh
        ? { ...opt, disabled: true, tooltip: "'xhigh' is only supported on Anthropic Opus 4.7/4.8 or Sonnet 5 models" }
        : opt
    )
  }
  return REASONING_EFFORT_OPTIONS.filter((opt) => opt.value !== 'xhigh' && opt.value !== 'ultra')
}
