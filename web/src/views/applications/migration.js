export function migrationForm(migration) {
  return {
    enabled: !!migration?.enabled,
    command_text: (migration?.command || ['/usr/local/bin/cylism-migrate']).join('\n'),
    directory: migration?.directory || '/app/migrations',
    database_prefix: migration?.database_prefix || 'DATABASE',
    timeout_seconds: migration?.timeout_seconds || 600,
  }
}

export function migrationPayload(form) {
  if (!form.enabled) return undefined
  return {
    enabled: true,
    command: form.command_text.split('\n').map(value => value.trim()).filter(Boolean),
    directory: form.directory.trim(),
    database_prefix: form.database_prefix.trim(),
    timeout_seconds: Number(form.timeout_seconds),
  }
}

export function releasePayload(form, template) {
  return {
    template_id: form.template_id,
    version: form.version,
    ...(template?.spec?.migration?.enabled ? { migration: { target: form.migration_target.trim() } } : {}),
  }
}
export const releaseStepLabels = {
  validate_image: '检查镜像', preflight: '发布前检查', database_migration: '数据库迁移',
  apply_resources: '更新应用', sync_endpoints: '同步访问入口', wait_ready: '等待应用就绪',
  interrupted: '发布中断',
}
