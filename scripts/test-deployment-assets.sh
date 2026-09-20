#!/usr/bin/env bash
set -Eeuo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

for asset in Dockerfile charts/cylism-manager k8s/platform-deployment.yaml; do
  if rg -q -i 'tailscale|/run/tailscale|hostPath' "$asset"; then
    echo "部署资产不得依赖宿主机 Tailscale 或 hostPath: $asset" >&2
    exit 1
  fi
done

rg -q 'kind: PersistentVolumeClaim' charts/cylism-manager/templates/pvc.yaml
rg -q 'persistentVolumeClaim:' charts/cylism-manager/templates/deployment.yaml
rg -q 'kind: PersistentVolumeClaim' k8s/platform-deployment.yaml
rg -q 'persistentVolumeClaim:' k8s/platform-deployment.yaml
rg -q 'strategy:' charts/cylism-manager/templates/deployment.yaml
rg -q 'type: Recreate' k8s/platform-deployment.yaml
if rg -q 'kubernetes.io/hostname' k8s/platform-deployment.yaml; then
  echo "静态部署清单不得固定调度到单个节点。" >&2
  exit 1
fi

rendered_file="$(mktemp)"
trap 'rm -f "$rendered_file"' EXIT
helm template cylism-manager charts/cylism-manager --namespace cylism-system >"$rendered_file"
rg -q 'kind: PersistentVolumeClaim' "$rendered_file"
rg -q 'persistentVolumeClaim:' "$rendered_file"
rg -q 'type: "Recreate"' "$rendered_file"
if rg -q -i 'tailscale|/run/tailscale|hostPath' "$rendered_file"; then
  echo "渲染后的 Helm Chart 包含已移除的宿主机依赖。" >&2
  exit 1
fi

echo "部署资产检查通过。"
