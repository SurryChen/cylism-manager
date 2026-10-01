#!/usr/bin/env bash
if [ -z "${BASH_VERSION:-}" ]; then exec bash "$0" "$@"; fi
set -Eeuo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
MANIFEST="${MANIFEST:-$ROOT_DIR/k8s/platform-deployment.yaml}"
DEFAULT_IMAGE="${CYLISM_DEFAULT_IMAGE:-ghcr.io/surrychen/cylism-manager:latest}"
NAMESPACE="${NAMESPACE:-cylism-system}"
DEPLOYMENT="${DEPLOYMENT:-cylism-manager}"
CONTAINER="${CONTAINER:-platform}"
ACTION="install"
IMAGE="${CYLISM_IMAGE:-}"
NODE_NAME="${CYLISM_NODE_NAME:-}"
HTTPS_IP="${CYLISM_HTTPS_IP:-}"
IMAGE_PULL_SECRET="${CYLISM_IMAGE_PULL_SECRET:-}"
GHCR_USERNAME="${CYLISM_GHCR_USERNAME:-}"
GHCR_TOKEN="${CYLISM_GHCR_TOKEN:-}"
CONFIGURE_GHCR_PULL_SECRET="${CYLISM_CONFIGURE_GHCR_PULL_SECRET:-}"
VERIFY_IMAGE_PULL="${CYLISM_VERIFY_IMAGE_PULL:-}"
BACKUP_DIR="${CYLISM_BACKUP_DIR:-}"
SKIP_BACKUP="${CYLISM_SKIP_BACKUP:-}"
PURGE_DATA="${CYLISM_PURGE_DATA:-}"
ASSUME_YES="${CYLISM_ASSUME_YES:-}"

case "${1:-}" in
  install) ACTION="install"; shift ;;
  uninstall|remove) ACTION="uninstall"; shift ;;
esac

usage() {
  cat <<'EOF'
Usage: scripts/deploy-platform.sh [install|uninstall] [options]
  --image IMAGE              Image reference (default: ghcr.io/surrychen/cylism-manager:latest)
  --namespace NAME           Kubernetes namespace (default: cylism-system)
  --node NAME                Node selector for a fresh deployment
  --https-ip IP              Create a temporary self-signed HTTPS Ingress for this IP
  --image-pull-secret NAME   Existing imagePullSecret name
  --ghcr-username NAME       GitHub username for private GHCR image pulls
  --ghcr-token TOKEN         GitHub token for private GHCR image pulls
  --configure-ghcr-pull      Create/reuse an imagePullSecret for private GHCR
  --verify-image-pull        Verify Kubernetes can pull the target image first
  --backup-dir DIR           Save pre-deploy resource backups under DIR
  --skip-backup              Do not create pre-deploy resource backups
  --purge-data               With uninstall, also delete the PVC and cylism-secret
  --yes                      Skip uninstall confirmation
  --manifest PATH            Kubernetes manifest path (install only)
  -h, --help                 Show this help

Existing cylism-secret values are reused. Missing values are generated or
requested interactively; existing values are never rotated. If no image is
specified and an existing deployment uses another registry, the script switches
it to ghcr.io/surrychen/cylism-manager:latest.
EOF
}

die() { echo "错误: $*" >&2; exit 1; }
need_cmd() { command -v "$1" >/dev/null 2>&1 || die "缺少命令: $1"; }

