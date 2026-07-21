import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { ChatComposerSuggestions, filterSuggestions } from './ChatComposerSuggestions'
import type { ConsoleSkill } from '@/types/consoleChat'

const SKILLS: ConsoleSkill[] = [
  { name: 'finalize', description: 'Close out a chunk of work' },
  { name: 'find-bugs', description: 'Hunt for defects' },
  { name: 'deploy', description: 'Ship a release' },
]

function setup(overrides: Partial<Parameters<typeof ChatComposerSuggestions>[0]> = {}) {
  const onSelect = vi.fn()
  const onHover = vi.fn()
  render(
    <ChatComposerSuggestions
      items={SKILLS}
      query=""
      activeIndex={0}
      onSelect={onSelect}
      onHover={onHover}
      {...overrides}
    />
  )
  return { onSelect, onHover }
}

describe('filterSuggestions', () => {
  it('returns all skills for an empty query', () => {
    expect(filterSuggestions(SKILLS, '')).toEqual(SKILLS)
  })

  it('matches by case-insensitive name prefix', () => {
    expect(filterSuggestions(SKILLS, 'FI')).toEqual([SKILLS[0], SKILLS[1]])
  })

  it('falls back to substring match when no prefix matches', () => {
    expect(filterSuggestions(SKILLS, 'bugs')).toEqual([SKILLS[1]])
  })

  it('returns an empty array when nothing matches', () => {
    expect(filterSuggestions(SKILLS, 'zzz')).toEqual([])
  })
})

describe('ChatComposerSuggestions', () => {
  it('renders only matching rows with name and description', () => {
    setup({ query: 'fi' })

    expect(screen.getByText('/finalize')).toBeInTheDocument()
    expect(screen.getByText('Close out a chunk of work')).toBeInTheDocument()
    expect(screen.getByText('/find-bugs')).toBeInTheDocument()
    expect(screen.queryByText('/deploy')).not.toBeInTheDocument()
  })

  it('highlights the row at activeIndex', () => {
    setup({ query: '', activeIndex: 2 })

    expect(screen.getByText('/deploy').closest('div')).toHaveClass('bg-muted')
    expect(screen.getByText('/finalize').closest('div')).not.toHaveClass('bg-muted')
  })

  it('fires onSelect with the skill name when a row is clicked', async () => {
    const { onSelect } = setup({ query: '' })
    const user = userEvent.setup()

    await user.click(screen.getByText('/deploy'))

    expect(onSelect).toHaveBeenCalledWith('deploy')
  })

  it('renders nothing when there are no matches', () => {
    const { container } = render(
      <ChatComposerSuggestions
        items={SKILLS}
        query="zzz"
        activeIndex={0}
        onSelect={vi.fn()}
        onHover={vi.fn()}
      />
    )

    expect(container).toBeEmptyDOMElement()
  })
})
