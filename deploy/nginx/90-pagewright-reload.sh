#!/bin/sh
# Pagewright はサイトを書き出すたびに _redirects.nginx（リダイレクトの設定）を
# 作り直す。nginx は設定を起動時にしか読まないので、変わったら読み込み直す。
# nginx 公式イメージの起動処理（/docker-entrypoint.d/）から呼ばれ、裏で動き続ける。
# 2つ目以降のサイト（templates/ に足した server ブロック）のものも見張る。
#
# 書き出されたリダイレクトの設定に誤りがあると、nginx は起動できず公開サイトも
# 管理画面も止まる。そこで起動時にも nginx -t で確かめ、だめなら include を外して
# リダイレクトなしで起動する（ファイル自体は /data に残るので調べられる）。
set -eu

files="/data/site/*/_redirects.nginx"
confd=/etc/nginx/conf.d
# conf.d のもとの設定（リダイレクトの include つき）
full=/tmp/pagewright-conf.d

rm -rf "$full"
mkdir -p "$full"
cp "$confd"/*.conf "$full"/

checksum() {
  # shellcheck disable=SC2086 # the glob must expand
  cat $files 2>/dev/null | md5sum
}

restore() {
  cp "$full"/*.conf "$confd"/
}

disable_redirects() {
  for f in "$full"/*.conf; do
    sed 's|^\([[:space:]]*include /data/site/.*/_redirects[^;]*;\)|# disabled by pagewright: \1|' "$f" >"$confd/${f##*/}"
  done
}

warn() {
  echo "pagewright: !!!!!!!! $* !!!!!!!!" >&2
}

redirects_disabled=0
if ! nginx -t -q 2>/dev/null; then
  disable_redirects
  if nginx -t -q 2>/dev/null; then
    redirects_disabled=1
    restore
    warn "リダイレクトの設定（$files）に誤りがあります"
    nginx -t 2>&1 | sed 's/^/pagewright:   /' >&2 || true
    disable_redirects
    warn "リダイレクトなしで起動します。サイトを書き出し直すと自動で読み込み直します"
  else
    # The redirects are not the cause; let nginx report the real error.
    restore
  fi
fi

(
  last=$(checksum)
  while sleep "${PAGEWRIGHT_RELOAD_INTERVAL:-10}"; do
    now=$(checksum)
    [ "$now" = "$last" ] && continue
    last=$now
    restore
    if nginx -t -q; then
      nginx -s reload
      redirects_disabled=0
      echo "pagewright: リダイレクトの設定が変わったので nginx を読み込み直しました" >&2
    else
      [ "$redirects_disabled" = 1 ] && disable_redirects
      echo "pagewright: リダイレクトの設定に誤りがあるため読み込み直していません" >&2
    fi
  done
) &
