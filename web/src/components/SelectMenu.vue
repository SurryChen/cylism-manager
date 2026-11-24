<template>
  <div ref="root" class="select-menu" :class="[rootClasses, { 'is-open': open, 'is-disabled': disabled }]" :style="$attrs.style">
    <button
      class="select-menu-trigger"
      ref="trigger"
      type="button"
      :id="id || undefined"
      :disabled="disabled"
      :aria-label="multiple ? `${ariaLabel}，${effectivePlaceholder}` : ariaLabel"
      :aria-required="required || undefined"
      :aria-expanded="open"
      :aria-haspopup="searchable ? 'dialog' : 'listbox'"
      @click="toggle"
      @keydown="handleTriggerKeydown"
    >
      <span class="select-menu-value" :class="{ 'is-placeholder': !selectedOption && (!multiple || !modelValue.length) }">{{ selectedOption?.triggerLabel || selectedOption?.label || effectivePlaceholder }}</span>
      <ChevronDown :size="14" aria-hidden="true" />
    </button>
    <select v-if="!multiple" v-bind="controlAttrs" :class="['select-menu-native', $attrs.class]" :value="modelValue" :aria-label="ariaLabel" :required="required" :disabled="disabled" tabindex="-1" aria-hidden="true" @change="selectNative">
      <template v-if="hasOptionSlot"><slot /></template>
      <template v-else><option v-if="effectivePlaceholder" value="">{{ effectivePlaceholder }}</option><option v-for="option in normalizedOptions" :key="String(option.value)" :value="option.value" :disabled="option.disabled">{{ option.label }}</option></template>
    </select>
    <Teleport to="body">
      <div v-if="open" ref="menu" class="select-menu-options" :class="{ 'select-menu-searchable': searchable }" :style="menuStyle" :role="searchable ? 'dialog' : 'listbox'" :aria-label="ariaLabel" :aria-multiselectable="!searchable && multiple || undefined" @keydown.esc.stop="closeAndFocusTrigger">
        <input v-if="searchable" ref="searchInput" v-model="search" type="search" class="form-input" :aria-label="searchLabel" :placeholder="searchLabel" @keydown.enter.prevent />
        <div ref="list" class="select-menu-list" :role="searchable ? 'listbox' : undefined" :aria-label="searchable ? ariaLabel : undefined" :aria-multiselectable="searchable && multiple || undefined" @scroll.passive="handleListScroll">
          <button
            v-for="option in normalizedOptions"
            :key="String(option.value)"
            class="select-menu-option"
            :class="{ 'is-selected': isSelected(option.value) }"
            :disabled="option.disabled"
            type="button"
            role="option"
            :aria-selected="isSelected(option.value)"
            @click="select(option.value, option.disabled)"
          >
            <span class="select-menu-option-copy"><span class="select-menu-option-label">{{ option.label }}</span><small v-if="option.description" class="select-menu-option-description">{{ option.description }}</small></span>
            <Check v-if="isSelected(option.value)" :size="14" aria-hidden="true" />
          </button>
        </div>
        <slot name="feedback" />
      </div>
    </Teleport>
  </div>
</template>

<script setup>
import { computed, nextTick, onBeforeUnmount, onMounted, ref, useAttrs, useSlots, watch } from 'vue'
import { Check, ChevronDown } from 'lucide-vue-next'

