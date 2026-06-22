# FileServer

ファイルサーバーの起動・停止をGUIで管理するアプリです。

## 必要なもの

- Python 3.8 以上
- Docker
- Tailscale

## セットアップ（初回のみ）

```bash
git clone <リポジトリURL>
cd saisei_file_server
./setup.sh
```

これだけで完了です。アプリランチャーに **FileServer** が追加されます。

## 使い方

アプリランチャーから **FileServer** を起動するか、直接実行：

```bash
python3 fileserver_gui.py
```

| ボタン | 動作 |
|--------|------|
| ▶ 起動 | サーバーをDockerで起動し、Tailscale Funnelを有効化 |
| ■ 停止 | サーバーを停止し、Funnelを無効化 |
| 🌐 ブラウザで開く | TailscaleのHTTPS URLでブラウザを開く |

ステータスインジケーターが **緑（稼働中）** / **グレー（停止中）** でサーバーの状態を5秒ごとに自動確認します。

## トラブルシューティング

**アイコンが表示されない**
```bash
./setup.sh  # 再実行
```

**「モジュールが見つからない」エラー**
```bash
pip install ttkbootstrap pillow --break-system-packages
```

**起動しても繋がらない**

Tailscaleにログインしているか確認：
```bash
tailscale status
```