while [ "$#" -gt 0 ]; do
  case "$1" in
    --image) IMAGE="${2:?--image 需要参数}"; shift 2 ;;
    --namespace) NAMESPACE="${2:?--namespace 需要参数}"; shift 2 ;;
    --node) NODE_NAME="${2:?--node 需要参数}"; shift 2 ;;
    --https-ip) HTTPS_IP="${2:?--https-ip 需要参数}"; shift 2 ;;
    --image-pull-secret) IMAGE_PULL_SECRET="${2:?--image-pull-secret 需要参数}"; shift 2 ;;
    --ghcr-username) GHCR_USERNAME="${2:?--ghcr-username 需要参数}"; shift 2 ;;
    --ghcr-token) GHCR_TOKEN="${2:?--ghcr-token 需要参数}"; shift 2 ;;
    --configure-ghcr-pull) CONFIGURE_GHCR_PULL_SECRET="true"; shift ;;
    --verify-image-pull) VERIFY_IMAGE_PULL="true"; shift ;;
    --backup-dir) BACKUP_DIR="${2:?--backup-dir 需要参数}"; shift 2 ;;
    --skip-backup) SKIP_BACKUP="true"; shift ;;
    --purge-data) PURGE_DATA="true"; shift ;;
    --yes) ASSUME_YES="true"; shift ;;
    --manifest) MANIFEST="${2:?--manifest 需要参数}"; shift 2 ;;
    -h|--help) usage; exit 0 ;;
    *) die "未知参数: $1（使用 --help 查看用法）" ;;
  esac
done

if [ "$ACTION" = "install" ] && [ -z "$HTTPS_IP" ]; then
  read -r -p "请输入 HTTPS 访问的服务器公网 IPv4（回车跳过）: " HTTPS_IP
fi

if [ "$ACTION" = "install" ]; then
  need_cmd base64
  need_cmd openssl
  need_cmd curl
  [ -f "$MANIFEST" ] || die "找不到 Kubernetes 清单: $MANIFEST"
fi

probe_kubectl() {
  "$@" get nodes >/dev/null 2>&1
}

select_kubectl() {
  if [ -n "${CYLISM_KUBECTL:-}" ]; then
    read -r -a KUBECTL <<< "$CYLISM_KUBECTL"
    probe_kubectl "${KUBECTL[@]}" || die "CYLISM_KUBECTL 无法访问 Kubernetes 集群: $CYLISM_KUBECTL"
    return
  fi

  if command -v kubectl >/dev/null 2>&1 && probe_kubectl kubectl; then
    KUBECTL=(kubectl)
    return
  fi
  if command -v k3s >/dev/null 2>&1 && probe_kubectl k3s kubectl; then
    KUBECTL=(k3s kubectl)
    return
  fi
  if [ -x /usr/local/bin/k3s ] && probe_kubectl /usr/local/bin/k3s kubectl; then
    KUBECTL=(/usr/local/bin/k3s kubectl)
    return
  fi
  if command -v sudo >/dev/null 2>&1 && command -v k3s >/dev/null 2>&1 && sudo -n k3s kubectl get nodes >/dev/null 2>&1; then
    KUBECTL=(sudo k3s kubectl)
    return
  fi
  if command -v sudo >/dev/null 2>&1 && [ -x /usr/local/bin/k3s ] && sudo -n /usr/local/bin/k3s kubectl get nodes >/dev/null 2>&1; then
    KUBECTL=(sudo /usr/local/bin/k3s kubectl)
    return
  fi

  if command -v sudo >/dev/null 2>&1 && command -v k3s >/dev/null 2>&1; then
    read -r -p "检测到 k3s，但当前用户直接访问失败，是否使用 sudo k3s kubectl？[Y/n] " answer
    if [[ ! "${answer:-Y}" =~ ^[Nn]$ ]]; then
      KUBECTL=(sudo k3s kubectl)
      probe_kubectl "${KUBECTL[@]}" || die "sudo k3s kubectl 无法访问 Kubernetes 集群"
      return
    fi
  fi
  if command -v sudo >/dev/null 2>&1 && [ -x /usr/local/bin/k3s ]; then
    read -r -p "检测到 /usr/local/bin/k3s，但当前用户直接访问失败，是否使用 sudo /usr/local/bin/k3s kubectl？[Y/n] " answer
    if [[ ! "${answer:-Y}" =~ ^[Nn]$ ]]; then
      KUBECTL=(sudo /usr/local/bin/k3s kubectl)
      probe_kubectl "${KUBECTL[@]}" || die "sudo /usr/local/bin/k3s kubectl 无法访问 Kubernetes 集群"
      return
    fi
  fi

  read -r -p "未检测到 K3s/kubectl，是否安装 K3s？[Y/n] " answer
  [[ "${answer:-Y}" =~ ^[Nn]$ ]] && die "请先安装 K3s 或 kubectl"
  curl -sfL https://get.k3s.io | sh -
  if [ -x /usr/local/bin/k3s ]; then
    KUBECTL=(sudo /usr/local/bin/k3s kubectl)
  else
    KUBECTL=(sudo k3s kubectl)
  fi
  sleep 5
  probe_kubectl "${KUBECTL[@]}" || die "K3s 安装后仍无法访问 Kubernetes 集群"
}

