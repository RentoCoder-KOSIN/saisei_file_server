/server go mod init fileserver

ver.1.1.0

機能
1.ログイン(DB認証)
2.ユーザ追加(先生のみ)
3.パスワード変更(全員)
4.フォルダアップロード -> zip化
5.アップロード者・日時の記録
6.ファイル一覧表示
7.ダウンロード
8.削除(先生のみ)
9.実行(先生のみ)
10.課題作成・一覧（先生のみ作成、全員閲覧）  New
11.課題への提出紐づけ（期限付き）            New
12.遅延提出通知（先生のみ）                  New
13.提出状況モーダル（提出済・遅延・未提出）   New

構成
.
├── docker-compose.yml
├── docker-python/
│   └── Dockerfile          # Python実行環境（pygame等）
├── server/                 # Go ファイルサーバー (port 4450)
│   ├── Dockerfile
│   ├── main.go
│   ├── go.mod / go.sum
│   └── static/
│       ├── index.html
│       ├── app.js
│       └── style.css
└── task-server/            # Python (FastAPI) 課題管理サーバー (port 8000)
    ├── Dockerfile
    ├── main.py
    └── requirements.txt

起動方法
# 1. Python実行イメージをビルド（初回のみ）
docker build -t pylab-python ./docker-python

# 2. 全サービス起動
docker compose up --build
docker compose up

# アクセス
# http://localhost:4450  →  メインUI
# http://localhost:8000/docs  →  FastAPI自動ドキュメント

デフォルトアカウント
username: admin
password: admin123
role: teacher
