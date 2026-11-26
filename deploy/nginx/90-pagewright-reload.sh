#!/bin/sh
# Pagewright はサイトを書き出すたびに _redirects.nginx（リダイレクトの設定）を
# 作り直す。nginx は設定を起動時にしか読まないので、変わったら読み込み直す。
# nginx 公式イメージの起動処理（/docker-entrypoint.d/）から呼ばれ、裏で動き続ける。
set -eu

files="/data/site/${PAGEWRIGHT_SITE_ID:-unset}/_redirects*.nginx"

checksum() {
  # shellcheck disable=SC2086 # the glob must expand
  cat $files 2>/dev/null | md5sum
}

(
  last=$(checksum)
  while sleep "${PAGEWRIGHT_RELOAD_INTERVAL:-10}"; do
    now=$(checksum)
    [ "$now" = "$last" ] && continue
    last=$now
    if nginx -t -q; then
      nginx -s reload
      echo "pagewright: リダイレクトの設定が変わったので nginx を読み込み直しました" >&2
    else
      echo "pagewright: リダイレクトの設定に誤りがあるため読み込み直していません" >&2
    fi
  done
) &
