<template>
  <div>
    <label class="check-row"><input v-model="form.enabled" type="checkbox" aria-label="发布前执行数据库迁移" />发布前执行数据库迁移</label>
    <p class="form-hint">使用本次发布镜像运行独立任务，成功后再更新应用。启用后，每次手动发布和自动回调都必须提供迁移目标版本。</p>
    <template v-if="form.enabled">
      <label class="form-group"><span class="form-label">迁移命令</span><textarea v-model="form.command_text" class="form-input migration-command" rows="3" required aria-label="迁移命令" placeholder="/usr/local/bin/cylism-migrate" /><span class="form-hint">每行一个命令或参数，直接执行，不经过 Shell。镜像需包含迁移工具及全部历史 SQL 文件。</span></label>
      <div class="form-row">
        <label class="form-group"><span class="form-label">镜像内脚本目录</span><input v-model.trim="form.directory" class="form-input" required pattern="/.*" aria-label="迁移脚本目录" placeholder="/app/migrations" /></label>
        <label class="form-group"><span class="form-label">数据库变量前缀</span><input v-model.trim="form.database_prefix" class="form-input" required pattern="[A-Za-z_][A-Za-z0-9_]{0,47}" aria-label="迁移数据库变量前缀" placeholder="DATABASE" /></label>
      </div>
      <label class="form-group"><span class="form-label">执行超时（秒）</span><input v-model.number="form.timeout_seconds" class="form-input" type="number" min="30" max="3600" required aria-label="迁移超时秒数" /></label>
      <p class="form-hint">连接复用模板中的 {{ form.database_prefix || 'DATABASE' }}_HOST / PORT / NAME / USER / PASSWORD。请在模板配置中填写连接信息，密码放在敏感配置中。迁移不挂载 PVC，请将脚本打入镜像。</p>
      <p class="form-hint">SQL 失败可能已部分生效，平台会中止发布。内置工具保留未完成标记，修复并核实数据库后才能重试；应用回滚不会撤销 SQL。</p>
    </template>
  </div>
</template>

<script setup>
defineProps({ form: { type: Object, required: true } })
</script>

<style scoped>
.migration-command { font-family:var(--font-mono); line-height:1.6; }
</style>
