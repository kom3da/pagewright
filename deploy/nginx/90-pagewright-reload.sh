#!/bin/sh
# Pagewright はサイトを書き出すたびに _redirects.nginx（リダイレクトの設定）を
# 作り直す。nginx は設定を起動時にしか読まないので、変わったら読み込み直す。
# nginx 公式イメージの起動処理（/docker-entrypoint.d/）から呼ばれ、裏で動き続ける。
#
# 書き出されたリダイレクトの設定に誤りがあると、nginx は起動できず公開サイトも
# 管理画面も止まる。そこで起動時にも nginx -t で確かめ、だめなら include を外して
# リダイレクトなしで起動する（ファイル自体は /data に残るので調べられる）。
set -eu

files="/data/site/${PAGEWRIGHT_SITE_ID:-unset}/_redirects*.nginx"
conf=/etc/nginx/conf.d/default.conf
full=/tmp/pagewright-default.conf.with-redirects

cp "$conf" "$full"

checksum() {
  # shellcheck disable=SC2086 # the glob must expand
  cat $files 2>/dev/null | md5sum
}

disable_redirects() {
  sed 's|^\([[:space:]]*include /data/site/.*/_redirects\*\.nginx;\)|# disabled by pagewright: \1|' "$full" >"$conf"
}

warn() {
  echo "pagewright: !!!!!!!! $* !!!!!!!!" >&2
}

redirects_disabled=0
if ! nginx -t -q 2>/dev/null; then
  disable_redirects
  if nginx -t -q 2>/dev/null; then
    redirects_disabled=1
    cp "$full" "$conf"
    warn "リダイレクトの設定（$files）に誤りがあります"
    nginx -t 2>&1 | sed 's/^/pagewright:   /' >&2 || true
    disable_redirects
    warn "リダイレクトなしで起動します。サイトを書き出し直すと自動で読み込み直します"
  else
    # The redirects are not the cause; let nginx report the real error.
    cp "$full" "$conf"
  fi
fi

(
  last=$(checksum)
  while sleep "${PAGEWRIGHT_RELOAD_INTERVAL:-10}"; do
    now=$(checksum)
    [ "$now" = "$last" ] && continue
    last=$now
    cp "$full" "$conf"
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
