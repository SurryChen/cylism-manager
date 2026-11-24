import { describe, expect, it } from 'vitest'
import { dueState, workbenchSummary, editableProduct } from './product-utils.js'

describe('personal workbench rules', () => {
  const now = new Date(2026, 10, 8, 23, 59)
  it('uses calendar dates and an inclusive seven-day window', () => {
    expect(dueState('2026-11-07T00:00:00Z', now)).toBe('overdue')
    expect(dueState('2026-11-08T00:00:00Z', now)).toBe('today')
    expect(dueState('2026-11-15', now)).toBe('soon')
    expect(dueState('2026-11-16', now)).toBe('later')
  })
  it('excludes paused/archived products and keeps undated actions', () => {
    const summary = workbenchSummary([
      { id: 1, stage: 'developing', next_action: 'publish', next_due_date: '2026-11-09', channels: [{ publish_status: 'rejected' }] },
      { id: 2, stage: 'idea', next_action: 'sketch' },
      { id: 3, stage: 'archived', next_action: 'old', next_due_date: '2026-11-07', channels: [{ publish_status: 'blocked' }] },
      { id: 4, stage: 'paused' },
    ], now)
    expect(summary.active).toHaveLength(2)
    expect(summary.actions.map(p => p.id)).toEqual([1, 2])
    expect(summary.due).toHaveLength(1)
    expect(summary.attention).toHaveLength(1)
  })
  it('copies only explicitly editable data without mutating detail state', () => {
    const product = { name: 'demo', channels: [{ id: 1, name: 'web' }], projects: [{ id: 2 }], owner_id: 9 }
    const edit = editableProduct(product)
    edit.channels[0].name = 'changed'
    expect(product.channels[0].name).toBe('web')
    expect(edit.project_ids).toEqual([2])
    expect(edit.owner_id).toBeUndefined()
  })
})
