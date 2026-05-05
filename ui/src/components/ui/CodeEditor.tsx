import { useRef, useEffect } from 'react'
import { EditorState } from '@codemirror/state'
import { EditorView, keymap, placeholder as cmPlaceholder, lineNumbers } from '@codemirror/view'
import { defaultKeymap, history, historyKeymap } from '@codemirror/commands'
import { markdown } from '@codemirror/lang-markdown'
import { python } from '@codemirror/lang-python'
import { languages } from '@codemirror/language-data'
import { markdownTheme } from '@/components/ui/codemirror-theme'

type CodeLanguage = 'python' | 'markdown' | 'plain'

function buildLanguageExtension(language: CodeLanguage) {
  if (language === 'python') return python()
  if (language === 'markdown') return markdown({ codeLanguages: languages })
  return []
}

export function CodeEditor({
  value,
  onChange,
  language = 'plain',
  placeholder,
  readOnly = false,
  minHeight = '200px',
  maxHeight = '400px',
}: {
  value: string
  onChange?: (value: string) => void
  language?: CodeLanguage
  placeholder?: string
  readOnly?: boolean
  minHeight?: string
  maxHeight?: string
}) {
  const containerRef = useRef<HTMLDivElement>(null)
  const viewRef = useRef<EditorView | null>(null)
  const isExternalUpdate = useRef(false)

  useEffect(() => {
    if (!containerRef.current) return

    const extensions = [
      markdownTheme,
      ...([buildLanguageExtension(language)].flat()),
      EditorView.lineWrapping,
      lineNumbers(),
      history(),
      keymap.of([...defaultKeymap, ...historyKeymap]),
      EditorView.theme({
        '.cm-scroller': { minHeight, maxHeight },
      }),
    ]

    if (placeholder) {
      extensions.push(cmPlaceholder(placeholder))
    }

    if (readOnly) {
      extensions.push(EditorState.readOnly.of(true), EditorView.editable.of(false))
    } else if (onChange) {
      extensions.push(
        EditorView.updateListener.of((update) => {
          if (update.docChanged && !isExternalUpdate.current) {
            onChange(update.state.doc.toString())
          }
        })
      )
    }

    const state = EditorState.create({ doc: value, extensions })
    const view = new EditorView({ state, parent: containerRef.current })
    viewRef.current = view

    return () => {
      view.destroy()
      viewRef.current = null
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [readOnly, language])

  useEffect(() => {
    const view = viewRef.current
    if (!view) return
    const current = view.state.doc.toString()
    if (current !== value) {
      isExternalUpdate.current = true
      view.dispatch({
        changes: { from: 0, to: current.length, insert: value },
      })
      isExternalUpdate.current = false
    }
  }, [value])

  return <div ref={containerRef} />
}
