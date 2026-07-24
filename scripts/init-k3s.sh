#!/bin/bash
set -e

echo "=== Cylism Manager K3s 初始化 ==="

# 1. 安装 K3s（如果未装）
if ! command -v kubectl &>/dev/null; then
  echo "安装 K3s..."
  curl -sfL https://get.k3s.io | sh -
  sleep 10
  mkdir -p ~/.kube
  sudo cp /etc/rancher/k3s/k3s.yaml ~/.kube/config
  sudo chown $(id -u):$(id -g) ~/.kube/config
fi

echo "K3s 已就绪"
kubectl get nodes

# 2. 安装 cert-manager
if ! kubectl get crd certificates.cert-manager.io &>/dev/null; then
  echo "安装 cert-manager..."
  kubectl apply -f https://github.com/cert-manager/cert-manager/releases/latest/download/cert-manager.yaml
  sleep 15
  kubectl wait --for=condition=Available deployment/cert-manager -n cert-manager --timeout=120s
fi

echo "cert-manager 已就绪"

# 3. 部署 Cylism Manager
echo "部署 Cylism Manager..."
kubectl apply -f k8s/platform-deployment.yaml

echo ""
echo "=== 初始化完成 ==="
echo "K3s token (用于添加节点):"
sudo cat /var/lib/rancher/k3s/server/node-token
echo ""
echo "Platform 启动中，可通过以下命令查看:"
echo "  kubectl get pods -l app=cylism-manager"
echo "  kubectl port-forward svc/cylism-manager 8080:8080"
