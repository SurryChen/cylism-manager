<template>
  <th v-if="header" class="table-action-column" scope="col">
    <slot>操作</slot>
  </th>
  <td v-else class="table-action-column">
    <div class="table-action-buttons"><slot /></div>
  </td>
</template>

<script setup>
defineProps({
  header: Boolean,
})
</script>