select_kubectl
k() { "${KUBECTL[@]}" "$@"; }
echo "使用 Kubernetes 命令: ${KUBECTL[*]}"
if [ "$ACTION" = "install" ]; then
  k get namespace "$NAMESPACE" >/dev/null 2>&1 || k create namespace "$NAMESPACE" >/dev/null
fi

configure_https_ingress() {
  [ -n "$HTTPS_IP" ] || return 0
  [[ "$HTTPS_IP" =~ ^([0-9]{1,3}\.){3}[0-9]{1,3}$ ]] || die "--https-ip 必须是 IPv4 地址"
  IFS='.' read -r -a ip_octets <<< "$HTTPS_IP"
  for octet in "${ip_octets[@]}"; do
    [ "$octet" -le 255 ] || die "--https-ip 不是有效的 IPv4 地址: $HTTPS_IP"
  done
  k -n kube-system get service traefik >/dev/null 2>&1 || die "未检测到 K3s Traefik，无法创建 HTTPS 入口"
  traefik_https_port="$(k -n kube-system get service traefik -o jsonpath='{.spec.ports[?(@.port==443)].port}' 2>/dev/null || true)"
  [ "$traefik_https_port" = "443" ] || die "Traefik Service 未暴露 443 端口"

  local tls_dir
  tls_dir="$(mktemp -d)"
  openssl req -x509 -nodes -newkey rsa:2048 -days 30 \
    -keyout "$tls_dir/tls.key" \
    -out "$tls_dir/tls.crt" \
    -subj "/CN=$HTTPS_IP" \
    -addext "subjectAltName = IP:$HTTPS_IP" >/dev/null 2>&1
  k -n "$NAMESPACE" create secret tls cylism-manager-tls \
    --cert="$tls_dir/tls.crt" --key="$tls_dir/tls.key" \
    --dry-run=client -o yaml | k apply -f - >/dev/null
  rm -rf "$tls_dir"

  cat <<EOF | k -n "$NAMESPACE" apply -f - >/dev/null
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: cylism-manager
spec:
  ingressClassName: traefik
  tls:
    - secretName: cylism-manager-tls
  rules:
    - http:
        paths:
          - path: /
            pathType: Prefix
            backend:
              service:
                name: cylism-manager
                port:
                  number: 8080
EOF
  echo "已配置临时 HTTPS 入口: https://$HTTPS_IP（自签证书有效期 30 天）"
}

