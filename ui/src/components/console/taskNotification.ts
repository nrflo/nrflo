// Pure parser for the `<task-notification>` envelope emitted by
// category='task_notification' rows — an async delegate/task result summary.
// Kept out of ChatTaskNotification.tsx so it's testable in the Vitest node
// project (see chatInvokeSchema.ts / chatStream.ts precedent).
export interface ParsedTaskNotification {
  taskId: string | null
  status: string | null
  summary: string | null
  result: string | null
}

function extractTag(content: string, tag: string): string | null {
  const match = content.match(new RegExp(`<${tag}>([\\s\\S]*?)</${tag}>`))
  return match ? match[1].trim() : null
}

// Returns null when content is not a task-notification envelope at all, so
// callers can fall back to rendering the raw content.
export function parseTaskNotification(content: string): ParsedTaskNotification | null {
  if (!content || !content.includes('<task-notification>')) return null

  return {
    taskId: extractTag(content, 'task-id'),
    status: extractTag(content, 'status'),
    summary: extractTag(content, 'summary'),
    result: extractTag(content, 'result'),
  }
}
