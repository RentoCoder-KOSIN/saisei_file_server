#!/bin/bash
set -e
cd "$(dirname "$0")"
BASE_DIR="$(pwd)"

echo "=== FileServer セットアップ ==="

# 1. 実行権限
echo "[1/4] autostart.sh に実行権限を付与..."
chmod +x "$BASE_DIR/autostart.sh"

# 2. Python依存ライブラリ
echo "[2/4] Python ライブラリをインストール..."
pip install ttkbootstrap pillow --break-system-packages -q

# 3. .desktop ファイルを生成・登録
echo "[3/4] デスクトップランチャーを登録..."
mkdir -p "$HOME/.local/share/applications"
cat > "$HOME/.local/share/applications/file-server.desktop" << EOF
[Desktop Entry]
Version=1.0
Type=Application
Name=FileServer
Comment=FileServer コントロールパネル
Exec=python3 $BASE_DIR/fileserver_gui.py
Icon=$BASE_DIR/icons/icon.png
Terminal=false
Categories=Utility;
EOF

update-desktop-database "$HOME/.local/share/applications/" 2>/dev/null || true

# 4. 完了
echo "[4/4] セットアップ完了！"
echo ""
echo "アプリランチャーから 'FileServer' を探して起動してください。"
echo "または: python3 $BASE_DIR/fileserver_gui.py"
