#!/bin/bash
set -e

IMAGE="crpi-c5u9bb8i5qxw1m72.cn-guangzhou.personal.cr.aliyuncs.com/surrychen/cylism-manager:latest"
SSH_HOST="8.148.243.141"
SSH_USER="chenyilong"
SSH_KEY="$HOME/.ssh/id_ed25519_claw"
PROJECT_DIR="$HOME/dev/cylism-manager"
YAML_SRC="$PROJECT_DIR/k8s/platform-deployment.yaml"
YAML_DST="/root/platform-deployment.yaml"

echo "[1/3] Building Docker image..."
cd "$PROJECT_DIR"
sudo docker build --build-arg GOPROXY=https://goproxy.cn,direct \
  -t cylism-manager:latest \
  -t "$IMAGE" \
  . 2>&1
if [ $? -ne 0 ]; then
  echo "❌ Build failed"
  exit 1
fi
echo "✅ Build done"

echo ""
echo "[2/3] Pushing to ACR..."
sudo docker push "$IMAGE" 2>&1
if [ $? -ne 0 ]; then
  echo "❌ Push failed"
  exit 1
fi
echo "✅ Push done"

echo ""
echo "[3/3] Deploying to k3s..."

# 创建 SSH key secret（本机生成 YAML 传远端 apply）
echo "📎 Creating SSH key secret..."
SSH_KEY_B64=$(base64 -w0 "$HOME/.ssh/id_ed25519_claw")
SECRET_YAML="/tmp/cylism-ssh-secret.yaml"
cat > "$SECRET_YAML" <<EOF
apiVersion: v1
kind: Secret
metadata:
  name: cylism-ssh-key
type: Opaque
data:
  id_ed25519: $SSH_KEY_B64
EOF
scp -i "$SSH_KEY" -o StrictHostKeyChecking=no -o ConnectTimeout=10 \
  "$SECRET_YAML" "$SSH_USER@$SSH_HOST:/tmp/cylism-ssh-secret.yaml"
ssh -i "$SSH_KEY" -o StrictHostKeyChecking=no -o ConnectTimeout=10 \
  "$SSH_USER@$SSH_HOST" "sudo /usr/local/bin/k3s kubectl apply -f /tmp/cylism-ssh-secret.yaml && rm -f /tmp/cylism-ssh-secret.yaml" 2>&1
rm -f "$SECRET_YAML"
echo "✅ SSH secret created/updated"

# 上传 yaml 到服务器用户目录，再 sudo 移到 /root
TMP_DST="/home/$SSH_USER/platform-deployment.yaml"
scp -i "$SSH_KEY" -o StrictHostKeyChecking=no -o ConnectTimeout=10 \
  "$YAML_SRC" "$SSH_USER@$SSH_HOST:$TMP_DST"
ssh -i "$SSH_KEY" -o StrictHostKeyChecking=no -o ConnectTimeout=10 \
  "$SSH_USER@$SSH_HOST" "sudo cp $TMP_DST $YAML_DST && echo '✅ YAML uploaded to $YAML_DST'"

# 判断原 pod 是否存在
EXISTING_POD=$(ssh -i "$SSH_KEY" -o StrictHostKeyChecking=no -o ConnectTimeout=10 \
  "$SSH_USER@$SSH_HOST" "sudo /usr/local/bin/k3s kubectl get pod -l app=cylism-manager -o name 2>/dev/null" || true)

if [ -n "$EXISTING_POD" ]; then
  echo "🔁 Existing pod found: $EXISTING_POD, rolling update..."
  # apply 新配置
  ssh -i "$SSH_KEY" -o StrictHostKeyChecking=no -o ConnectTimeout=10 \
    "$SSH_USER@$SSH_HOST" "sudo /usr/local/bin/k3s kubectl apply -f $YAML_DST 2>&1"
  # 删除旧 pod 触发重建
  ssh -i "$SSH_KEY" -o StrictHostKeyChecking=no -o ConnectTimeout=10 \
    "$SSH_USER@$SSH_HOST" "sudo /usr/local/bin/k3s kubectl delete pod -l app=cylism-manager 2>&1"
else
  echo "🆕 No existing pod, fresh apply..."
  ssh -i "$SSH_KEY" -o StrictHostKeyChecking=no -o ConnectTimeout=10 \
    "$SSH_USER@$SSH_HOST" "sudo /usr/local/bin/k3s kubectl apply -f $YAML_DST 2>&1"
fi

echo "⏳ Waiting for pod to be ready..."
sleep 10

if ! ssh -i "$SSH_KEY" -o StrictHostKeyChecking=no -o ConnectTimeout=10 \
  "$SSH_USER@$SSH_HOST" "sudo /usr/local/bin/k3s kubectl wait --for=condition=Ready pod -l app=cylism-manager --timeout=180s 2>&1"; then
  echo "❌ Pod not ready within 180s"
  exit 1
fi

echo ""
echo "✅ Deploy complete"
echo "   Pod: $(ssh -i "$SSH_KEY" -o StrictHostKeyChecking=no "$SSH_USER@$SSH_HOST" "sudo /usr/local/bin/k3s kubectl get pod -l app=cylism-manager -o name 2>/dev/null")"
