ver.1.5.0

機能
1.ログイン(DB認証)
2.ユーザ追加(先生のみ)
3.パスワード変更(全員)
4.フォルダアップロード -> zip化
5.アップロード者・日時の記録
6.ファイル一覧表示
7.ダウンロード
8.削除(先生のみ)
9.実行(先生のみ) ← project.toml 対応
10.課題作成・一覧（先生・admin作成、全員閲覧）
11.課題への提出紐づけ（期限付き・複数提出対応）
12.遅延提出通知（先生のみ）
13.提出状況モーダル
14.複数フォルダアップロード
15.ファイル検索
16.外部アクセス（Cloudflare Tunnel / ドメイン不要）
17.OS起動時の自動起動
18.リロード時のログイン状態保持
19.古い提出ファイルの自動削除（ディスク節約）

構成
.
├── start.sh              # 手動起動（サーバー + 外部公開）
├── start.bat             # 手動起動（Windows用）
├── setup-autostart.sh    # 自動起動セットアップ（初回1回だけ実行）
├── docker-compose.yml
├── docker-python/
│   └── Dockerfile
├── server/
│   ├── main.go
│   ├── go.mod
│   ├── go.sum            # ← 依存ロック（初回起動時に自動生成）
│   ├── pyrunner.py       # project.toml ベース実行エンジン
│   └── static/
└── task-server/
    ├── main.py
    └── requirements.txt

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
起動時間について
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

  【初回起動】go.sum が無い場合、自動で生成します（数分かかります）。
  【2回目以降】go.sum とDockerキャッシュが効くため、大幅に高速化されます。

  go.sum を手動で生成する場合:
    docker run --rm -v "$(pwd)/server":/app -w /app \
      -e GONOSUMDB="*" golang:1.23-alpine sh -c "go mod tidy"

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
初回セットアップ（1回だけ実行）
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

  Linux/Mac:
    ./setup-autostart.sh

  これだけで:
  - PC起動時に自動でサーバーが立ち上がる
  - PC起動時に自動で外部公開される

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
手動起動
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

  Linux/Mac:  ./start.sh
  Windows:    start.bat をダブルクリック

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
アクセス
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

  ローカル: http://localhost:4450

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
外部公開URL（Cloudflare Tunnel / 固定URL）
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

  Cloudflareアカウントがあればドメイン不要で固定URLを取得できます。

  【初回セットアップ】
  1. https://one.dash.cloudflare.com/ にログイン
  2. Networks → Tunnels → Create a tunnel → Cloudflared を選択
  3. 名前をつける（例: saisei-server）
  4. トークン（eyJ...）をコピーして .env の CLOUDFLARE_TOKEN に貼り付け
  5. Public Hostname タブで以下を設定:
       Subdomain: 好きな名前（例: saisei）
       Domain:    cfargotunnel.com（無料・ドメイン不要）
       Service:   http://file-server:4450
  6. docker compose up -d で起動

  設定したURL（例: https://saisei.cfargotunnel.com）が固定で使えます。
  再起動してもURLは変わりません。

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
  状態: sudo systemctl status saisei-server
  ログ: docker compose logs -f

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
デフォルトアカウント
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

  username: admin
  password: admin123
  role: teacher
