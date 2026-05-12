#!/bin/bash
# =============================================
#  自動起動セットアップ（初回1回だけ実行）
#  Tailscaleのインストール・設定も含む
# =============================================
set -e
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PORT=4450

echo ""
echo "  ╔══════════════════════════════════════════╗"
echo "  ║   saisei_file_server  初回セットアップ   ║"
echo "  ╚══════════════════════════════════════════╝"
echo ""

# ── [1/5] Tailscale インストール ──────────────────
echo "  [1/5] Tailscale をインストールします..."
if command -v tailscale > /dev/null 2>&1; then
  echo "  [1/5] ✅ Tailscale はすでにインストール済みです"
else
  curl -fsSL https://tailscale.com/install.sh | sh
  echo "  [1/5] ✅ Tailscale インストール完了"
fi

# ── [2/5] Tailscale 起動・ログイン ────────────────
echo ""
echo "  [2/5] Tailscale にログインします..."
if tailscale status > /dev/null 2>&1; then
  echo "  [2/5] ✅ すでにログイン済みです"
else
  echo "  ──────────────────────────────────────────"
  echo "  ブラウザでURLを開いてログインしてください"
  echo "  ──────────────────────────────────────────"
  sudo tailscale up
  echo "  [2/5] ✅ ログイン完了"
fi

# ── [3/5] Tailscale operator 設定 ─────────────────
echo ""
echo "  [3/5] Tailscale operator を設定します..."
sudo tailscale set --operator=$USER
echo "  [3/5] ✅ operator 設定完了（sudoなしでfunnelを実行できます）"

# ── [4/5] Tailscale Funnel 有効化 ─────────────────
echo ""
echo "  [4/5] Tailscale Funnel を有効化します..."
echo "  ⚠️  Funnel はTailscale管理画面でACL設定が必要な場合があります"
echo "      https://login.tailscale.com/admin/acls"
tailscale funnel --bg $PORT
FUNNEL_URL=$(tailscale funnel status 2>/dev/null | grep "https://" | awk '{print $1}' | head -1)
echo "  [4/5] ✅ Funnel 有効化完了"
if [ -n "$FUNNEL_URL" ]; then
  echo "         外部URL: $FUNNEL_URL"
fi

# ── [5/5] systemd 自動起動設定 ────────────────────
echo ""
echo "  [5/5] 自動起動（systemd）を設定します..."

# autostart.sh を生成
cat > "$SCRIPT_DIR/autostart.sh" << AUTO
#!/bin/bash
set -e
cd "\$(dirname "\$0")"

# HOST_UPLOAD_DIR を自動設定
export HOST_UPLOAD_DIR="\$(pwd)/server/uploads"
mkdir -p "\$HOST_UPLOAD_DIR"

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
tailscale funnel --bg $PORT
AUTO
chmod +x "$SCRIPT_DIR/autostart.sh"

# systemd サービスファイルを作成
sudo tee /etc/systemd/system/saisei-server.service > /dev/null << SERVICE
[Unit]
Description=Saisei File Server
After=network-online.target docker.service tailscaled.service
Wants=network-online.target
Requires=docker.service tailscaled.service

[Service]
Type=oneshot
RemainAfterExit=yes
User=$USER
WorkingDirectory=$SCRIPT_DIR
ExecStart=/bin/bash $SCRIPT_DIR/autostart.sh
ExecStop=/usr/bin/docker compose -f $SCRIPT_DIR/docker-compose.yml down
StandardOutput=journal
StandardError=journal

[Install]
WantedBy=multi-user.target
SERVICE

sudo systemctl daemon-reload
sudo systemctl enable saisei-server.service
echo "  [5/5] ✅ 自動起動設定完了"

# ── 完了メッセージ ────────────────────────────────
echo ""
echo "  ┌──────────────────────────────────────────────────────┐"
echo "  │  ✅ セットアップ完了！                               │"
echo "  │                                                      │"
echo "  │  🖥  ローカル:   http://localhost:$PORT               │"
if [ -n "$FUNNEL_URL" ]; then
echo "  │  🌐 外部URL:    $FUNNEL_URL"
else
echo "  │  🌐 外部URL:    tailscale funnel status で確認       │"
fi
echo "  │                                                      │"
echo "  │  PC起動時に自動でサーバーと外部公開が始まります      │"
echo "  │                                                      │"
echo "  │  手動操作:                                           │"
echo "  │    起動: sudo systemctl start saisei-server          │"
echo "  │    停止: sudo systemctl stop saisei-server           │"
echo "  │    状態: sudo systemctl status saisei-server         │"
echo "  │    ログ: journalctl -u saisei-server -f              │"
echo "  └──────────────────────────────────────────────────────┘"
echo ""
