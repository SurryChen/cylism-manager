import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import DatabaseReplicationPanel from './DatabaseReplicationPanel.vue'

describe('replication observations', () => {
  const resources = [{ ref: 'instance:1', name: 'RDS', location: '外部 MySQL' }, { ref: 'instance:2', name: 'Replica', location: 'worker' }]
  it('shows the actual source, target, zero lag and recorded thread state', async () => {
    const relation = { id: 1, source: 'instance:1', target: 'instance:2', channel: '', status: 'running', last_checked_at: '2026-11-07T01:00:00Z', io_running: true, sql_running: true, lag_seconds: 0, read_only: true }
    const wrapper = mount(DatabaseReplicationPanel, { props: { relations: [relation], resources } })
    expect(wrapper.text()).toContain('RDS')
    expect(wrapper.text()).toContain('Replica')
    expect(wrapper.text()).toContain('0 秒')
    expect(wrapper.text()).toContain('默认通道')
    await wrapper.get('.relation-actions .btn-primary').trigger('click')
    expect(wrapper.emitted('check')[0]).toEqual([relation])
    wrapper.unmount()
  })
  it('does not label older metadata as a live healthy replication', () => {
    const wrapper = mount(DatabaseReplicationPanel, { props: { relations: [{ id: 1, source: 'instance:1', target: 'instance:2', status: 'running' }], resources } })
    expect(wrapper.text()).toContain('待检查')
    expect(wrapper.text()).toContain('未知')
    expect(wrapper.text()).not.toContain('线程正常')
    wrapper.unmount()
  })
})
