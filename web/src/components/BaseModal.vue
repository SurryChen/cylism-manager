<template>
  <div
    v-if="open"
    class="overlay base-modal-overlay"
    :class="overlayClass"
    @click.self="handleOverlayClick"
  >
    <section
      class="modal base-modal"
      :class="[`base-modal--${size}`, dialogClass]"
      role="dialog"
      aria-modal="true"
      :aria-labelledby="title && !$slots.header ? titleID : undefined"
      :aria-label="title && $slots.header ? title : (title ? undefined : ariaLabel)"
    >
      <header v-if="title || $slots.header" class="base-modal-header">
        <slot name="header">
          <h2 :id="titleID" class="modal-title">{{ title }}</h2>
        </slot>
        <button
          v-if="showClose"
          type="button"
          class="icon-button base-modal-close"
          aria-label="关闭"
          @click="emit('close')"
        >
          ×
        </button>
      </header>
      <button
        v-else-if="showClose"
        type="button"
        class="icon-button base-modal-close base-modal-close--standalone"
        aria-label="关闭"
        @click="emit('close')"
      >
        ×
      </button>
      <div class="base-modal-body">
        <slot />
      </div>
      <footer v-if="$slots.actions" class="modal-actions base-modal-actions">
        <slot name="actions" />
      </footer>
    </section>
  </div>
</template>

<script setup>
import { onBeforeUnmount, watch } from 'vue'

let modalID = 0
const props = defineProps({
  open: { type: Boolean, default: false },
  title: { type: String, default: '' },
  ariaLabel: { type: String, default: '对话框' },
  size: { type: String, default: 'medium', validator: value => ['small', 'medium', 'large'].includes(value) },
  showClose: { type: Boolean, default: true },
  closeOnOverlay: { type: Boolean, default: true },
  closeOnEscape: { type: Boolean, default: true },
  overlayClass: { type: [String, Array, Object], default: '' },
  dialogClass: { type: [String, Array, Object], default: '' },
})
const emit = defineEmits(['close'])
const titleID = `base-modal-title-${++modalID}`

function handleOverlayClick() {
  if (props.closeOnOverlay) emit('close')
}

function onKeydown(event) {
  if (event.key === 'Escape' && props.open && props.closeOnEscape) emit('close')
}

watch(() => props.open, open => {
  if (open) window.addEventListener('keydown', onKeydown)
  else window.removeEventListener('keydown', onKeydown)
}, { immediate: true })

onBeforeUnmount(() => window.removeEventListener('keydown', onKeydown))
</script>

<style scoped>
.base-modal-overlay {
  z-index: 1200;
}

.base-modal {
  position: relative;
  display: flex;
  flex-direction: column;
  width: min(540px, 100%);
  max-height: calc(100dvh - 32px);
  padding: var(--space-24);
}

.base-modal--small {
  width: min(420px, 100%);
}

.base-modal--large {
  width: min(760px, 100%);
}

.base-modal-header {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: var(--space-16);
}

.base-modal-header .modal-title {
  min-width: 0;
  margin-bottom: var(--space-16);
}

.base-modal-body {
  min-width: 0;
  overflow-y: auto;
  overscroll-behavior: contain;
}

.base-modal-close {
  flex: 0 0 auto;
}

.base-modal-close--standalone {
  position: absolute;
  top: 14px;
  right: 14px;
}

.base-modal-actions {
  flex: 0 0 auto;
}

@media (max-width: 640px) {
  .base-modal {
    padding: 18px;
  }
}
</style>
