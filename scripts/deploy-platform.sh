#!/usr/bin/env bash
if [ -z "${BASH_VERSION:-}" ]; then exec bash "$0" "$@"; fi
set -Eeuo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
MANIFEST="${MANIFEST:-$ROOT_DIR/k8s/platform-deployment.yaml}"
NAMESPACE="${NAMESPACE:-default}"
DEPLOYMENT="${DEPLOYMENT:-cylism-manager}"
CONTAINER="${CONTAINER:-platform}"
IMAGE="${CYLISM_IMAGE:-}"
NODE_NAME="${CYLISM_NODE_NAME:-}"
SSH_KEY_PATH="${CYLISM_SSH_KEY_PATH:-${HOME:-}/.ssh/id_ed25519}"
IMAGE_PULL_SECRET="${CYLISM_IMAGE_PULL_SECRET:-}"

usage() {
  cat <<'EOF'
Usage: scripts/deploy-platform.sh [options]
  --image IMAGE              Image reference (prompted if omitted)
  --namespace NAME           Kubernetes namespace (default: default)
  --node NAME                Node selector for a fresh deployment
  --ssh-key PATH             SSH private key used by the Manager
  --image-pull-secret NAME   Existing imagePullSecret name
  --manifest PATH            Kubernetes manifest path
  -h, --help                 Show this help

Existing cylism-secret values are reused. Missing values are generated or
requested interactively; existing values are never rotated.
EOF
}

die() { echo "错误: $*" >&2; exit 1; }
need_cmd() { command -v "$1" >/dev/null 2>&1 || die "缺少命令: $1"; }

while [ "$#" -gt 0 ]; do
  case "$1" in
    --image) IMAGE="${2:?--image 需要参数}"; shift 2 ;;
    --namespace) NAMESPACE="${2:?--namespace 需要参数}"; shift 2 ;;
    --node) NODE_NAME="${2:?--node 需要参数}"; shift 2 ;;
    --ssh-key) SSH_KEY_PATH="${2:?--ssh-key 需要参数}"; shift 2 ;;
    --image-pull-secret) IMAGE_PULL_SECRET="${2:?--image-pull-secret 需要参数}"; shift 2 ;;
    --manifest) MANIFEST="${2:?--manifest 需要参数}"; shift 2 ;;
    -h|--help) usage; exit 0 ;;
    *) die "未知参数: $1（使用 --help 查看用法）" ;;
  esac
done

need_cmd base64
need_cmd openssl
need_cmd curl
[ -f "$MANIFEST" ] || die "找不到 Kubernetes 清单: $MANIFEST"

if command -v kubectl >/dev/null 2>&1; then
  KUBECTL=(kubectl)
elif command -v k3s >/dev/null 2>&1; then
  KUBECTL=(k3s kubectl)
elif [ -x /usr/local/bin/k3s ]; then
  KUBECTL=(/usr/local/bin/k3s kubectl)
else
  read -r -p "未检测到 K3s/kubectl，是否安装 K3s？[Y/n] " answer
  [[ "${answer:-Y}" =~ ^[Nn]$ ]] && die "请先安装 K3s 或 kubectl"
  curl -sfL https://get.k3s.io | sh -
  KUBECTL=(/usr/local/bin/k3s kubectl)
  sleep 5
fi

k() { "${KUBECTL[@]}" "$@"; }
k get nodes >/dev/null 2>&1 || die "无法访问 Kubernetes 集群，请检查 kubeconfig 或权限"
k get namespace "$NAMESPACE" >/dev/null 2>&1 || k create namespace "$NAMESPACE" >/dev/null

if ! k get crd certificates.cert-manager.io >/dev/null 2>&1; then
  read -r -p "未检测到 cert-manager，是否安装？[Y/n] " answer
  if [[ ! "${answer:-Y}" =~ ^[Nn]$ ]]; then
    echo "安装 cert-manager..."
    curl -fsSL https://github.com/cert-manager/cert-manager/releases/latest/download/cert-manager.yaml | k apply -f - >/dev/null
    k -n cert-manager wait --for=condition=Available deployment/cert-manager --timeout=180s >/dev/null
  else
    echo "跳过 cert-manager 安装；证书相关功能暂不可用。"
  fi
fi

decode_b64() {
  if base64 --help 2>&1 | grep -q -- '-d'; then base64 -d; else base64 -D; fi
}
b64() {
  if base64 --help 2>&1 | grep -q -- '-w'; then printf '%s' "$1" | base64 -w0; else printf '%s' "$1" | base64 | tr -d '\n'; fi
}
secret_value() {
  local key="$1" value
  value="$(k -n "$NAMESPACE" get secret cylism-secret -o "jsonpath={.data['$key']}" 2>/dev/null || true)"
  [ -n "$value" ] || return 0
  printf '%s' "$value" | decode_b64
}

encryption_key="$(secret_value encryption-key || true)"
jwt_secret="$(secret_value jwt-secret || true)"
admin_password="$(secret_value admin-password || true)"
if [ -z "$encryption_key" ]; then
  echo "cylism-secret 缺少 encryption-key，将生成新的 AES-256 密钥。"
  encryption_key="$(openssl rand -hex 16)"
