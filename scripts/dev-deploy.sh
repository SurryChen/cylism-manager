#!/bin/bash
set -e

SSH_HOST="8.148.243.141"
SSH_USER="chenyilong"
SSH_KEY="$HOME/.ssh/id_ed25519_claw"
PROJECT_DIR="$HOME/dev/cylism-manager"
POD=$(ssh -i "$SSH_KEY" -o StrictHostKeyChecking=no -o ConnectTimeout=10 \
  "$SSH_USER@$SSH_HOST" "sudo /usr/local/bin/k3s kubectl get pod -l app=cylism-manager -o jsonpath='{.items[0].metadata.name}' 2>/dev/null")

if [ -z "$POD" ]; then
  echo "❌ No running pod found"
  exit 1
fi
echo "📎 Target pod: $POD"

# 编译后端
echo ""
echo "[1/3] Building Go binary..."
cd "$PROJECT_DIR"
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o /tmp/cylism-manager ./cmd/platform/ 2>&1
echo "✅ Go build done"

# 拷贝到 Pod
echo ""
echo "[2/3] Copying to pod..."
gzip -c /tmp/cylism-manager | ssh -i "$SSH_KEY" -o StrictHostKeyChecking=no -o ConnectTimeout=10 \
  "$SSH_USER@$SSH_HOST" "gunzip -c | sudo /usr/local/bin/k3s kubectl cp - default/$POD:/app/cylism-manager" 2>&1
echo "✅ Binary copied"

# 重启 Pod
echo ""
echo "[3/3] Restarting pod..."
ssh -i "$SSH_KEY" -o StrictHostKeyChecking=no -o ConnectTimeout=10 \
  "$SSH_USER@$SSH_HOST" "sudo /usr/local/bin/k3s kubectl delete pod $POD" 2>&1

echo ""
echo "⏳ Waiting for new pod..."
sleep 5
NEW_POD=$(ssh -i "$SSH_KEY" -o StrictHostKeyChecking=no -o ConnectTimeout=10 \
  "$SSH_USER@$SSH_HOST" "sudo /usr/local/bin/k3s kubectl wait --for=condition=Ready pod -l app=cylism-manager --timeout=180s -o jsonpath='{.items[0].metadata.name}' 2>/dev/null")
echo "✅ Pod ready: $NEW_POD"
