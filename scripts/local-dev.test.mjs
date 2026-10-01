import assert from 'node:assert/strict'
import { spawnSync } from 'node:child_process'
import { copyFileSync, existsSync, mkdtempSync, mkdirSync, readFileSync, rmSync, statSync, writeFileSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { join, resolve } from 'node:path'
import { test } from 'node:test'

const script = resolve('scripts/local-dev.sh')

function fixture() {
  const root = mkdtempSync(join(tmpdir(), 'cylism-local-dev-'))
  mkdirSync(join(root, 'scripts'))
  mkdirSync(join(root, 'web', 'node_modules', 'vite'), { recursive: true })
  mkdirSync(join(root, 'bin'))
  copyFileSync(script, join(root, 'scripts', 'local-dev.sh'))
  writeFileSync(join(root, 'bin', 'go'), '#!/bin/sh\nprintf "%s\\n" "$@" > "$LOCAL_DEV_TEST_OUTPUT"\n', { mode: 0o755 })
  writeFileSync(join(root, 'bin', 'npm'), '#!/bin/sh\nif [ "$3" = "ci" ]; then : > "$LOCAL_DEV_INSTALL_MARKER"; fi\nprintf "%s\\n" "$VITE_API_PROXY_TARGET" "$@" > "$LOCAL_DEV_TEST_OUTPUT"\n', { mode: 0o755 })
  const output = join(root, 'output.txt')
  const installMarker = join(root, 'npm-installed')
  return {
    root,
    output,
    installMarker,
    run(...args) {
      return spawnSync('bash', [join(root, 'scripts', 'local-dev.sh'), ...args], {
        encoding: 'utf8',
        env: { ...process.env, PATH: `${join(root, 'bin')}:${process.env.PATH}`, LOCAL_DEV_TEST_OUTPUT: output, LOCAL_DEV_INSTALL_MARKER: installMarker },
      })
    },
    cleanup() { rmSync(root, { recursive: true, force: true }) },
  }
}

test('creates a private local config once and starts the API in the foreground', () => {
  const app = fixture()
  try {
    const first = app.run('api')
    assert.equal(first.status, 0, first.stderr)
    const configPath = join(app.root, 'config', 'config.yaml')
    const config = readFileSync(configPath, 'utf8')
    assert.match(config, /key: "[0-9a-f]{32}"/)
    assert.match(config, /admin_password: "[0-9a-f]{24}"/)
    assert.match(config, /path: "\.\/data\/cylism\.db"/)
    assert.equal(statSync(configPath).mode & 0o777, 0o600)
    assert.match(readFileSync(app.output, 'utf8'), /run\n\.\/cmd\/platform/)

    assert.equal(app.run('api').status, 0)
    assert.equal(readFileSync(configPath, 'utf8'), config)
  } finally {
    app.cleanup()
  }
})

test('web defaults to the local API and accepts a remote override', () => {
  const app = fixture()
  try {
    assert.equal(app.run('web').status, 0)
    assert.match(readFileSync(app.output, 'utf8'), /^http:\/\/127\.0\.0\.1:8080\n/)
    assert.equal(app.run('web', '--remote', 'https://example.com').status, 0)
    assert.match(readFileSync(app.output, 'utf8'), /^https:\/\/example\.com\n/)
    assert.equal(app.run('web', '--remote', 'not-a-url').status, 2)
  } finally {
    app.cleanup()
  }
})

test('installs missing web dependencies before starting the foreground server', () => {
  const app = fixture()
  try {
    rmSync(join(app.root, 'web', 'node_modules'), { recursive: true })
    assert.equal(app.run('web').status, 0)
    assert.equal(existsSync(app.installMarker), true)
    assert.match(readFileSync(app.output, 'utf8'), /^http:\/\/127\.0\.0\.1:8080\n/)
  } finally {
    app.cleanup()
  }
})
