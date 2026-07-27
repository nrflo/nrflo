import { describe, it, expect } from 'vitest'
import { statusColor, planStatusLabel, partitionWorkflowInstances } from './utils'

const PLAN_STATUSES = ['planning', 'waiting_input', 'waiting_approval']

describe('statusColor — plan-boundary statuses', () => {
  it.each(PLAN_STATUSES)('returns a non-default (amber) class for status=%s', (status) => {
    const result = statusColor(status)
    expect(result).toContain('bg-amber-100')
    expect(result).toContain('text-amber-800')
  })

  it('groups plan statuses with "waiting" under the same amber bucket', () => {
    expect(statusColor('waiting')).toBe(statusColor('waiting_approval'))
  })

  it('falls back to the default gray bucket for an unrecognized status', () => {
    expect(statusColor('totally_unknown')).toBe('bg-gray-100 text-gray-800 dark:bg-gray-700 dark:text-gray-200')
  })
})

describe('planStatusLabel', () => {
  it('returns a human label for each plan-boundary status', () => {
    expect(planStatusLabel('planning')).toBe('Planning')
    expect(planStatusLabel('waiting_input')).toBe('Needs input')
    expect(planStatusLabel('waiting_approval')).toBe('Awaiting plan approval')
  })

  it('returns undefined for a non-plan status', () => {
    expect(planStatusLabel('active')).toBeUndefined()
    expect(planStatusLabel('completed')).toBeUndefined()
  })

  it('returns undefined for undefined input', () => {
    expect(planStatusLabel(undefined)).toBeUndefined()
  })
})

describe('partitionWorkflowInstances', () => {
  function make(status: string) {
    return { status }
  }

  it('places completed and project_completed statuses in completedInstances', () => {
    const { completedInstances } = partitionWorkflowInstances({
      a: make('completed'),
      b: make('project_completed'),
    })
    expect(Object.keys(completedInstances)).toEqual(['a', 'b'])
  })

  it('places failed status in failedInstances', () => {
    const { failedInstances } = partitionWorkflowInstances({ a: make('failed') })
    expect(Object.keys(failedInstances)).toEqual(['a'])
  })

  it.each(PLAN_STATUSES)('places plan-suspended status=%s in runningInstances (not terminal)', (status) => {
    const { runningInstances, failedInstances, completedInstances } = partitionWorkflowInstances({
      a: make(status),
    })
    expect(Object.keys(runningInstances)).toEqual(['a'])
    expect(Object.keys(failedInstances)).toEqual([])
    expect(Object.keys(completedInstances)).toEqual([])
  })

  it('places active and waiting statuses in runningInstances', () => {
    const { runningInstances } = partitionWorkflowInstances({
      a: make('active'),
      b: make('waiting'),
    })
    expect(Object.keys(runningInstances)).toEqual(['a', 'b'])
  })

  it('splits a mixed set of instances across all three buckets', () => {
    const result = partitionWorkflowInstances({
      running1: make('active'),
      planSuspended1: make('waiting_approval'),
      failed1: make('failed'),
      completed1: make('completed'),
    })
    expect(Object.keys(result.runningInstances)).toEqual(['running1', 'planSuspended1'])
    expect(Object.keys(result.failedInstances)).toEqual(['failed1'])
    expect(Object.keys(result.completedInstances)).toEqual(['completed1'])
  })
})
