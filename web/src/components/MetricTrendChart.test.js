import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import MetricTrendChart from './MetricTrendChart.vue'

describe('MetricTrendChart', () => {
  it('renders a metric title, unit, threshold, and chart series', () => {
    const wrapper = mount(MetricTrendChart, {
      props: {
        title: 'CPU 使用率',
        unit: '%',
        threshold: 85,
        series: [{ label: 'node-a', values: [{ timestamp: 1_785_000_000, value: 42.5 }] }],
      },
      global: { stubs: { Line: true } },
    })

    expect(wrapper.text()).toContain('CPU 使用率')
    expect(wrapper.text()).toContain('阈值 85%')
    expect(wrapper.find('.metric-trend-chart').exists()).toBe(true)
  })
})
