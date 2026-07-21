import { describe, it, expect } from 'vitest'
import { parseSchema, initialValues, buildArguments, type FieldDescriptor } from './chatInvokeSchema'
import type { ConsoleToolInputSchema } from '@/types/consoleChat'

const SCHEMA: ConsoleToolInputSchema = {
  type: 'object',
  required: ['path', 'count'],
  properties: {
    path: { type: 'string', description: 'File path' },
    count: { type: 'number', default: 3 },
    enabled: { type: 'boolean', default: true },
    mode: { type: 'string', enum: ['fast', 'slow'], default: 'fast' },
    tags: { type: 'array' },
    meta: { type: 'object', default: { a: 1 } },
    nullableName: { type: ['string', 'null'] },
    weird: {},
  },
}

describe('parseSchema', () => {
  it('maps each property to a field kind, preserving declaration order', () => {
    const fields = parseSchema(SCHEMA)
    expect(fields.map((f) => f.name)).toEqual([
      'path',
      'count',
      'enabled',
      'mode',
      'tags',
      'meta',
      'nullableName',
      'weird',
    ])
    expect(fields.map((f) => f.kind)).toEqual([
      'string',
      'number',
      'boolean',
      'enum',
      'json',
      'json',
      'string',
      'json',
    ])
  })

  it('marks required properties from the schema required list', () => {
    const fields = parseSchema(SCHEMA)
    const byName = Object.fromEntries(fields.map((f) => [f.name, f.required]))
    expect(byName.path).toBe(true)
    expect(byName.count).toBe(true)
    expect(byName.enabled).toBe(false)
  })

  it('resolves a type array by picking the first non-null entry', () => {
    const fields = parseSchema({ properties: { x: { type: ['null', 'string'] } } })
    expect(fields[0].kind).toBe('string')
  })

  it('carries description/enumOptions/default through', () => {
    const fields = parseSchema(SCHEMA)
    const mode = fields.find((f) => f.name === 'mode')!
    expect(mode.enumOptions).toEqual(['fast', 'slow'])
    expect(mode.default).toBe('fast')
    const path = fields.find((f) => f.name === 'path')!
    expect(path.description).toBe('File path')
  })

  it('returns an empty list for a schema with no properties', () => {
    expect(parseSchema(undefined)).toEqual([])
    expect(parseSchema({})).toEqual([])
  })
})

describe('initialValues', () => {
  it('pre-fills defaults per field kind', () => {
    const fields = parseSchema(SCHEMA)
    const values = initialValues(fields)
    expect(values.count).toBe('3')
    expect(values.enabled).toBe(true)
    expect(values.mode).toBe('fast')
    expect(values.meta).toBe(JSON.stringify({ a: 1 }, null, 2))
  })

  it('falls back to empty string / false when no default is set', () => {
    const fields = parseSchema(SCHEMA)
    const values = initialValues(fields)
    expect(values.path).toBe('')
    expect(values.tags).toBe('')
  })
})

describe('buildArguments', () => {
  const stringField: FieldDescriptor = { name: 's', kind: 'string', required: false }
  const requiredStringField: FieldDescriptor = { name: 's', kind: 'string', required: true }
  const numberField: FieldDescriptor = { name: 'n', kind: 'number', required: false }
  const boolField: FieldDescriptor = { name: 'b', kind: 'boolean', required: false }
  const jsonField: FieldDescriptor = { name: 'j', kind: 'json', required: false }
  const enumField: FieldDescriptor = { name: 'e', kind: 'enum', required: false, enumOptions: ['a', 'b'] }

  it('coerces a valid number string to a number', () => {
    const { args, errors } = buildArguments([numberField], { n: '42' })
    expect(args).toEqual({ n: 42 })
    expect(errors).toEqual({})
  })

  it('errors when a number field is not numeric', () => {
    const { args, errors } = buildArguments([numberField], { n: 'abc' })
    expect(errors.n).toBe('Must be a number')
    expect(args.n).toBeUndefined()
  })

  it('parses valid JSON for json-kind fields', () => {
    const { args, errors } = buildArguments([jsonField], { j: '{"a":1}' })
    expect(args).toEqual({ j: { a: 1 } })
    expect(errors).toEqual({})
  })

  it('errors on invalid JSON', () => {
    const { args, errors } = buildArguments([jsonField], { j: '{not json' })
    expect(errors.j).toBe('Invalid JSON')
  })

  it('errors when a required field is empty', () => {
    const { args, errors } = buildArguments([requiredStringField], { s: '  ' })
    expect(errors.s).toBe('Required')
    expect(args.s).toBeUndefined()
  })

  it('omits an empty optional string rather than sending ""', () => {
    const { args, errors } = buildArguments([stringField], { s: '' })
    expect(args).toEqual({})
    expect(errors).toEqual({})
  })

  it('passes booleans through regardless of emptiness', () => {
    const { args, errors } = buildArguments([boolField], { b: true })
    expect(args).toEqual({ b: true })
    expect(errors).toEqual({})
  })

  it('passes enum values through as strings', () => {
    const { args, errors } = buildArguments([enumField], { e: 'b' })
    expect(args).toEqual({ e: 'b' })
    expect(errors).toEqual({})
  })
})