const props = defineProps({
  modelValue: { type: [String, Number, Array], default: '' },
  multiple: Boolean,
  searchable: Boolean,
  searchLabel: { type: String, default: '搜索选项' },
  options: { type: Array, default: () => [] },
  placeholder: { type: String, default: '' },
  ariaLabel: { type: String, default: '选择' },
  disabled: { type: Boolean, default: false },
  required: { type: Boolean, default: false },
  id: { type: String, default: '' },
  modelModifiers: { type: Object, default: () => ({}) },
})
defineOptions({ inheritAttrs: false })
const emit = defineEmits(['update:modelValue', 'change', 'open', 'load-more'])
const search = defineModel('search', { type: String, default: '' })
const attrs = useAttrs()
const slots = useSlots()
const root = ref(null)
const trigger = ref(null)
const menu = ref(null)
const list = ref(null)
const searchInput = ref(null)
const open = ref(false)
const menuStyle = ref({})
const slotOptions = computed(() => flattenOptionNodes(slots.default?.()).map(node => ({
  value: node.props?.value ?? '',
  label: nodeText(node.children),
  disabled: Boolean(node.props?.disabled),
})))
const hasOptionSlot = computed(() => slotOptions.value.length > 0)
const normalizedOptions = computed(() => (props.options.length ? props.options : slotOptions.value.filter(option => !option.disabled)).map(option => typeof option === 'object' ? option : { value: option, label: String(option) }))
const selectedOption = computed(() => props.multiple ? undefined : normalizedOptions.value.find(option => isSelected(option.value)))
const effectivePlaceholder = computed(() => props.placeholder || slotOptions.value.find(option => option.disabled)?.label || '请选择')
const controlAttrs = computed(() => {
  const { class: ignoredClass, style: ignoredStyle, ...rest } = attrs
  return rest
})
const rootClasses = computed(() => typeof attrs.class === 'string' ? attrs.class.split(/\s+/).filter(name => name && name !== 'form-select') : attrs.class)

function flattenOptionNodes(nodes = []) {
  return nodes.flatMap(node => {
    if (node?.type === 'option') return [node]
    return Array.isArray(node?.children) ? flattenOptionNodes(node.children) : []
  })
}

function nodeText(children) {
  if (typeof children === 'string') return children
  if (!Array.isArray(children)) return ''
  return children.map(child => typeof child === 'string' ? child : nodeText(child?.children)).join('')
}

function toggle() {
  if (!props.disabled) open.value = !open.value
}

function isSelected(value) {
  return props.multiple ? props.modelValue.includes(value) : String(value) === String(props.modelValue)
}

function positionMenu() {
  if (!root.value) return
  const rect = root.value.getBoundingClientRect()
  const triggerWidth = rect.width || root.value.offsetWidth
  const height = menu.value?.offsetHeight || 260
  const below = rect.bottom + 5
  const top = below + height <= window.innerHeight - 8 ? below : Math.max(8, rect.top - height - 5)
  const overlay = root.value.closest('.overlay')
  const overlayZ = overlay ? Number.parseInt(window.getComputedStyle(overlay).zIndex, 10) : 0
  const viewportWidth = window.innerWidth || 1280
  const maxWidth = Math.min(props.searchable ? Math.max(160, triggerWidth) : 420, viewportWidth - 16)
  const left = Math.max(8, Math.min(rect.left, viewportWidth - maxWidth - 8))
  menuStyle.value = {
    top: `${top}px`,
    left: `${left}px`,
    minWidth: `${Math.min(triggerWidth, viewportWidth - 16)}px`,
    maxWidth: `${maxWidth}px`,
    width: props.searchable ? `${maxWidth}px` : 'max-content',
    zIndex: String(Math.max(1300, (overlayZ || 0) + 1)),
  }
}

function select(value, disabled = false) {
  if (disabled || props.disabled) return
  if (props.multiple) {
    const next = isSelected(value) ? props.modelValue.filter(item => item !== value) : [...props.modelValue, value]
    emit('update:modelValue', next)
    emit('change', next)
    return
  }
  emit('update:modelValue', value)
  emit('change', value)
  open.value = false
}

function selectNative(event) {
  const selected = normalizedOptions.value.find(option => String(option.value) === event.target.value)
  const value = selected ? selected.value : event.target.value
  select(props.modelModifiers.number && typeof value === 'string' && value !== '' ? Number(value) : value)
}

function handleTriggerKeydown(event) {
  if (event.key === 'Escape' && open.value) { event.stopPropagation(); closeAndFocusTrigger(); return }
  if (event.key === 'Escape') { open.value = false; return }
  if ((event.key === 'Enter' || event.key === ' ') && !open.value) { event.preventDefault(); open.value = true }
  if (event.key === 'ArrowDown' && !open.value) { event.preventDefault(); open.value = true }
}

function closeAndFocusTrigger() {
  open.value = false
  trigger.value?.focus()
}

function handleListScroll(event) {
  const element = event.currentTarget
  if (element.scrollHeight - element.scrollTop - element.clientHeight < 48) emit('load-more')
}

