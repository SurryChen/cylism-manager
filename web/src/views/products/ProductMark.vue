<template>
  <span class="product-mark" aria-hidden="true">
    <img v-if="url && !failed" :key="url" :src="url" alt="" loading="lazy" referrerpolicy="no-referrer" @error="failed = true" />
    <template v-else>{{ product.name.slice(0, 1) }}</template>
  </span>
</template>

<script setup>
import { computed, ref, watch } from 'vue'
import { webURL } from './product-utils.js'

const props = defineProps({ product: { type: Object, required: true } })
const url = computed(() => webURL(props.product.icon_url)?.href)
const failed = ref(false)
watch(url, () => { failed.value = false })
</script>