fi
[ "${#encryption_key}" -eq 32 ] || die "encryption-key 必须是 32 字节"
if [ -z "$jwt_secret" ]; then
  echo "cylism-secret 缺少 jwt-secret，将生成新的 JWT 密钥。"
  jwt_secret="$(openssl rand -hex 32)"
fi
if [ -z "$admin_password" ]; then
  while :; do
    read -r -s -p "请输入首次管理员密码（不会回显）: " admin_password; echo
    [ -n "$admin_password" ] && break
    echo "管理员密码不能为空。"
  done
else
  echo "复用现有 cylism-secret。"
fi

{
  echo 'apiVersion: v1'
  echo 'kind: Secret'
  echo 'metadata:'
  echo '  name: cylism-secret'
  printf '  namespace: %s\n' "$NAMESPACE"
  echo 'type: Opaque'
  echo 'data:'
  printf '  encryption-key: %s\n' "$(b64 "$encryption_key")"
  printf '  jwt-secret: %s\n' "$(b64 "$jwt_secret")"
  printf '  admin-password: %s\n' "$(b64 "$admin_password")"
} | k apply -f - >/dev/null

read_config() {
  k -n "$NAMESPACE" get configmap cylism-config -o "jsonpath={.data['$1']}" 2>/dev/null || true
}
public_url="$(read_config public-url)"
if [ -z "$public_url" ]; then read -r -p "公开访问地址（可留空）: " public_url; fi
admin_user="$(read_config admin-user)"; admin_user="${admin_user:-admin}"
access_ttl="$(read_config access-token-ttl)"; access_ttl="${access_ttl:-7200}"
refresh_ttl="$(read_config refresh-token-ttl)"; refresh_ttl="${refresh_ttl:-604800}"
retention_days="$(read_config operation-log-retention-days)"; retention_days="${retention_days:-30}"
{
  echo 'apiVersion: v1'
  echo 'kind: ConfigMap'
  echo 'metadata:'
  echo '  name: cylism-config'
  printf '  namespace: %s\n' "$NAMESPACE"
  echo 'data:'
  printf '  admin-user: %s\n' "$admin_user"
  printf '  public-url: %s\n' "$public_url"
  printf '  access-token-ttl: %s\n' "$access_ttl"
  printf '  refresh-token-ttl: %s\n' "$refresh_ttl"
  printf '  operation-log-retention-days: %s\n' "$retention_days"
} | k apply -f - >/dev/null

if [ -z "$IMAGE" ]; then IMAGE="$(k -n "$NAMESPACE" get deployment "$DEPLOYMENT" -o "jsonpath={.spec.template.spec.containers[?(@.name=='$CONTAINER')].image}" 2>/dev/null || true)"; fi
if [ -z "$IMAGE" ]; then read -r -p "请输入要部署的镜像地址: " IMAGE; fi
[ -n "$IMAGE" ] || die "镜像地址不能为空"

if ! k -n "$NAMESPACE" get secret cylism-ssh-key >/dev/null 2>&1; then
  [ -f "$SSH_KEY_PATH" ] || die "找不到 SSH 私钥 $SSH_KEY_PATH，请使用 --ssh-key 指定路径"
  echo "创建 cylism-ssh-key（仅首次执行）..."
  k -n "$NAMESPACE" create secret generic cylism-ssh-key --from-file=id_ed25519="$SSH_KEY_PATH" --dry-run=client -o yaml | k apply -f - >/dev/null
else
  echo "复用现有 cylism-ssh-key。"
fi

echo "应用 Kubernetes 清单..."
k -n "$NAMESPACE" apply -f "$MANIFEST" >/dev/null
k -n "$NAMESPACE" set image "deployment/$DEPLOYMENT" "$CONTAINER=$IMAGE" >/dev/null
if [ -n "$IMAGE_PULL_SECRET" ]; then
  k -n "$NAMESPACE" patch deployment "$DEPLOYMENT" --type merge -p "{\"spec\":{\"template\":{\"spec\":{\"imagePullSecrets\":[{\"name\":\"$IMAGE_PULL_SECRET\"}]}}}}" >/dev/null
fi

if [ -z "$NODE_NAME" ]; then NODE_NAME="$(k -n "$NAMESPACE" get deployment "$DEPLOYMENT" -o jsonpath='{.spec.template.spec.nodeSelector.kubernetes\.io/hostname}' 2>/dev/null || true)"; fi
if [ -z "$NODE_NAME" ]; then
  NODE_NAME="$(k get nodes -o jsonpath='{.items[0].metadata.name}')"
  echo "新环境使用节点: $NODE_NAME"
  k -n "$NAMESPACE" patch deployment "$DEPLOYMENT" --type merge -p "{\"spec\":{\"template\":{\"spec\":{\"nodeSelector\":{\"kubernetes.io/hostname\":\"$NODE_NAME\"}}}}}" >/dev/null
fi

# A mutable tag such as :latest may be unchanged between releases. Restarting
# explicitly makes kubelet pull the current image (the manifest uses
# imagePullPolicy: Always) instead of leaving the old Pod running.
k -n "$NAMESPACE" rollout restart "deployment/$DEPLOYMENT" >/dev/null

echo "等待 $DEPLOYMENT rollout..."
k -n "$NAMESPACE" rollout status "deployment/$DEPLOYMENT" --timeout=180s
echo "部署完成: $IMAGE"
echo "管理员用户名: $admin_user（已有数据库不会因本次部署改变密码）"
