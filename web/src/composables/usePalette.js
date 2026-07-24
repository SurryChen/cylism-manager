import { computed, ref } from 'vue'

export const PALETTES = ['mint', 'blue', 'orchid', 'sky', 'night']
export const STORAGE_KEY = 'cylism-palette'

function isPalette(value) {
  return PALETTES.includes(value)
}

export function getInitialPalette() {
  const saved = localStorage.getItem(STORAGE_KEY)
  if (isPalette(saved)) return saved

  return window.matchMedia?.('(prefers-color-scheme: dark)').matches ? 'night' : 'mint'
}

export function applyPalette(palette) {
  const resolvedPalette = isPalette(palette) ? palette : 'mint'
  document.documentElement.dataset.palette = resolvedPalette
  return resolvedPalette
}

export function setPalette(palette) {
  const resolvedPalette = applyPalette(palette)
  localStorage.setItem(STORAGE_KEY, resolvedPalette)
  return resolvedPalette
}

export function initializePalette() {
  return applyPalette(getInitialPalette())
}

const activePalette = ref(getInitialPalette())

export function usePalette() {
  function selectPalette(palette) {
    activePalette.value = setPalette(palette)
  }

  return {
    activePalette: computed(() => activePalette.value),
    selectPalette,
  }
}