uninstall_platform() {
  if ! k get namespace "$NAMESPACE" >/dev/null 2>&1; then
    echo "命名空间 $NAMESPACE 不存在，无需卸载。"
    return 0
  fi
  if [[ ! "$ASSUME_YES" =~ ^([Yy]|true|TRUE|1)$ ]]; then
    read -r -p "确认卸载 Cylism Manager（默认保留 PVC 和核心 Secret）？[y/N] " answer
    [[ "$answer" =~ ^([Yy]|yes|YES)$ ]] || die "已取消卸载。"
  fi

  echo "清理 $NAMESPACE 中的 Manager 资源..."
  k -n "$NAMESPACE" delete \
    deployment/"$DEPLOYMENT" \
    service/"$DEPLOYMENT" \
    ingress/"$DEPLOYMENT" \
    serviceaccount/"$DEPLOYMENT" \
    role/"$DEPLOYMENT" \
    rolebinding/"$DEPLOYMENT" \
    configmap/cylism-config \
    secret/cylism-manager-tls \
    secret/cylism-ssh-key \
    --ignore-not-found >/dev/null
  k delete \
    clusterrolebinding/cylism-manager \
    clusterrole/cylism-manager \
    clusterrolebinding/cylism-alidns-webhook-solver \
    clusterrole/cylism-alidns-webhook-solver \
    --ignore-not-found >/dev/null

  if [[ "$PURGE_DATA" =~ ^([Yy]|true|TRUE|1)$ ]]; then
    echo "删除 PVC 和核心 Secret..."
    k -n "$NAMESPACE" delete pvc/cylism-manager-data secret/cylism-secret --ignore-not-found >/dev/null
  else
    echo "已保留 PVC cylism-manager-data 和 Secret cylism-secret。"
  fi
  echo "卸载完成；命名空间 $NAMESPACE 未删除。"
}

if [ "$ACTION" = "uninstall" ]; then
  uninstall_platform
  exit 0
fi

backup_if_exists() {
  local resource="$1"
  local name="$2"
  local file="$3"
  if k -n "$NAMESPACE" get "$resource" "$name" >/dev/null 2>&1; then
    k -n "$NAMESPACE" get "$resource" "$name" -o yaml > "$BACKUP_DIR/$file"
  fi
}

backup_secret_keys_if_exists() {
  local name="$1"
  local file="$2"
  if k -n "$NAMESPACE" get secret "$name" >/dev/null 2>&1; then
    k -n "$NAMESPACE" get secret "$name" -o go-template='{{range $k,$v := .data}}{{printf "%s\n" $k}}{{end}}' > "$BACKUP_DIR/$file"
  fi
}

create_predeploy_backup() {
  if [[ "$SKIP_BACKUP" =~ ^([Yy]|true|TRUE|1)$ ]]; then
    echo "跳过部署前备份。"
    return
  fi
  if [ -z "$BACKUP_DIR" ]; then
    BACKUP_DIR="${HOME:-/tmp}/cylism-manager-backups/$(date +%Y%m%d-%H%M%S)"
  fi
  mkdir -p "$BACKUP_DIR"
  chmod 700 "$BACKUP_DIR"
  backup_if_exists deployment "$DEPLOYMENT" "deployment-$DEPLOYMENT.yaml"
  backup_if_exists service "$DEPLOYMENT" "service-$DEPLOYMENT.yaml"
  backup_if_exists serviceaccount "$DEPLOYMENT" "serviceaccount-$DEPLOYMENT.yaml"
  backup_if_exists role "$DEPLOYMENT" "role-$DEPLOYMENT.yaml"
  backup_if_exists rolebinding "$DEPLOYMENT" "rolebinding-$DEPLOYMENT.yaml"
  backup_if_exists configmap cylism-config "configmap-cylism-config.yaml"
  backup_secret_keys_if_exists cylism-secret "secret-cylism-secret-keys.txt"
  if [ -n "$IMAGE_PULL_SECRET" ]; then
    backup_secret_keys_if_exists "$IMAGE_PULL_SECRET" "secret-$IMAGE_PULL_SECRET-keys.txt"
  fi
  echo "部署前备份目录: $BACKUP_DIR"
  echo "Secret 仅备份 key 名称，不导出敏感值。"
}

