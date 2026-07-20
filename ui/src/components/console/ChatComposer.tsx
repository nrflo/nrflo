import { useRef, useState } from 'react'
import { Button } from '@/components/ui/Button'
import { Textarea } from '@/components/ui/Textarea'
import { Spinner } from '@/components/ui/Spinner'

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
export function ChatComposer({ isRunning, sendPending, stopPending, onSend, onStop }: ChatComposerProps) {
  const [text, setText] = useState('')
  const textareaRef = useRef<HTMLTextAreaElement>(null)

  const resize = () => {
    const el = textareaRef.current
    if (!el) return
    el.style.height = 'auto'
    el.style.height = `${el.scrollHeight}px`
  }

  const handleSend = () => {
    const value = text.trim()
    if (!value || isRunning) return
    setText('')
    if (textareaRef.current) textareaRef.current.style.height = 'auto'
    onSend(value)
  }

  return (
    <div className="border-t border-border p-3">
      <div className="flex items-end gap-2">
        <Textarea
          ref={textareaRef}
          rows={1}
          value={text}
          onChange={(e) => {
            setText(e.target.value)
            resize()
          }}
          onKeyDown={(e) => {
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
