import { afterEach, describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import { reactive } from 'vue'
import ApplicationStacks from './ApplicationStacks.vue'

vi.mock('../api/index.js', () => ({
  api: { get: vi.fn(), post: vi.fn(), put: vi.fn(), delete: vi.fn() },
}))

const route = reactive({ query: { project_id: '1', environment_id: '2' } })
const push = vi.fn()
vi.mock('vue-router', () => ({ useRoute: () => route, useRouter: () => ({ push }) }))
const mountOptions = { global: { stubs: { RouterLink: { template: '<a><slot /></a>' } } } }

const stack = {
  id: 7,
  name: 'Karakeep',
  enabled: true,
  spec: {
    entry_application: 'karakeep',
    components: [
      { application_name: 'karakeep', spec: { image: 'ghcr.io/karakeep-app/karakeep', version: 'latest', container_port: 3000, replicas: 1, config: {}, secrets: {}, volumes: [] } },
    ],
  },
}

async function flush() {
  await new Promise(resolve => setTimeout(resolve, 0))
}

function stubLoad({ stacks = [stack], apps = [{ id: 12, name: 'karakeep' }] } = {}) {
  return path => {
    if (path === '/projects') return Promise.resolve([{ id: 1, name: 'knowledge', environments: [{ id: 2, name: 'production', namespace: 'project-knowledge-prod' }] }])
    if (path === '/api-unreachable') return Promise.resolve([])
    if (path === '/application-stacks?environment_id=2') return Promise.resolve(stacks)
    if (path === '/k8s/persistent-volume-claims?environment_id=2') return Promise.resolve([{ name: 'karakeep-data', phase: 'Bound' }])
    if (path === '/applications?project_id=1&environment_id=2') return Promise.resolve(apps)
    if (path === '/application-stacks/7/releases') return Promise.resolve([])
    return Promise.resolve([])
  }
}

afterEach(() => {
  document.body.innerHTML = ''
  vi.clearAllMocks()
})

describe('ApplicationStacks view', () => {
  it('creates the Karakeep preset in the selected environment', async () => {
    const { api } = await import('../api/index.js')
    api.get.mockImplementation(stubLoad())
    api.post.mockResolvedValue({ id: 7 })
    const wrapper = mount(ApplicationStacks, mountOptions)
    await flush()

    await wrapper.findAll('button').find(button => button.text() === '使用 Karakeep 预设').trigger('click')
    const modal = document.body.querySelector('.modal')
    const selects = modal.querySelectorAll('select')
    selects[0].value = 'karakeep-data'
    selects[0].dispatchEvent(new Event('change'))
    selects[1].value = 'karakeep-data'
    selects[1].dispatchEvent(new Event('change'))
    const passwords = modal.querySelectorAll('input[type="password"]')
    passwords[0].value = 'nextauth-secret'
    passwords[0].dispatchEvent(new Event('input'))
    passwords[1].value = 'meili-secret'
    passwords[1].dispatchEvent(new Event('input'))
    await modal.querySelector('form').dispatchEvent(new Event('submit'))
    await flush()

    expect(api.post).toHaveBeenCalledWith('/application-stacks/presets/karakeep', expect.objectContaining({
      environment_id: 2,
      data_pvc: 'karakeep-data',
      meilisearch_pvc: 'karakeep-data',
      nextauth_secret: 'nextauth-secret',
      meili_master_key: 'meili-secret',
    }))
    wrapper.unmount()
  })

  it('publishes versions by component and opens the independent application', async () => {
    const { api } = await import('../api/index.js')
    api.get.mockImplementation(stubLoad())
    api.post.mockResolvedValue({ id: 3 })
    const wrapper = mount(ApplicationStacks, mountOptions)
    await flush()

    await wrapper.find('.component-link').trigger('click')
    expect(push).toHaveBeenCalledWith('/applications/12')

    await wrapper.findAll('button').find(button => button.text() === '发布').trigger('click')
    const modal = document.body.querySelector('.modal')
    const version = modal.querySelector('input')
    version.value = '0.25.1'
    version.dispatchEvent(new Event('input'))
    await modal.querySelector('form').dispatchEvent(new Event('submit'))
    await flush()

    expect(api.post).toHaveBeenCalledWith('/application-stacks/7/releases', { versions: { karakeep: '0.25.1' } })
    wrapper.unmount()
  })

  it('saves a generic stack instead of creating an embedded application', async () => {
    const { api } = await import('../api/index.js')
    api.get.mockImplementation(stubLoad({ stacks: [] }))
    api.post.mockResolvedValue({ id: 8 })
    const wrapper = mount(ApplicationStacks, mountOptions)
    await flush()

    await wrapper.findAll('button').find(button => button.text() === '新建通用栈').trigger('click')
    const modal = document.body.querySelector('.editor-modal')
    const inputs = modal.querySelectorAll('input')
    inputs[0].value = 'knowledge'
    inputs[0].dispatchEvent(new Event('input'))
    inputs[2].value = 'search'
    inputs[2].dispatchEvent(new Event('input'))
    inputs[3].value = 'getmeili/meilisearch'
    inputs[3].dispatchEvent(new Event('input'))
    await modal.querySelector('form').dispatchEvent(new Event('submit'))
    await flush()

    expect(api.post).toHaveBeenCalledWith('/application-stacks', expect.objectContaining({
      environment_id: 2,
      name: 'knowledge',
      spec: expect.objectContaining({ components: [expect.objectContaining({ application_name: 'search' })] }),
    }))
    wrapper.unmount()
  })
})
