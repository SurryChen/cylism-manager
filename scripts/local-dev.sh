#!/usr/bin/env bash
set -euo pipefail

root_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$root_dir"

usage() {
  cat <<'EOF'
用法:
  scripts/local-dev.sh init
  scripts/local-dev.sh api
  scripts/local-dev.sh web [--remote <http(s)://地址>]

api 和 web 分别在当前终端以前台运行。web 默认连接本地 127.0.0.1:8080。
EOF
}

require_command() {
  if ! command -v "$1" >/dev/null 2>&1; then
    printf '缺少 %s，请先安装后重试。\n' "$1" >&2
    exit 1
  fi
}

ensure_config() {
  local config_file="$root_dir/config/config.yaml"
  mkdir -p "$root_dir/data"
  if [[ -f "$config_file" ]]; then
    printf '使用现有本地配置：%s\n' "$config_file"
    return
  fi

  require_command openssl
  local encryption_key jwt_secret admin_password
  encryption_key="$(openssl rand -hex 16)"
  jwt_secret="$(openssl rand -hex 32)"
  admin_password="$(openssl rand -hex 12)"
  mkdir -p "$root_dir/config"
  umask 077
  printf 'server:\n  host: "127.0.0.1"\n  port: 8080\n\ndatabase:\n  path: "./data/cylism.db"\n\nencryption:\n  key: "%s"\n\nauth:\n  admin_user: "admin"\n  admin_password: "%s"\n  jwt_secret: "%s"\n' \
    "$encryption_key" "$admin_password" "$jwt_secret" > "$config_file"
  printf '已创建本地配置：%s（管理员密码见 auth.admin_password）\n' "$config_file"
}

ensure_web_dependencies() {
  require_command node
  require_command npm
  if [[ ! -d "$root_dir/web/node_modules/vite" ]]; then
    printf '正在安装 Web 依赖...\n'
    npm --prefix web ci
  fi
}

case "${1:-}" in
  init)
    [[ $# -eq 1 ]] || { usage >&2; exit 2; }
    require_command go
    ensure_config
    ensure_web_dependencies
    printf '本地环境已就绪。\n'
    ;;
  api)
    [[ $# -eq 1 ]] || { usage >&2; exit 2; }
    require_command go
    ensure_config
    mkdir -p "$root_dir/data"
    printf '启动 API：http://127.0.0.1:${SERVER_PORT:-8080}\n'
    exec go run ./cmd/platform
    ;;
  web)
    proxy_target='http://127.0.0.1:8080'
    if [[ $# -eq 3 && "$2" == '--remote' ]]; then
      proxy_target="$3"
      if [[ ! "$proxy_target" =~ ^https?://[^[:space:]]+$ ]]; then
        printf '远端地址必须是 http:// 或 https:// 开头的 URL。\n' >&2
        exit 2
      fi
    elif [[ $# -ne 1 ]]; then
      usage >&2
      exit 2
    fi
    ensure_web_dependencies
    export VITE_API_PROXY_TARGET="$proxy_target"
    printf '启动 Web，API 目标：%s\n' "$proxy_target"
    exec npm --prefix web run dev -- --host 127.0.0.1
    ;;
  -h|--help|help)
    usage
    ;;
  *)
    usage >&2
    exit 2
    ;;
esac
