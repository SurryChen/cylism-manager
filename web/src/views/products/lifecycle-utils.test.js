import { describe, expect, it } from 'vitest'
import { developmentSummary } from './lifecycle-utils.js'
describe('development workbench', () => {
  it('distinguishes waiting from actions and preserves legacy data without counting channels twice', () => {
    const result = developmentSummary({ products: [
      { id: 1, name: 'App', stage: 'developing', next_action: 'legacy action', channels: [{ id: 7, publish_status: 'in_review' }, { id: 8, publish_status: 'rejected' }] },
      { id: 2, stage: 'archived', next_action: 'hidden' },
    ], tasks: [
      { id: 1, product_id: 1, title: 'later', status: 'todo', due_date: '2026-12-10' },
      { id: 2, product_id: 1, title: 'urgent', status: 'blocked' },
      { id: 3, product_id: 1, title: 'overdue', status: 'todo', due_date: '2026-11-01' },
      { id: 4, product_id: 1, title: 'done', status: 'done' },
    ], distributions: [{ id: 4, product_id: 1, channel_id: 7, status: 'in_review' }], versions: [{ id: 3, product_id: 1, status: 'testing' }] }, new Date(2026, 10, 17))
    expect(result.actions.map(task => task.title)).toEqual(['urgent', 'overdue', 'later', 'legacy action'])
    expect(result.followups).toHaveLength(2)
    expect(result.needsAction.map(row => row.status)).toEqual(['rejected'])
    expect(result.waiting).toHaveLength(1)
    expect(result.due).toHaveLength(1)
    expect(result.versions).toHaveLength(1)
    expect(result.unplanned).toHaveLength(0)
  })
})
