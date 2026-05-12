#!/bin/bash
# =============================================
#  saisei_file_server  かんたん起動スクリプト
#  Linux / macOS 対応
# =============================================
set -e
cd "$(dirname "$0")"

EXTERNAL=0
REBUILD=0
for arg in "$@"; do
  case "$arg" in
    --external|-e) EXTERNAL=1 ;;
    --rebuild|-r)  REBUILD=1  ;;
  esac
done

echo ""
echo "  ╔══════════════════════════════════════╗"
echo "  ║     saisei_file_server  起動中...    ║"
echo "  ╚══════════════════════════════════════╝"
echo ""

# [1/3] pylab-python イメージのビルド
if ! docker image inspect pylab-python > /dev/null 2>&1 || [ "$REBUILD" = "1" ]; then
  echo "  [1/3] pylab-python イメージをビルドします..."
  cp server/pyrunner.py docker-python/pyrunner.py
  docker build -f docker-python/Dockerfile -t pylab-python docker-python/ 2>&1 | sed 's/^/    /'
  rm -f docker-python/pyrunner.py
  echo "  [1/3] ✅ ビルド完了"
else
  echo "  [1/3] ✅ pylab-python イメージは既に存在します（再ビルドするには --rebuild を指定）"
fi

# [2/3] HOST_UPLOAD_DIR を自動設定
export HOST_UPLOAD_DIR="$(pwd)/server/uploads"
mkdir -p "$HOST_UPLOAD_DIR"
echo "  [2/3] HOST_UPLOAD_DIR = $HOST_UPLOAD_DIR"

# [3/3] サーバー起動
echo "  [3/3] サーバーを起動します..."
if [ "$EXTERNAL" = "1" ]; then
  docker compose --profile tunnel up --build -d
else
  docker compose up --build -d
fi
echo "  [3/3] ✅ 起動完了"

echo ""
echo "  ┌──────────────────────────────────────────────────┐"
echo "  │  ✅ 起動しました                                 │"
echo "  │  🖥  ローカル: http://localhost:4450              │"
echo "  │  📋 ログ確認: docker compose logs -f             │"
echo "  │  🛑 停止:     docker compose down                │"
if [ "$EXTERNAL" = "1" ]; then
echo "  │  🌐 外部URL:  docker compose logs tunnel          │"
fi
echo "  └──────────────────────────────────────────────────┘"
echo ""
