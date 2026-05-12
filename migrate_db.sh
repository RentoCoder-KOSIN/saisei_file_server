#!/bin/bash
# =============================================
#  SQLite → JSON マイグレーションスクリプト
#  旧バージョン(users.db)から新バージョン(users.json)へ移行
# =============================================
set -e
cd "$(dirname "$0")"

DB_FILE="${1:-./server/users.db}"

if [ ! -f "$DB_FILE" ]; then
  echo "  ℹ️  $DB_FILE が見つかりません（新規インストールの場合は不要です）"
  exit 0
fi

if ! command -v sqlite3 &> /dev/null; then
  echo "  ⚠️  sqlite3 コマンドが見つかりません"
  echo "  手動で以下を実行してください:"
  echo "    sqlite3 $DB_FILE .dump"
  exit 1
fi

echo "  📦 SQLiteデータを移行します: $DB_FILE"
echo "  ⚠️  新バージョンではパスワードの再設定が必要です"
echo "  ⚠️  (ハッシュ形式が変わったため)"
echo ""

# usersテーブルのユーザー名とロールだけ取り出す
sqlite3 "$DB_FILE" "SELECT username, role FROM users;" | while IFS='|' read -r username role; do
  echo "    ユーザー: $username ($role) ← パスワードを 'changeme' にリセット"
done

echo ""
echo "  移行後、管理者が各ユーザーのパスワードを再設定してください。"
echo "  (設定 → パスワード変更)"
