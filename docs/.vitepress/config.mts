import { defineConfig, type DefaultTheme } from "vitepress";

const repo = "https://github.com/kom3da/pagewright";

// @mdit-vue/shared's slugify (VitePress's default) with the result recomposed (NFC).
const slugify = (s: string) =>
  s
    .normalize("NFKD")
    .replace(/[\u0300-\u036F]/g, "")
    .replace(/[\u0000-\u001f]/g, "")
    .replace(/[\s~`!@#$%^&*()\-_+=[\]{}|\\;:"'“”‘’<>,.?/]+/g, "-")
    .replace(/-{2,}/g, "-")
    .replace(/^-+|-+$/g, "")
    .replace(/^(\d)/, "_$1")
    .toLowerCase()
    .normalize("NFC");

const guide: DefaultTheme.SidebarItem[] = [
  {
    text: "使い方",
    items: [
      { text: "はじめに", link: "/guide/" },
      { text: "ログインとログアウト", link: "/guide/login" },
      { text: "ホーム", link: "/guide/home" },
      { text: "はじめての記事（画像つきのお知らせを出す）", link: "/guide/first-post" },
      { text: "記事を書く・公開する", link: "/guide/posts" },
      { text: "本文の書き方", link: "/guide/editor" },
      { text: "固定ページと親子", link: "/guide/pages" },
      { text: "カテゴリとタグ", link: "/guide/taxonomy" },
      { text: "画像", link: "/guide/images" },
      { text: "サイトの設定", link: "/guide/settings" },
      { text: "お問い合わせ", link: "/guide/inquiries" },
      { text: "メンバーと招待", link: "/guide/members" },
      { text: "サイトを更新する", link: "/guide/site-update" },
      { text: "困ったとき", link: "/guide/troubleshooting" },
    ],
  },
];

const install: DefaultTheme.SidebarItem[] = [
  {
    text: "最短の道",
    items: [
      { text: "はじめに（必要なもの）", link: "/install/" },
      { text: "メール送信サービスを用意する", link: "/install/mail" },
      { text: "導入", link: "/install/setup" },
      { text: "使う人に渡すもの", link: "/install/handover" },
      { text: "バックアップと復元", link: "/install/backup" },
      { text: "アップデート", link: "/install/update" },
      { text: "困ったとき", link: "/install/troubleshooting" },
    ],
  },
  {
    text: "WordPress から移る",
    items: [{ text: "WordPress からの移行", link: "/install/wordpress" }],
  },
  {
    text: "くわしい設定",
    items: [
      { text: "HTTPS と構成の変更", link: "/install/https" },
      { text: "2つ目のサイト", link: "/install/second-site" },
      { text: "画像の容量・DBパスワード", link: "/install/settings" },
      { text: "版の付け方と互換性", link: "/install/versioning" },
    ],
  },
  {
    text: "資料",
    items: [{ text: "用語集", link: "/install/glossary" }],
  },
];

const themes: DefaultTheme.SidebarItem[] = [
  {
    text: "テーマを作る",
    items: [
      { text: "ファイル構成と theme.json", link: "/themes/" },
      { text: "テンプレートに渡るデータ", link: "/themes/data" },
      { text: "設定項目・色と書体", link: "/themes/settings" },
      { text: "本文の HTML と部品", link: "/themes/body-html" },
      { text: "アクセシビリティ", link: "/themes/accessibility" },
      { text: "本文JSONのスキーマ", link: "/dev/body-schema" },
    ],
  },
];

const dev: DefaultTheme.SidebarItem[] = [
  {
    text: "開発",
    items: [
      { text: "設計", link: "/design" },
      { text: "開発環境", link: "/dev/" },
      { text: "管理画面の API", link: "/dev/admin-api" },
      { text: "管理画面のデザインの決まり", link: "/dev/admin-design" },
      { text: "本文JSONのスキーマ", link: "/dev/body-schema" },
    ],
  },
];

export default defineConfig({
  lang: "ja",
  title: "Pagewright",
  description: "中小企業・個人のWebサイト向けの、セルフホストでも速く、日本語に強いCMS",
  // A broken link fails the build. Only the dev URLs (http://localhost:8080/
  // and so on) are let through: they point at a running dev server.
  ignoreDeadLinks: "localhostLinks",
  lastUpdated: true,
  markdown: {
    // VitePress's slugify decomposes (NFKD) and leaves 「で」 as 「て」+ U+3099, but links
    // written in Markdown stay composed, so #…で… never matched. Same rules, recomposed.
    anchor: { slugify },
    config(md) {
      // Inline code shows Go templates and JSX (`{{.SelfURL}}`, `style={{ … }}`);
      // keep Vue from reading it as interpolation, as it already does for code blocks.
      const render = md.renderer.rules.code_inline!;
      md.renderer.rules.code_inline = (...args) =>
        render(...args).replace(/^<code/, "<code v-pre");
    },
  },
  themeConfig: {
    nav: [
      { text: "使い方", link: "/guide/", activeMatch: "^/guide/" },
      { text: "導入・運用", link: "/install/", activeMatch: "^/install/" },
      { text: "テーマを作る", link: "/themes/", activeMatch: "^/themes/" },
      { text: "開発", link: "/dev/", activeMatch: "^/(dev/|design)" },
    ],
    sidebar: {
      "/guide/": guide,
      "/install/": install,
      "/themes/": themes,
      "/dev/": dev,
      "/design": dev,
    },
    socialLinks: [{ icon: "github", link: repo }],
    editLink: {
      // Runs in the browser, so it cannot use the constants above. Pages that
      // only include a file kept next to its code link to that file.
      pattern: ({ filePath }) => {
        const included: Record<string, string> = {
          "dev/body-schema.md": "schema/body-v1.md",
          "dev/admin-design.md": "web/admin/DESIGN.md",
        };
        const path = included[filePath] ?? `docs/${filePath}`;
        return `https://github.com/kom3da/pagewright/edit/main/${path}`;
      },
      text: "このページを GitHub で直す",
    },
    outline: { level: [2, 3], label: "目次" },
    docFooter: { prev: "前のページ", next: "次のページ" },
    lastUpdated: { text: "最終更新" },
    darkModeSwitchLabel: "表示の色",
    lightModeSwitchTitle: "ライトに切り替える",
    darkModeSwitchTitle: "ダークに切り替える",
    sidebarMenuLabel: "メニュー",
    returnToTopLabel: "ページの先頭へ",
    langMenuLabel: "言語",
    skipToContentLabel: "本文へ移動",
    externalLinkIcon: true,
    notFound: {
      title: "ページが見つかりません",
      quote: "URL が変わったか、ページが削除された可能性があります。",
      linkLabel: "トップへ",
      linkText: "トップへ戻る",
    },
  },
});
