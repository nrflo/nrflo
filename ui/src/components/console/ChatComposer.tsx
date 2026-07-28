import { useRef, useState } from 'react'
import { Button } from '@/components/ui/Button'
import { Textarea } from '@/components/ui/Textarea'
import { Spinner } from '@/components/ui/Spinner'
import { ChatComposerSuggestions, filterSuggestions, type SuggestionItem } from './ChatComposerSuggestions'
import { ChatInvokeForm } from './ChatInvokeForm'
import { useProjectSkills } from '@/hooks/useConsoleChats'
import { useChatTools } from '@/hooks/useChatTools'
import type { ConsoleTool } from '@/types/consoleChat'

interface ChatComposerProps {
  sid: string
  isRunning: boolean
  sendPending: boolean
  stopPending: boolean
  onSend: (value: string) => void
  onStop: () => void
}

const INVOKE_DIRECTIVE: SuggestionItem = { name: 'invoke', description: 'Run a tool directly, outside the model' }
const INVOKE_PREFIX = '/invoke '

// Composer wrapper: owns the draft text + a JS autoresize (height='auto' then
// scrollHeight) on the shared Textarea — not CSS field-sizing, and no edits
// to the shared component. min-h/max-h overrides via twMerge cap growth to
// ~8 lines, then overflow-y-auto scrolls internally.
//
// '/' suggestions: typing '/' at the start of an otherwise-empty draft (no
// space yet typed) opens a dropdown of matching skills plus a reserved
// '/invoke' directive row, fetched once per project via useProjectSkills.
// Selecting the directive (or typing '/invoke ') switches the dropdown to
// the chat's tool list (useChatTools) — selecting a tool opens
// ChatInvokeForm, a schema-driven argument form, instead of inserting text.
// While a dropdown is open, Arrow/Enter/Tab/Escape are owned by it instead
// of the normal send/newline handling.
export function ChatComposer({ sid, isRunning, sendPending, stopPending, onSend, onStop }: ChatComposerProps) {
  const [text, setText] = useState('')
  const [activeIndex, setActiveIndex] = useState(0)
  const [dismissed, setDismissed] = useState(false)
  const [selectedTool, setSelectedTool] = useState<ConsoleTool | null>(null)
  const textareaRef = useRef<HTMLTextAreaElement>(null)

  const { data: skills = [] } = useProjectSkills()
  const { data: tools = [] } = useChatTools(sid)

  const isToolMode = text.startsWith(INVOKE_PREFIX)
  const isSkillMode = !isToolMode && text.startsWith('/') && !text.slice(1).includes(' ')

  const query = isToolMode ? text.slice(INVOKE_PREFIX.length) : isSkillMode ? text.slice(1) : null
  const items: SuggestionItem[] = isToolMode ? tools : [INVOKE_DIRECTIVE, ...skills]
  const matches = query !== null ? filterSuggestions(items, query) : []
  const suggestionsOpen = query !== null && matches.length > 0 && !dismissed && !selectedTool

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
    if (!value) return
    setText('')
    closeSuggestions()
    if (textareaRef.current) textareaRef.current.style.height = 'auto'
    onSend(value)
  }

  const selectSuggestion = (name: string) => {
    if (isToolMode) {
      const tool = tools.find((t) => t.name === name)
      if (tool) {
        setSelectedTool(tool)
        setText('')
      }
      closeSuggestions()
      return
    }
    if (name === INVOKE_DIRECTIVE.name) {
      setText(INVOKE_PREFIX)
      setActiveIndex(0)
      textareaRef.current?.focus()
      return
    }
    setText(`/${name} `)
    closeSuggestions()
    textareaRef.current?.focus()
  }

  return (
    <div className="border-t border-border p-3">
      {suggestionsOpen && (
        <ChatComposerSuggestions
          items={items}
          query={query ?? ''}
          activeIndex={activeIndex}
          onSelect={selectSuggestion}
          onHover={setActiveIndex}
        />
      )}
      {selectedTool && (
        <ChatInvokeForm sid={sid} tool={selectedTool} onClose={() => setSelectedTool(null)} />
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
                selectSuggestion(matches[activeIndex].name)
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
          placeholder={isRunning ? 'Turn running — your message will be queued…' : 'Message the agent…'}
          className="min-h-[40px] max-h-[192px] resize-none overflow-y-auto"
        />
        {isRunning ? (
          <>
            <Button onClick={handleSend} disabled={!text.trim() || sendPending}>
              Queue
            </Button>
            <Button variant="destructive" onClick={onStop} disabled={stopPending}>
              {stopPending ? <Spinner size="sm" /> : 'Stop'}
            </Button>
          </>
        ) : (
          <Button onClick={handleSend} disabled={!text.trim() || sendPending}>
            Send
          </Button>
        )}
      </div>
    </div>
  )
}
