#!/bin/bash
# =============================================
#  自動起動セットアップ（初回1回だけ実行）
# =============================================
set -e
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"

echo ""
echo "  自動起動を設定します..."

# systemd サービスファイルを作成
sudo tee /etc/systemd/system/saisei-server.service > /dev/null << UNIT
[Unit]
Description=Saisei File Server
After=network-online.target docker.service
Wants=network-online.target
Requires=docker.service

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
UNIT

# サービス有効化
sudo systemctl daemon-reload
sudo systemctl enable saisei-server.service
sudo systemctl start saisei-server.service

echo ""
echo "  ✅ 自動起動の設定が完了しました！"
echo "  PC起動時に自動でサーバーとTailscale Funnelが起動します。"
echo ""
echo "  外部公開URL: https://penguin.tail2ef6f4.ts.net"
echo ""
echo "  手動操作:"
echo "    起動: sudo systemctl start saisei-server"
echo "    停止: sudo systemctl stop saisei-server"
echo "    状態: sudo systemctl status saisei-server"
echo ""
