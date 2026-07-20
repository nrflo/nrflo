import { useRef, useState } from 'react'
import { Button } from '@/components/ui/Button'
import { Textarea } from '@/components/ui/Textarea'
import { Spinner } from '@/components/ui/Spinner'
import { ChatComposerSuggestions, filterSkills } from './ChatComposerSuggestions'
import { useProjectSkills } from '@/hooks/useConsoleChats'

interface ChatComposerProps {
  isRunning: boolean
  sendPending: boolean
  stopPending: boolean
  onSend: (value: string) => void
  onStop: () => void
}

// Composer wrapper: owns the draft text + a JS autoresize (height='auto' then
// scrollHeight) on the shared Textarea — not CSS field-sizing, and no edits
// to the shared component. min-h/max-h overrides via twMerge cap growth to
// ~8 lines, then overflow-y-auto scrolls internally.
//
// Skill suggestions: typing '/' at the start of an otherwise-empty draft
// (no space yet typed) opens a dropdown of matching skills fetched once per
// project via useProjectSkills. While open, Arrow/Enter/Tab/Escape are owned
// by the dropdown instead of the normal send/newline handling.
export function ChatComposer({ isRunning, sendPending, stopPending, onSend, onStop }: ChatComposerProps) {
  const [text, setText] = useState('')
  const [activeIndex, setActiveIndex] = useState(0)
  const [dismissed, setDismissed] = useState(false)
  const textareaRef = useRef<HTMLTextAreaElement>(null)

  const { data: skills = [] } = useProjectSkills()

  const slashQuery = text.startsWith('/') && !text.slice(1).includes(' ') ? text.slice(1) : null
  const matches = slashQuery !== null ? filterSkills(skills, slashQuery) : []
  const suggestionsOpen = slashQuery !== null && matches.length > 0 && !dismissed

  const resize = () => {
    const el = textareaRef.current
    if (!el) return
    el.style.height = 'auto'
    el.style.height = `${el.scrollHeight}px`
  }

  const closeSuggestions = () => {
    setActiveIndex(0)
    setDismissed(false)
  }

  const handleSend = () => {
    const value = text.trim()
    if (!value || isRunning) return
    setText('')
    closeSuggestions()
    if (textareaRef.current) textareaRef.current.style.height = 'auto'
    onSend(value)
  }

  const selectSkill = (name: string) => {
    setText(`/${name} `)
    closeSuggestions()
    textareaRef.current?.focus()
  }

  return (
    <div className="border-t border-border p-3">
      {suggestionsOpen && (
        <ChatComposerSuggestions
          skills={skills}
          query={slashQuery ?? ''}
          activeIndex={activeIndex}
          onSelect={selectSkill}
          onHover={setActiveIndex}
        />
      )}
      <div className="flex items-end gap-2">
        <Textarea
          ref={textareaRef}
          rows={1}
          value={text}
          onChange={(e) => {
            setText(e.target.value)
            setActiveIndex(0)
            setDismissed(false)
            resize()
          }}
          onKeyDown={(e) => {
            if (suggestionsOpen) {
              if (e.key === 'ArrowDown') {
                e.preventDefault()
                setActiveIndex((i) => (i + 1) % matches.length)
                return
              }
              if (e.key === 'ArrowUp') {
                e.preventDefault()
                setActiveIndex((i) => (i - 1 + matches.length) % matches.length)
                return
              }
              if (e.key === 'Enter' || e.key === 'Tab') {
                e.preventDefault()
                selectSkill(matches[activeIndex].name)
                return
              }
              if (e.key === 'Escape') {
                e.preventDefault()
                setDismissed(true)
                return
              }
            }
            if (e.key === 'Enter' && !e.shiftKey) {
              e.preventDefault()
              handleSend()
            }
          }}
          placeholder={isRunning ? 'Waiting for the agent to finish its turn…' : 'Message the agent…'}
          disabled={isRunning}
          className="min-h-[40px] max-h-[192px] resize-none overflow-y-auto"
        />
        {isRunning ? (
          <Button variant="destructive" onClick={onStop} disabled={stopPending}>
            {stopPending ? <Spinner size="sm" /> : 'Stop'}
          </Button>
        ) : (
          <Button onClick={handleSend} disabled={!text.trim() || sendPending}>
            Send
          </Button>
        )}
      </div>
    </div>
  )
}