function onDocumentClick(event) {
  if (open.value && root.value && !root.value.contains(event.target) && !menu.value?.contains(event.target)) open.value = false
}

onMounted(() => {
  document.addEventListener('click', onDocumentClick)
  if (props.searchable) document.addEventListener('focusin', onDocumentClick)
})
watch(search, () => { if (list.value) list.value.scrollTop = 0 })
watch(() => props.disabled, disabled => { if (disabled) open.value = false })
watch(() => normalizedOptions.value.length, () => { if (open.value && props.searchable) positionMenu() }, { flush: 'post' })
watch(open, value => {
  if (value) {
    positionMenu()
    window.addEventListener('resize', positionMenu)
    window.addEventListener('scroll', positionMenu, true)
    emit('open')
    if (props.searchable) void nextTick(() => searchInput.value?.focus())
  } else {
    window.removeEventListener('resize', positionMenu)
    window.removeEventListener('scroll', positionMenu, true)
  }
}, { flush: 'post' })
onBeforeUnmount(() => {
  document.removeEventListener('click', onDocumentClick)
  document.removeEventListener('focusin', onDocumentClick)
  window.removeEventListener('resize', positionMenu)
  window.removeEventListener('scroll', positionMenu, true)
})
</script>

<style scoped>
.select-menu { position: relative; min-width: 0; }
.select-menu-trigger { display: flex; width: 100%; min-height: var(--button-height); align-items: center; justify-content: space-between; gap: 8px; padding: 7px 10px; border: 1px solid var(--border-muted); border-radius: var(--radius-control); background: var(--surface-input); color: var(--text-primary); font: inherit; font-size: 12px; text-align: left; cursor: pointer; transition: border-color .18s ease, background .18s ease, box-shadow .18s ease; }
.select-menu-trigger:hover { border-color: var(--action-primary); background: var(--surface-hover); }
.select-menu-trigger:focus-visible, .select-menu.is-open .select-menu-trigger { outline: 2px solid var(--focus); outline-offset: 1px; border-color: var(--action-primary); }
.select-menu-trigger svg { flex: 0 0 auto; color: var(--text-muted); transition: transform .18s ease; }
.select-menu.is-open .select-menu-trigger svg { color: var(--action-primary); transform: rotate(180deg); }
.select-menu-value { min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.select-menu-value.is-placeholder { color: var(--text-muted); }
.select-menu-native { position: absolute; width: 1px; height: 1px; overflow: hidden; clip: rect(0, 0, 0, 0); white-space: nowrap; }
.select-menu-options { position: fixed; z-index: 1300; display: grid; width: max-content; max-height: 260px; gap: 2px; overflow-y: auto; padding: 5px; border: 1px solid var(--border); border-radius: var(--radius-control); background: var(--surface-raised); box-shadow: var(--shadow); backdrop-filter: blur(24px) saturate(140%); }
.select-menu-list { display: grid; gap: 2px; }
.select-menu-searchable { grid-template-rows: auto minmax(0, 1fr) auto; gap: var(--space-4); overflow: hidden; }
.select-menu-searchable .select-menu-list { min-width: 0; overflow-y: auto; overscroll-behavior: contain; }
.select-menu-searchable input { min-height: var(--button-height); font-size: 12px; }
.select-menu-option { display: flex; width: 100%; min-width: 0; align-items: center; justify-content: space-between; gap: 12px; padding: 8px 9px; border: 0; border-radius: 6px; background: transparent; color: var(--text-secondary); font: inherit; font-size: 12px; text-align: left; cursor: pointer; }
.select-menu-option:hover, .select-menu-option.is-selected { background: var(--surface-hover); color: var(--text-primary); }
.select-menu-option:disabled { cursor: not-allowed; opacity: .55; }
.select-menu-option-copy { display: grid; min-width: 0; gap: 2px; }
.select-menu-option-label, .select-menu-option-description { min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.select-menu-option-description { color: var(--text-muted); font-size: 10px; font-weight: 400; }
.select-menu-option svg { flex: 0 0 auto; color: var(--action-primary); }
.is-disabled .select-menu-trigger { cursor: not-allowed; opacity: .5; }
</style>
