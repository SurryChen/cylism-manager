<template>
  <BaseModal teleport :open="open" :title="product?.id ? section.title : '创建产品'" size="large" :show-close="!busy" :close-on-overlay="!busy" :close-on-escape="!busy" @close="$emit('close')">
    <SectionTabsHeader :title="section.title" compact :tabs="[section]" :active-tab="tab" />
    <form :id="formID" @submit.prevent="$emit('save', form)">
      <fieldset :disabled="busy" class="product-editor-fields">
        <div v-if="tab === 'basics'" class="product-form-grid">
          <label class="form-group product-full-width"><span class="form-label">名称 *</span><input v-model.trim="form.name" class="form-input" maxlength="128" required aria-label="产品名称" /></label>
          <label class="form-group"><span class="form-label">阶段</span><SelectMenu v-model="form.stage" aria-label="产品阶段"><option v-for="item in stages" :key="item.id" :value="item.id">{{ item.label }}</option></SelectMenu></label>
          <label class="form-group"><span class="form-label">标识（文字或 Emoji）</span><input v-model="form.identifier" class="form-input" maxlength="32" /></label>
          <label class="form-group product-full-width"><span class="form-label">图标地址</span><input v-model.trim="form.icon_url" class="form-input" type="url" placeholder="https://" /></label>
          <label class="form-group product-full-width"><span class="form-label">简介</span><textarea v-model="form.summary" class="form-input" maxlength="1024" /></label>
        </div>
        <template v-else-if="tab === 'links'">
          <EmptyState v-if="!form.links.length" message="还没有资料链接" />
          <div v-for="(link, index) in form.links" :key="index" class="product-editor-row">
            <div class="product-actions"><span class="product-muted">资料 {{ index + 1 }}</span><button class="icon-button" type="button" title="移除资料链接" aria-label="移除资料链接" @click="form.links.splice(index, 1)"><Trash2 :size="15" /></button></div>
            <div class="product-form-grid">
              <label class="form-group"><span class="form-label">类型</span><SelectMenu v-model="link.kind" aria-label="资料类型"><option v-for="kind in linkKinds" :key="kind.id" :value="kind.id">{{ kind.label }}</option></SelectMenu></label>
              <label class="form-group"><span class="form-label">名称</span><input v-model="link.label" class="form-input" maxlength="128" placeholder="例如：客户端仓库" /></label>
              <label class="form-group product-full-width"><span class="form-label">链接 *</span><input v-model.trim="link.url" class="form-input" type="url" required placeholder="https://" /></label>
            </div>
          </div>
          <button class="btn btn-sm product-section-gap" type="button" :disabled="form.links.length >= 30" @click="form.links.push({ kind: 'repository', label: '', url: '' })">添加资料链接</button>
        </template>
        <template v-else-if="tab === 'channels'">
          <p class="product-muted">维护平台标识和访问链接。版本、Build 和审核进度统一在发行记录中更新；暂停渠道会保留历史。</p>
          <EmptyState v-if="!form.channels.length" message="还没有发行渠道" />
          <div v-for="(channel, index) in form.channels" :key="index" class="product-editor-row">
            <div class="product-actions"><strong>渠道 {{ index + 1 }}</strong><button class="icon-button" type="button" title="移除发行渠道" aria-label="移除发行渠道" @click="form.channels.splice(index, 1)"><Trash2 :size="15" /></button></div>
            <div class="product-form-grid">
              <label class="form-group"><span class="form-label">平台</span><SelectMenu v-model="channel.platform" aria-label="发行平台"><option v-for="item in platforms" :key="item.id" :value="item.id">{{ item.label }}</option></SelectMenu></label>
              <label class="form-group"><span class="form-label">渠道名称</span><input v-model="channel.name" class="form-input" maxlength="128" placeholder="例如：微信正式版" /></label>
              <label class="form-group"><span class="form-label">平台应用标识</span><input v-model="channel.app_identifier" class="form-input" maxlength="256" placeholder="App ID / 包名 / Bundle ID" /></label>
              <label class="product-full-width product-muted"><input type="checkbox" :checked="channel.publish_status === 'paused'" aria-label="暂停发行" @change="setChannelPaused(channel, $event.target.checked)" /> 暂停发行</label>
              <label class="form-group product-full-width"><span class="form-label">管理后台链接</span><input v-model.trim="channel.admin_url" class="form-input" type="url" placeholder="https://" /></label>
              <label class="form-group product-full-width"><span class="form-label">公开访问或商店链接</span><input v-model.trim="channel.public_url" class="form-input" type="url" placeholder="https://" /></label>
              <label class="form-group product-full-width"><span class="form-label">备注</span><textarea v-model="channel.notes" class="form-input" maxlength="1024" /></label>
            </div>
          </div>
          <button class="btn btn-sm product-section-gap" type="button" :disabled="form.channels.length >= 30" @click="form.channels.push({ platform: 'web', name: '', publish_status: 'draft', app_identifier: '', admin_url: '', public_url: '', online_version: '', pending_version: '', notes: '' })">添加发行渠道</button>
        </template>
        <template v-else-if="tab === 'resources'">
          <p class="product-muted">可不关联运维资源。项目中的环境和应用入口沿用已有资料，修改关联不会改变部署。</p>
          <template v-if="options.platform || form.platform_linked">
            <h3 class="product-section-title">管理平台自身</h3>
            <div class="product-checklist"><label><input v-model="form.platform_linked" type="checkbox" aria-label="关联当前管理平台" />{{ options.platform?.name || 'Cylism Manager（当前管理平台）' }}</label></div>
            <p class="product-muted">关联后可在产品版本中选择平台更新记录；平台的发布与回滚仍在系统设置中操作。</p>
          </template>
          <h3 class="product-section-title">运维项目</h3>
          <div v-if="options.projects.length" class="product-checklist"><label v-for="item in options.projects" :key="item.id"><input v-model="form.project_ids" type="checkbox" :value="item.id" />{{ item.name }}</label></div>
          <EmptyState v-else message="暂无可关联的自有项目" />
          <h3 class="product-section-title product-section-gap">指定部署组件（可选）</h3>
          <div v-if="options.applications.length" class="product-checklist"><label v-for="item in options.applications" :key="item.id"><input v-model="form.application_ids" type="checkbox" :value="item.id" />{{ projectName(item.project_id) }} / {{ item.name }} <span class="product-muted">环境 #{{ item.environment_id }}</span></label></div>
          <EmptyState v-else message="暂无可关联的部署组件" />
          <h3 class="product-section-title product-section-gap">数据库资源</h3>
          <p class="product-muted">数据库信息由关联项目或部署组件的数据库绑定自动汇总，无需在产品中重复填写。在数据库管理中登记或创建实例、配置业务连接并绑定应用后，总览会自动展示对应资源。</p>
          <router-link class="btn btn-sm" to="/cloud-services/databases" @click="$emit('close')">数据库管理</router-link>
        </template>
        <div v-else class="product-form-grid">
          <p class="product-muted product-full-width">这些资料用于以后选择公开作品。第一版只保存资料，不会发布到个人主页。</p>
          <label class="form-group product-full-width"><span class="form-label">公开简介</span><textarea v-model="form.public_bio" class="form-input" maxlength="1024" /></label>
          <label class="form-group product-full-width"><span class="form-label">展示链接</span><input v-model.trim="form.public_url" class="form-input" type="url" placeholder="https://" /></label>
          <label class="product-full-width product-muted"><input v-model="form.public_showcase" type="checkbox" /> 计划在个人主页展示</label>
        </div>
      </fieldset>
    </form>
    <template #actions><button class="btn" :disabled="busy" @click="$emit('close')">取消</button><button class="btn btn-primary" :disabled="busy" type="submit" :form="formID">{{ busy ? '保存中...' : product?.id ? '保存' : '创建产品' }}</button></template>
  </BaseModal>
