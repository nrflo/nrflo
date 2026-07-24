/** Derives the CLI name from a colon-form model_id (e.g. "claude:sonnet-5" -> "claude"). */
export function cliFromModelId(modelId?: string): string | undefined {
  if (!modelId) return undefined
  const idx = modelId.indexOf(':')
  if (idx === -1) return undefined
  return modelId.slice(0, idx)
}
