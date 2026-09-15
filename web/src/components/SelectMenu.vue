<template>
  <div ref="root" class="select-menu" :class="{ 'is-open': open, 'is-disabled': disabled }">
    <button
      class="select-menu-trigger"
      type="button"
      :disabled="disabled"
      :aria-label="ariaLabel"
      :aria-expanded="open"
      aria-haspopup="listbox"
      @click="toggle"
      @keydown="handleTriggerKeydown"
    >
      <span class="select-menu-value" :class="{ 'is-placeholder': !selectedOption }">{{ selectedOption?.label || placeholder }}</span>
      <ChevronDown :size="14" aria-hidden="true" />
    </button>
    <select class="select-menu-native" :value="modelValue" :aria-label="ariaLabel" tabindex="-1" aria-hidden="true" @change="selectNative">
      <option v-if="placeholder" value="">{{ placeholder }}</option>
      <option v-for="option in normalizedOptions" :key="String(option.value)" :value="option.value">{{ option.label }}</option>
    </select>
    <div v-if="open" class="select-menu-options" role="listbox" :aria-label="ariaLabel">
      <button
        v-for="option in normalizedOptions"
        :key="String(option.value)"
        class="select-menu-option"
        :class="{ 'is-selected': String(option.value) === String(modelValue) }"
        type="button"
        role="option"
        :aria-selected="String(option.value) === String(modelValue)"
        @click="select(option.value)"
      >
        <span>{{ option.label }}</span>
        <Check v-if="String(option.value) === String(modelValue)" :size="14" aria-hidden="true" />
      </button>
    </div>
  </div>
</template>

<script setup>
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { Check, ChevronDown } from 'lucide-vue-next'

const props = defineProps({
  modelValue: { type: [String, Number], default: '' },
  options: { type: Array, default: () => [] },
  placeholder: { type: String, default: '请选择' },
  ariaLabel: { type: String, default: '选择' },
  disabled: { type: Boolean, default: false },
})
const emit = defineEmits(['update:modelValue', 'change'])
const root = ref(null)
const open = ref(false)
const normalizedOptions = computed(() => props.options.map(option => typeof option === 'object' ? option : { value: option, label: String(option) }))
const selectedOption = computed(() => normalizedOptions.value.find(option => String(option.value) === String(props.modelValue)))

function toggle() {
  if (!props.disabled) open.value = !open.value
}

function select(value) {
  emit('update:modelValue', value)
  emit('change', value)
  open.value = false
}

function selectNative(event) { select(event.target.value) }

function handleTriggerKeydown(event) {
  if (event.key === 'Escape') { open.value = false; return }
  if ((event.key === 'Enter' || event.key === ' ') && !open.value) { event.preventDefault(); open.value = true }
  if (event.key === 'ArrowDown' && !open.value) { event.preventDefault(); open.value = true }
}

function onDocumentClick(event) {
  if (open.value && root.value && !root.value.contains(event.target)) open.value = false
}

onMounted(() => document.addEventListener('click', onDocumentClick))
onBeforeUnmount(() => document.removeEventListener('click', onDocumentClick))
</script>

<style scoped>
.select-menu { position: relative; min-width: 0; }
.select-menu-trigger { display: flex; width: 100%; min-height: 36px; align-items: center; justify-content: space-between; gap: 8px; padding: 7px 10px; border: 1px solid var(--border-muted); border-radius: var(--radius-control); background: var(--surface-input); color: var(--text-primary); font: inherit; font-size: 12px; text-align: left; cursor: pointer; transition: border-color .18s ease, background .18s ease, box-shadow .18s ease; }
.select-menu-trigger:hover { border-color: var(--action-primary); background: var(--surface-hover); }
.select-menu-trigger:focus-visible, .select-menu.is-open .select-menu-trigger { outline: 2px solid var(--focus); outline-offset: 1px; border-color: var(--action-primary); }
.select-menu-trigger svg { flex: 0 0 auto; color: var(--text-muted); transition: transform .18s ease; }
.select-menu.is-open .select-menu-trigger svg { color: var(--action-primary); transform: rotate(180deg); }
.select-menu-value { min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.select-menu-value.is-placeholder { color: var(--text-muted); }
.select-menu-native { position: absolute; width: 1px; height: 1px; overflow: hidden; clip: rect(0, 0, 0, 0); white-space: nowrap; }
.select-menu-options { position: absolute; z-index: 30; top: calc(100% + 5px); right: 0; left: 0; display: grid; max-height: 260px; gap: 2px; overflow-y: auto; padding: 5px; border: 1px solid var(--border); border-radius: var(--radius-control); background: var(--surface-raised); box-shadow: var(--shadow); backdrop-filter: blur(24px) saturate(140%); }
.select-menu-option { display: flex; width: 100%; min-width: 0; align-items: center; justify-content: space-between; gap: 12px; padding: 8px 9px; border: 0; border-radius: 6px; background: transparent; color: var(--text-secondary); font: inherit; font-size: 12px; text-align: left; cursor: pointer; }
.select-menu-option:hover, .select-menu-option.is-selected { background: var(--surface-hover); color: var(--text-primary); }
.select-menu-option span { min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.select-menu-option svg { flex: 0 0 auto; color: var(--action-primary); }
.is-disabled .select-menu-trigger { cursor: not-allowed; opacity: .5; }
</style>
