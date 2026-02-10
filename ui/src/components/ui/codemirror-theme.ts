import { EditorView } from '@codemirror/view'
import { HighlightStyle, syntaxHighlighting } from '@codemirror/language'
import { tags } from '@lezer/highlight'

const editorTheme = EditorView.theme({
  '&': {
    backgroundColor: 'var(--color-background)',
    color: 'var(--color-foreground)',
    fontSize: '13px',
    borderRadius: 'var(--radius-md)',
    border: '1px solid var(--color-border)',
  },
  '&.cm-focused': {
    outline: '2px solid var(--color-primary)',
    outlineOffset: '-1px',
  },
  '.cm-content': {
    fontFamily: 'ui-monospace, SFMono-Regular, "SF Mono", Menlo, monospace',
    padding: '8px 0',
    caretColor: 'var(--color-foreground)',
  },
  '.cm-line': {
    padding: '0 12px',
  },
  '.cm-gutters': {
    backgroundColor: 'var(--color-muted)',
    color: 'var(--color-muted-foreground)',
    border: 'none',
    borderRight: '1px solid var(--color-border)',
  },
  '.cm-activeLineGutter': {
    backgroundColor: 'var(--color-secondary)',
  },
  '.cm-activeLine': {
    backgroundColor: 'color-mix(in srgb, var(--color-muted) 40%, transparent)',
  },
  '.cm-selectionBackground': {
    backgroundColor: 'color-mix(in srgb, var(--color-primary) 25%, transparent) !important',
  },
  '.cm-cursor': {
    borderLeftColor: 'var(--color-foreground)',
  },
  '.cm-placeholder': {
    color: 'var(--color-muted-foreground)',
  },
  '.cm-scroller': {
    overflow: 'auto',
  },
})

const highlightStyle = HighlightStyle.define([
  { tag: tags.heading1, fontWeight: '700', fontSize: '1.3em', color: 'var(--color-foreground)' },
  { tag: tags.heading2, fontWeight: '700', fontSize: '1.15em', color: 'var(--color-foreground)' },
  { tag: tags.heading3, fontWeight: '600', fontSize: '1.05em', color: 'var(--color-foreground)' },
  { tag: [tags.heading4, tags.heading5, tags.heading6], fontWeight: '600', color: 'var(--color-foreground)' },
  { tag: tags.emphasis, fontStyle: 'italic', color: 'var(--color-muted-foreground)' },
  { tag: tags.strong, fontWeight: '700' },
  { tag: tags.strikethrough, textDecoration: 'line-through', color: 'var(--color-muted-foreground)' },
  { tag: tags.link, color: 'var(--color-primary)', textDecoration: 'underline' },
  { tag: tags.url, color: 'var(--color-primary)' },
  { tag: tags.monospace, color: 'var(--color-warning)', backgroundColor: 'var(--color-muted)', borderRadius: '3px', padding: '1px 4px' },
  { tag: [tags.processingInstruction, tags.contentSeparator], color: 'var(--color-muted-foreground)' },
  { tag: tags.list, color: 'var(--color-primary)' },
  { tag: tags.quote, color: 'var(--color-muted-foreground)', fontStyle: 'italic' },
  { tag: tags.meta, color: 'var(--color-muted-foreground)' },
])

export const markdownTheme = [editorTheme, syntaxHighlighting(highlightStyle)]
