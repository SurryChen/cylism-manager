#!/bin/bash
set -e
AUTH_KEY="${1:?Usage: $0 <tskey-auth-xxx>}"
HOSTNAME="${2:-$(hostname)}"

echo "[1/2] Installing Tailscale..."
curl -fsSL https://tailscale.com/install.sh | sh

echo "[2/2] Registering node..."
tailscale up --auth-key="$AUTH_KEY" --hostname="$HOSTNAME" --accept-routes

echo "Tailscale IP: $(tailscale ip -4)"
