#!/bin/sh
# DBを初めて作るときに1回だけ実行される（PostgreSQL公式イメージの仕組み）。
# アプリが接続に使うユーザー pagewright_app を、ログインできる形で先に作る。
# マイグレーション（00001）は pagewright_app が既にあれば作らないので、
# この設定がそのまま使われる。パスワードを後から変えるときは docs/install/settings.md を参照。
set -eu

if [ -z "${PAGEWRIGHT_APP_DB_PASSWORD:-}" ]; then
  echo "PAGEWRIGHT_APP_DB_PASSWORD が空です" >&2
  exit 1
fi

psql -v ON_ERROR_STOP=1 -v pw="$PAGEWRIGHT_APP_DB_PASSWORD" \
  --username "$POSTGRES_USER" --dbname "$POSTGRES_DB" <<'SQL'
CREATE ROLE pagewright_app LOGIN PASSWORD :'pw' NOSUPERUSER NOBYPASSRLS NOCREATEDB NOCREATEROLE;
SQL