create_predeploy_backup

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
is_cylism_ghcr_image() {
  case "$1" in
    ghcr.io/surrychen/cylism-manager:*|ghcr.io/surrychen/cylism-manager@*) return 0 ;;
    *) return 1 ;;
  esac
}
image_uses_ghcr() {
  case "$1" in
    ghcr.io/*) return 0 ;;
    *) return 1 ;;
  esac
}
verify_image_pull() {
  [[ "$VERIFY_IMAGE_PULL" =~ ^([Yy]|true|TRUE|1)$ ]] || return
  local check_pod="cylism-image-pull-check-$(date +%s)-$$"
  local overrides=''
  if [ -n "$IMAGE_PULL_SECRET" ]; then
    overrides="{\"spec\":{\"imagePullSecrets\":[{\"name\":\"$IMAGE_PULL_SECRET\"}]}}"
  fi

  echo "验证 Kubernetes 是否可以拉取镜像: $IMAGE"
  if [ -n "$overrides" ]; then
    k -n "$NAMESPACE" run "$check_pod" --image="$IMAGE" --restart=Never --overrides="$overrides" --command -- sh -c 'sleep 5' >/dev/null
  else
    k -n "$NAMESPACE" run "$check_pod" --image="$IMAGE" --restart=Never --command -- sh -c 'sleep 5' >/dev/null
  fi

  local i image_id waiting_reason waiting_message
  for i in $(seq 1 30); do
    image_id="$(k -n "$NAMESPACE" get pod "$check_pod" -o jsonpath='{.status.containerStatuses[0].imageID}' 2>/dev/null || true)"
    if [ -n "$image_id" ]; then
      k -n "$NAMESPACE" delete pod "$check_pod" --ignore-not-found --wait=false >/dev/null 2>&1 || true
      echo "镜像拉取验证通过。"
      return
    fi
    waiting_reason="$(k -n "$NAMESPACE" get pod "$check_pod" -o jsonpath='{.status.containerStatuses[0].state.waiting.reason}' 2>/dev/null || true)"
    waiting_message="$(k -n "$NAMESPACE" get pod "$check_pod" -o jsonpath='{.status.containerStatuses[0].state.waiting.message}' 2>/dev/null || true)"
    case "$waiting_reason" in
      ErrImagePull|ImagePullBackOff|InvalidImageName)
        k -n "$NAMESPACE" describe pod "$check_pod" >&2 || true
        k -n "$NAMESPACE" delete pod "$check_pod" --ignore-not-found --wait=false >/dev/null 2>&1 || true
        die "镜像拉取验证失败: ${waiting_message:-$waiting_reason}"
        ;;
    esac
    sleep 2
  done

  k -n "$NAMESPACE" describe pod "$check_pod" >&2 || true
  k -n "$NAMESPACE" delete pod "$check_pod" --ignore-not-found --wait=false >/dev/null 2>&1 || true
  die "镜像拉取验证超时"
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
if [ -z "$public_url" ]; then
  if [ -n "$HTTPS_IP" ]; then
    public_url="https://$HTTPS_IP"
    echo "使用 HTTPS 公开访问地址: $public_url"
  else
    read -r -p "公开访问地址（可留空）: " public_url
  fi
fi
admin_user="$(read_config admin-user)"; admin_user="${admin_user:-admin}"
access_ttl="$(read_config access-token-ttl)"; access_ttl="${access_ttl:-7200}"
refresh_ttl="$(read_config refresh-token-ttl)"; refresh_ttl="${refresh_ttl:-604800}"
retention_days="$(read_config operation-log-retention-days)"; retention_days="${retention_days:-30}"
k -n "$NAMESPACE" create configmap cylism-config \
  --from-literal="admin-user=$admin_user" \
  --from-literal="public-url=$public_url" \
  --from-literal="access-token-ttl=$access_ttl" \
  --from-literal="refresh-token-ttl=$refresh_ttl" \
  --from-literal="operation-log-retention-days=$retention_days" \
  --dry-run=client -o yaml | k apply -f - >/dev/null

current_image=""
if [ -z "$IMAGE" ]; then
  current_image="$(k -n "$NAMESPACE" get deployment "$DEPLOYMENT" -o "jsonpath={.spec.template.spec.containers[?(@.name=='$CONTAINER')].image}" 2>/dev/null || true)"
  if is_cylism_ghcr_image "$current_image"; then
    IMAGE="$current_image"
    echo "复用现有 GHCR 镜像: $IMAGE"
  elif [ -n "$current_image" ]; then
    IMAGE="$DEFAULT_IMAGE"
    echo "检测到现有镜像: $current_image"
    echo "将更新为默认 GHCR 镜像: $IMAGE"
  else
    IMAGE="$DEFAULT_IMAGE"
    echo "使用默认 GHCR 镜像: $IMAGE"
  fi
fi
[ -n "$IMAGE" ] || die "镜像地址不能为空"

if [ -z "$IMAGE_PULL_SECRET" ] && image_uses_ghcr "$IMAGE"; then
  existing_pull_secret="$(k -n "$NAMESPACE" get deployment "$DEPLOYMENT" -o jsonpath='{.spec.template.spec.imagePullSecrets[0].name}' 2>/dev/null || true)"
  IMAGE_PULL_SECRET="$existing_pull_secret"
  if [ -z "$IMAGE_PULL_SECRET" ]; then
    configure_answer="$CONFIGURE_GHCR_PULL_SECRET"
    if [ -z "$configure_answer" ]; then
      read -r -p "GHCR 镜像如果是私有，需要 imagePullSecret。是否现在配置？[y/N] " configure_answer
    fi
    if [[ "$configure_answer" =~ ^([Yy]|true|TRUE|1)$ ]]; then
      IMAGE_PULL_SECRET="${CYLISM_IMAGE_PULL_SECRET_NAME:-ghcr-pull-secret}"
      if k -n "$NAMESPACE" get secret "$IMAGE_PULL_SECRET" >/dev/null 2>&1; then
        echo "复用现有 imagePullSecret: $IMAGE_PULL_SECRET"
      else
        [ -n "$GHCR_USERNAME" ] || read -r -p "GitHub 用户名: " GHCR_USERNAME
        if [ -z "$GHCR_TOKEN" ]; then
          read -r -s -p "GitHub Token（需要 read:packages 权限，不会回显）: " GHCR_TOKEN; echo
        fi
        [ -n "$GHCR_USERNAME" ] || die "GitHub 用户名不能为空"
        [ -n "$GHCR_TOKEN" ] || die "GitHub Token 不能为空"
        k -n "$NAMESPACE" create secret docker-registry "$IMAGE_PULL_SECRET" \
          --docker-server=ghcr.io \
          --docker-username="$GHCR_USERNAME" \
          --docker-password="$GHCR_TOKEN" \
          --dry-run=client -o yaml | k apply -f - >/dev/null
        echo "已创建 imagePullSecret: $IMAGE_PULL_SECRET"
      fi
    else
      echo "未配置 imagePullSecret；GHCR 镜像需要设置为 Public，或后续 Pod 可能无法拉取镜像。"
    fi
  else
    echo "复用现有 imagePullSecret: $IMAGE_PULL_SECRET"
  fi
fi

verify_image_pull

echo "应用 Kubernetes 清单..."
k -n "$NAMESPACE" apply -f "$MANIFEST" >/dev/null
# The static manifest is namespace-agnostic, but ClusterRoleBinding subjects
# are cluster-scoped and must follow the selected deployment namespace.
k patch clusterrolebinding cylism-manager --type merge \
  -p "{\"subjects\":[{\"kind\":\"ServiceAccount\",\"name\":\"cylism-manager\",\"namespace\":\"$NAMESPACE\"}]}" >/dev/null
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
configure_https_ingress
echo "部署完成: $IMAGE"
echo "管理员用户名: $admin_user（已有数据库不会因本次部署改变密码）"
echo "命名空间: $NAMESPACE"
echo "Deployment: $DEPLOYMENT"
echo "容器: $CONTAINER"
echo "配置来源: cylism-secret / cylism-config"
echo "数据库路径: /data/cylism.db"
echo "imagePullSecret: ${IMAGE_PULL_SECRET:-未配置}"
echo "节点选择: ${NODE_NAME:-未设置}"
if [[ ! "$SKIP_BACKUP" =~ ^([Yy]|true|TRUE|1)$ ]]; then
  echo "部署前备份目录: $BACKUP_DIR"
fi
