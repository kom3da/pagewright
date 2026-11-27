# リリースの手順（メンテナー向け）

版の付け方と「互換性を壊す変更」の決まりは [docs/install/versioning.md](docs/install/versioning.md)。
タグ `v*` を push すると `.github/workflows/release.yaml` が動き、CI と同じ検査が通ったら GoReleaser がバイナリ・チェックサム・Docker イメージ（`ghcr.io/kom3da/pagewright`）を公開します。

## ふだん

- 利用者に関わる変更をしたら、同じ PR で [CHANGELOG.md](CHANGELOG.md) の `## [Unreleased]` に1行足す（`### 追加`・`### 変更`・`### 修正`・`### セキュリティ`）
- 互換性を壊す変更なら、何をすればよいかを `### 注意` に書く
- リリース済みのマイグレーションは書き換えない。直すときは新しいマイグレーションを足す（CI の「Released migrations are unchanged」が止める）

## リリースする

1. 版を決める（0.x のあいだは、修正だけならパッチ、それ以外はマイナー）
2. main で、次の2つを1つのコミットにする（`Release 0.2.0` など）
   - `CHANGELOG.md`：`## [Unreleased]` の中身を `## [0.2.0] - YYYY-MM-DD` に移し、空の `## [Unreleased]` を残す。末尾のリンクを直す

     ```
     [Unreleased]: https://github.com/kom3da/pagewright/compare/v0.2.0...HEAD
     [0.2.0]: https://github.com/kom3da/pagewright/compare/v0.1.0...v0.2.0
     ```

     最初の版は `[0.1.0]: https://github.com/kom3da/pagewright/releases/tag/v0.1.0`

   - `deploy/.env.example`：`PAGEWRIGHT_IMAGE=ghcr.io/kom3da/pagewright:0.2.0`
3. 手元で確かめる

   ```sh
   .github/scripts/release-notes.sh 0.2.0            # リリースノートになる部分。準備が足りなければ失敗する
   mise exec -- goreleaser release --snapshot --clean
   docker run --rm ghcr.io/kom3da/pagewright:<表示された版>-amd64 version
   ```

4. タグを付けて push する

   ```sh
   git tag -a v0.2.0 -m v0.2.0
   git push origin v0.2.0
   ```

5. Actions の Release が通ったら、リリースのページを確かめる
6. **最初のリリースのときだけ：** GHCR のパッケージ `pagewright` の公開範囲を Public にし、リポジトリとつなぐ（Package settings → Change visibility、Manage Actions access）。
   GHCR のパッケージは最初は Private で作られるので、これをしないと利用者は `docker pull` できない
7. ログインしていない状態（`docker logout ghcr.io` のあと）で `docker pull ghcr.io/kom3da/pagewright:0.2.0` が通ることを確かめてから、リリースを知らせる

## リポジトリを公開するとき（一度だけ）

リポジトリを Public にしたら、Settings で次を有効にする。

- **Private vulnerability reporting**（Security → Advisories）：[SECURITY.md](SECURITY.md) の報告の窓口になる
- **Reported content**（Moderation options）：Issue やコメントの通報を受け付ける（[CODE_OF_CONDUCT.md](CODE_OF_CONDUCT.md)）

## 試験版（プレリリース）

`v0.2.0-rc.1` のように `-` の付いたタグは、GitHub でプレリリースになり、イメージに `0.2` と `latest` のタグは付きません。
CHANGELOG.md にその版の節がなければ `## [Unreleased]` がリリースノートになり、`deploy/.env.example` は前の版のままでかまいません。

## うまくいかなかったとき

- preflight が失敗した：CHANGELOG.md の節か `.env.example` の版が足りない。タグを消して（`git push --delete origin v0.2.0`、`git tag -d v0.2.0`）直してから付け直す
- 公開のあとで問題が見つかった：同じ版を作り直さず、パッチの版を出す
