export function renderTemplatePreview(template: string, sample: Record<string, unknown>): string {
  return template.replace(/\$\{([^}]+)\}/g, (_, key: string) => {
    const val = sample[key]
    return val !== undefined ? String(val) : ''
  })
}
