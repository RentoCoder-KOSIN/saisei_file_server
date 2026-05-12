#!/bin/bash
set -e
cd "$(dirname "$0")"

# HOST_UPLOAD_DIR を自動設定
export HOST_UPLOAD_DIR="$(pwd)/server/uploads"
mkdir -p "$HOST_UPLOAD_DIR"

# pylab-python イメージのビルド（なければ）
if ! docker image inspect pylab-python > /dev/null 2>&1; then
  echo "pylab-python をビルドします..."
  cp server/pyrunner.py docker-python/pyrunner.py
  docker build -f docker-python/Dockerfile -t pylab-python docker-python/
  rm -f docker-python/pyrunner.py
fi

# サーバー起動
docker compose up --build -d

# Tailscale Funnel 起動（バックグラウンド）
tailscale funnel --bg 4450
