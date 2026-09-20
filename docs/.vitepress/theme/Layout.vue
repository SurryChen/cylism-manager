<template>
  <DefaultTheme.Layout>
    <template #aside-top>
      <div class="cylism-drawer-brand">
        <a class="cylism-drawer-logo" href="/" aria-label="Cylism Manager 首页">
          <img src="/assets/cylism-manager-icon.svg" alt="" />
        </a>
        <strong>Cylism Manager</strong>
      </div>
      <a
        class="cylism-drawer-source"
        href="https://github.com/SurryChen/cylism-manager"
        target="_blank"
        rel="noreferrer"
      >
        <span class="cylism-drawer-source__icon" aria-hidden="true">◆</span>
        <span>
          <strong>SurryChen/cylism-manager</strong>
          <small>GitHub 仓库</small>
        </span>
      </a>
    </template>
    <template #sidebar-nav-before>
      <div class="cylism-sidebar-brand">
        <img src="/assets/cylism-manager-icon.svg" alt="" />
        <span>
          <strong>Cylism Manager</strong>
          <small>产品文档</small>
        </span>
      </div>
    </template>
    <template #nav-screen-content-before>
      <button class="cylism-drawer-overlay" type="button" aria-label="关闭导航" @click="closeScreen" />
      <div class="cylism-drawer-brand">
        <a class="cylism-drawer-logo" href="/" aria-label="Cylism Manager 首页">
          <img src="/assets/cylism-manager-icon.svg" alt="" />
        </a>
        <strong>Cylism Manager</strong>
      </div>
      <a
        class="cylism-drawer-source"
        href="https://github.com/SurryChen/cylism-manager"
        target="_blank"
        rel="noreferrer"
      >
        <span class="cylism-drawer-source__icon" aria-hidden="true">◆</span>
        <span>
          <strong>SurryChen/cylism-manager</strong>
          <small>GitHub 仓库</small>
        </span>
      </a>
    </template>
  </DefaultTheme.Layout>
</template>

<script setup lang="ts">
import DefaultTheme from 'vitepress/theme'

function closeScreen() {
  document.querySelector<HTMLButtonElement>('.VPNavBarHamburger')?.click()
}
</script>