</template>

<script setup>
import { computed, ref, useId, watch } from 'vue'
import { Trash2 } from 'lucide-vue-next'
import BaseModal from '../../components/BaseModal.vue'
import EmptyState from '../../components/EmptyState.vue'
import SectionTabsHeader from '../../components/SectionTabsHeader.vue'
import SelectMenu from '../../components/SelectMenu.vue'
import { editableProduct, stages, platforms, linkKinds } from './product-utils.js'

const props = defineProps({ open: Boolean, busy: Boolean, initialTab: { type: String, default: 'basics' }, product: { type: Object, default: null }, options: { type: Object, default: () => ({ projects: [], applications: [] }) } })
defineEmits(['close', 'save'])
const formID = `product-form-${useId()}`
const form = ref(editableProduct())
const tab = ref('basics')
const tabs = [{ id: 'basics', label: '基本信息', title: '编辑基本信息' }, { id: 'links', label: '资料链接', title: '管理资料链接' }, { id: 'channels', label: '发行渠道', title: '管理发行渠道' }, { id: 'resources', label: '资源关联', title: '管理资源关联' }, { id: 'showcase', label: '公开资料', title: '编辑公开资料' }]
const section = computed(() => tabs.find(item => item.id === tab.value))
watch(() => props.open, open => { if (open) { form.value = editableProduct(props.product || {}); tab.value = tabs.some(item => item.id === props.initialTab) ? props.initialTab : 'basics' } }, { immediate: true })
function setChannelPaused(channel, paused) {
  const original = props.product?.channels?.find(item => item.id === channel.id)?.publish_status
  channel.publish_status = paused ? 'paused' : original && original !== 'paused' ? original : 'draft'
}
function projectName(id) { return props.options.projects.find(item => item.id === id)?.name || `项目 #${id}` }
</script>

<style scoped>
.product-editor-fields { min-width: 0; padding: 0; border: 0; margin: 0; }
</style>
