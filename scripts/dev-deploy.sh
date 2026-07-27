#!/bin/bash
set -e

SSH_HOST="8.148.243.141"
SSH_USER="chenyilong"
SSH_KEY="$HOME/.ssh/id_ed25519_claw"
PROJECT_DIR="$HOME/dev/cylism-manager"
SSH_CMD="ssh -i $SSH_KEY -o StrictHostKeyChecking=no -o ConnectTimeout=10 $SSH_USER@$SSH_HOST"

# 编译后端
echo ""
echo "[1/5] Building Go binary..."
cd "$PROJECT_DIR"
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o /tmp/cylism-manager ./cmd/platform/ 2>&1
echo "✅ Go build done"

# 构建前端
echo ""
echo "[2/5] Building frontend..."
cd "$PROJECT_DIR/web"
npm run build 2>&1
echo "✅ Frontend build done"

# 先 delete 旧 pod，等新 pod 从镜像就绪
echo ""
echo "[3/5] Restarting pod (from image)..."
OLD_POD=$($SSH_CMD "sudo /usr/local/bin/k3s kubectl get pod -l app=cylism-manager -o jsonpath='{.items[0].metadata.name}' 2>/dev/null")
if [ -n "$OLD_POD" ]; then
  $SSH_CMD "sudo /usr/local/bin/k3s kubectl delete pod $OLD_POD" 2>&1
  echo "✅ Old pod deleted: $OLD_POD"
fi
sleep 5
POD=$($SSH_CMD "sudo /usr/local/bin/k3s kubectl wait --for=condition=Ready pod -l app=cylism-manager --timeout=180s -o jsonpath='{.items[0].metadata.name}' 2>/dev/null")
echo "📎 Target pod: $POD"

KUBE_EXEC="sudo /usr/local/bin/k3s kubectl exec -i $POD -- sh -c"

# 拷贝后端
echo ""
echo "[4/5] Copying binary to pod..."
gzip -c /tmp/cylism-manager | $SSH_CMD "gunzip -c | $KUBE_EXEC 'cat > /app/cylism-manager && chmod +x /app/cylism-manager'" 2>&1
echo "✅ Binary copied"

# 拷贝前端
echo "[5/5] Copying frontend to pod..."
cd "$PROJECT_DIR/web/dist"
tar czf - . | $SSH_CMD "gunzip -c | $KUBE_EXEC 'cat > /tmp/frontend.tar && mkdir -p /app/web && rm -rf /app/web/* && tar xf /tmp/frontend.tar -C /app/web && rm /tmp/frontend.tar'" 2>&1
echo "✅ Frontend copied"

# 杀掉容器里的主进程(pid 1)，让 k8s 自动重启容器（容器重启不重建文件系统）
echo ""
echo "🔄 Killing main process to restart with new binary..."
$SSH_CMD "sudo /usr/local/bin/k3s kubectl exec $POD -- sh -c 'kill 1'" 2>&1 || true

echo "⏳ Waiting for pod to restart..."
sleep 5
$SSH_CMD "sudo /usr/local/bin/k3s kubectl wait --for=condition=Ready pod -l app=cylism-manager --timeout=60s > /dev/null 2>&1"
NEW_POD=$($SSH_CMD "sudo /usr/local/bin/k3s kubectl get pod -l app=cylism-manager -o jsonpath='{.items[0].metadata.name}' 2>/dev/null")
echo "✅ Pod ready: $NEW_POD"
