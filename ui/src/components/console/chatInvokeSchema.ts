import type { ConsoleToolInputSchema, JSONSchemaProperty } from '@/types/consoleChat'

export type FieldKind = 'string' | 'number' | 'boolean' | 'enum' | 'json'

export interface FieldDescriptor {
  name: string
  kind: FieldKind
  required: boolean
  description?: string
  enumOptions?: string[]
  default?: unknown
}

function resolveType(type: string | string[] | undefined): string | undefined {
  if (Array.isArray(type)) return type.find((t) => t !== 'null') ?? type[0]
  return type
}

function fieldKind(prop: JSONSchemaProperty): FieldKind {
  if (prop.enum && prop.enum.length > 0) return 'enum'
  const type = resolveType(prop.type)
  if (type === 'number' || type === 'integer') return 'number'
  if (type === 'boolean') return 'boolean'
  if (type === 'string') return 'string'
  return 'json'
}

// Parses a tool's input_schema into an ordered list of form field
// descriptors — one per top-level property, preserving declaration order.
export function parseSchema(inputSchema: ConsoleToolInputSchema | undefined): FieldDescriptor[] {
  const properties = inputSchema?.properties ?? {}
  const required = new Set(inputSchema?.required ?? [])
  return Object.entries(properties).map(([name, prop]) => ({
    name,
    kind: fieldKind(prop),
    required: required.has(name),
    description: prop.description,
    enumOptions: prop.enum?.map((v) => String(v)),
    default: prop.default,
  }))
}

// Pre-fills form state from each field's schema default.
export function initialValues(fields: FieldDescriptor[]): Record<string, string | boolean> {
  const values: Record<string, string | boolean> = {}
  for (const field of fields) {
    if (field.default === undefined) {
      values[field.name] = field.kind === 'boolean' ? false : ''
      continue
    }
    if (field.kind === 'boolean') {
      values[field.name] = Boolean(field.default)
    } else if (field.kind === 'json') {
      values[field.name] = JSON.stringify(field.default, null, 2)
    } else {
      values[field.name] = String(field.default)
    }
  }
  return values
}

interface BuildResult {
  args: Record<string, unknown>
  errors: Record<string, string>
}

// Coerces raw form values into typed arguments per field kind, validating
// required-ness and JSON/number parsing. Empty optional strings are omitted
// rather than sent as "".
export function buildArguments(fields: FieldDescriptor[], values: Record<string, string | boolean>): BuildResult {
  const args: Record<string, unknown> = {}
  const errors: Record<string, string> = {}

  for (const field of fields) {
    const raw = values[field.name]

    if (field.kind === 'boolean') {
      args[field.name] = Boolean(raw)
      continue
    }

    const text = typeof raw === 'string' ? raw : ''
    const isEmpty = text.trim() === ''

    if (isEmpty) {
      if (field.required) errors[field.name] = 'Required'
      continue
    }

    if (field.kind === 'number') {
      const n = Number(text)
      if (Number.isNaN(n)) {
        errors[field.name] = 'Must be a number'
      } else {
        args[field.name] = n
      }
      continue
    }

    if (field.kind === 'json') {
      try {
        args[field.name] = JSON.parse(text)
      } catch {
        errors[field.name] = 'Invalid JSON'
      }
      continue
    }

    // string | enum
    args[field.name] = text
  }

  return { args, errors }
}
