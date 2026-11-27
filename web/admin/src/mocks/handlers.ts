import { HttpResponse, http } from "msw";
import type {
  Build,
  Category,
  ContentCounts,
  ContentDetail,
  ContentSummary,
  ContentType,
  Me,
  Media,
  MenuItem,
  PreviewLink,
  Session,
  SiteSettings,
  Submission,
  Tag,
  ThemeInfo,
} from "../api/types";
import { contrast, isHexColor, MIN_CONTRAST } from "../lib/color";
import { slugProblem } from "../lib/slug";

// In-memory mock of docs/dev/admin-api.md, used by the unit tests and `pnpm dev:mock`.

type Db = {
  loggedIn: boolean;
  me: Me;
  contents: ContentDetail[];
  categories: Category[];
  tags: Tag[];
  media: Media[];
  /** Settings of the current site. The mock keeps one set for every site. */
  site: SiteSettings;
  submissions: Submission[];
  /** The logged-in user's sessions; the one with `current` is this browser. */
  sessions: Session[];
  /** The real server stores only a hash of the token; the mock keeps it for tests. */
  previewLinks: (PreviewLink & { contentId: number; token: string })[];
  builds: number;
  /** Finished builds, newest first (GET /api/site/builds). */
  buildLog: Build[];
  nextId: number;
  /** Live settings of contents whose saved settings wait for publishing, by content id. */
  liveSettings: Record<number, LiveSettings>;
};

type LiveSettings = Pick<
  ContentDetail,
  "slug" | "parentId" | "categoryIds" | "tagIds" | "publishedAt"
>;

const settingsOf = (c: ContentDetail): LiveSettings => ({
  slug: c.slug,
  parentId: c.parentId,
  categoryIds: [...c.categoryIds],
  tagIds: [...c.tagIds],
  publishedAt: c.publishedAt,
});

/** The settings of a content as the public site has them. */
const live = (c: ContentDetail): ContentDetail => ({ ...c, ...db.liveSettings[c.id] });

const now = () => new Date().toISOString();

/** Where the mock says the public site is (the dev fallback of the admin). */
const MOCK_PUBLIC_BASE_URL = "http://localhost:8081";

const siteA = "11111111-1111-4111-8111-111111111111";
const siteB = "22222222-2222-4222-8222-222222222222";

export function seedDb(overrides: Partial<Db> = {}): Db {
  const t = "2026-11-20T01:00:00Z";
  const content = (
    id: number,
    type: ContentType,
    title: string,
    extra: Partial<ContentDetail> = {},
  ): ContentDetail => ({
    id,
    latestRevisionId: id * 100,
    number: id,
    type,
    title,
    slug: null,
    status: "draft",
    hasUnpublishedChanges: false,
    url: null,
    publishedAt: null,
    scheduledAt: null,
    updatedAt: t,
    parentId: null,
    categoryIds: [],
    tagIds: [],
    revision: {
      id: id * 100,
      title,
      body: {
        type: "doc",
        content: [{ type: "paragraph", content: [{ type: "text", text: `${title}の本文です。` }] }],
      },
      bodySchemaVersion: 1,
      fields: {},
      createdAt: t,
    },
    publishedRevisionId: null,
    scheduledRevisionId: null,
    pendingSettings: false,
    ...extra,
  });
  return {
    loggedIn: true,
    me: {
      user: { id: "u1", email: "taro@example.jp", displayName: "山田太郎" },
      sites: [
        { id: siteA, name: "山田工務店", role: "admin" },
        { id: siteB, name: "山田カフェ", role: "editor" },
      ],
      currentSiteId: siteA,
    },
    contents: [
      content(3, "post", "年末年始の営業について", {
        status: "published",
        slug: "new-year",
        url: "/posts/new-year/",
        publishedAt: t,
        hasUnpublishedChanges: true,
        publishedRevisionId: 299,
        categoryIds: [1],
      }),
      content(2, "post", "新しい施工事例を追加しました", {
        status: "scheduled",
        scheduledAt: "2026-12-01T00:00:00Z",
        scheduledRevisionId: 200,
      }),
      content(1, "post", "ホームページを公開しました"),
      content(4, "page", "会社概要", {
        status: "published",
        slug: "company",
        url: "/company/",
        publishedAt: t,
        publishedRevisionId: 400,
      }),
      content(5, "page", "沿革", {
        status: "published",
        slug: "history",
        url: "/company/history/",
        publishedAt: t,
        publishedRevisionId: 500,
        parentId: 4,
      }),
      content(6, "page", "お問い合わせ"),
    ],
    categories: [
      { id: 1, name: "お知らせ", slug: "news", parentId: null, position: 0 },
      { id: 2, name: "施工事例", slug: "works", parentId: null, position: 1 },
      { id: 3, name: "リフォーム", slug: "reform", parentId: 2, position: 0 },
    ],
    tags: [
      { id: 1, name: "イベント", slug: "event" },
      { id: 2, name: "キャンペーン", slug: "campaign" },
    ],
    media: [],
    site: {
      name: "山田工務店",
      logoMediaId: null,
      logo: null,
      timeZone: "Asia/Tokyo",
      theme: "corporate",
      themeSettings: { corporate: { phone: "03-1234-5678" } },
      themes: mockThemes,
      publicBaseUrl: MOCK_PUBLIC_BASE_URL,
      menu: [],
    },
    submissions: [
      {
        id: 2,
        name: "佐藤花子",
        kana: "さとうはなこ",
        email: "hanako@example.jp",
        phone: "090-1234-5678",
        message: "キッチンのリフォームについて相談したいです。\n見積もりは無料でしょうか。",
        pagePath: "/contact/",
        createdAt: "2026-11-21T03:15:00Z",
        handledAt: null,
        handledBy: null,
      },
      {
        id: 1,
        name: "鈴木一郎",
        kana: "",
        email: "ichiro@example.com",
        phone: "",
        message: "見学会の日程を教えてください。",
        pagePath: "/contact/",
        createdAt: "2026-11-19T23:40:00Z",
        handledAt: null,
        handledBy: null,
      },
    ],
    sessions: [
      {
        id: "a".repeat(64),
        device: "Windows の Chrome",
        createdAt: "2026-11-20T00:00:00Z",
        lastUsedAt: "2026-11-21T03:00:00Z",
        expiresAt: "2026-12-20T00:00:00Z",
        current: true,
      },
      {
        id: "b".repeat(64),
        device: "iPhone の Safari",
        createdAt: "2026-11-18T09:30:00Z",
        lastUsedAt: "2026-11-20T12:00:00Z",
        expiresAt: "2026-12-18T09:30:00Z",
        current: false,
      },
      {
        id: "c".repeat(64),
        device: "",
        createdAt: "2026-11-01T00:00:00Z",
        lastUsedAt: "2026-11-02T00:00:00Z",
        expiresAt: "2026-12-01T00:00:00Z",
        current: false,
      },
    ],
    previewLinks: [],
    builds: 0,
    buildLog: [
      {
        id: 1,
        startedAt: "2026-11-20T01:05:00Z",
        finishedAt: "2026-11-20T01:05:03Z",
        files: 24,
        changed: 3,
        removed: 0,
        warnings: [],
        error: null,
      },
    ],
    nextId: 100,
    liveSettings: {},
    ...overrides,
  };
}

/**
 * The themes GET /api/site lists. "corporate" mirrors themes/corporate/theme.json; "simple"
 * exists only in the mock, to have a second theme with url and page settings.
 */
export const mockThemes: ThemeInfo[] = [
  {
    name: "corporate",
    title: "コーポレート",
    description:
      "中小企業の会社サイト向け。トップに大見出しとお知らせ、固定ページの案内、ご相談の帯を並べます。",
    settingsTitle: "会社の連絡先",
    settingsDescription: "ページの下（フッター）と「お気軽にご相談ください」の帯に表示します。",
    settings: [
      {
        key: "address",
        label: "住所",
        description: "フッターに表示します。",
        type: "text",
        maxLength: 200,
      },
      {
        key: "phone",
        label: "電話番号",
        description: "フッターとご相談の帯に表示し、スマートフォンではそのまま電話をかけられます。",
        type: "tel",
        maxLength: 0,
      },
      {
        key: "businessHours",
        label: "受付時間",
        description: "例：平日 9:00〜18:00。電話番号と一緒に表示します。",
        type: "text",
        maxLength: 0,
      },
    ],
  },
  {
    name: "simple",
    title: "シンプル",
    description: "お店や個人のサイト向け。写真を大きく見せ、文字を控えめにします。",
    settingsTitle: "",
    settingsDescription: "",
    settings: [
      {
        key: "instagram",
        label: "Instagram のURL",
        description: "",
        type: "url",
        maxLength: 0,
      },
      {
        key: "recruitPage",
        label: "採用情報のページ",
        description: "フッターにリンクを出します。",
        type: "page",
        maxLength: 0,
      },
    ],
  },
];

export let db = seedDb();
export const resetDb = (overrides: Partial<Db> = {}) => {
  db = seedDb(overrides);
  return db;
};

// Mirrors errorFields in internal/api/server.go.
const errorFields: Record<string, string> = {
  invalid_title: "title",
  invalid_slug: "slug",
  conflict: "slug",
  invalid_parent: "parentId",
  invalid_name: "name",
  invalid_publish_at: "publishAt",
  invalid_published_at: "publishedAt",
  invalid_days: "days",
  alt_required: "altText",
  file_required: "file",
  invalid_menu: "menu",
  invalid_logo: "logoMediaId",
  invalid_front_page: "frontPageId",
  invalid_privacy_page: "privacyPageId",
};

const error = (status: number, code: string, message: string) =>
  HttpResponse.json(
    { error: { code, message, ...(errorFields[code] && { field: errorFields[code] }) } },
    { status },
  );

/** Lists show the live settings, as the server does. */
const summary = (c: ContentDetail): ContentSummary => {
  const { categoryIds, tagIds, revision, publishedRevisionId, pendingSettings, ...s } = live(c);
  return s;
};

/** Recomputes the derived fields after a change, as the server does. */
function refresh(c: ContentDetail) {
  c.status = c.publishedRevisionId ? "published" : c.scheduledRevisionId ? "scheduled" : "draft";
  c.latestRevisionId = c.revision.id;
  // Saved settings wait only while the content is live.
  if (!c.publishedRevisionId) delete db.liveSettings[c.id];
  c.pendingSettings = c.id in db.liveSettings;
  c.hasUnpublishedChanges =
    c.publishedRevisionId !== null &&
    (c.publishedRevisionId !== c.revision.id || c.pendingSettings);
  c.url = c.publishedRevisionId ? contentUrl(live(c)) : null;
  c.updatedAt = now();
}

/** Auth + CSRF + site checks shared by every handler that needs them. */
function guard(request: Request, { site = true } = {}) {
  if (request.method !== "GET" && request.headers.get("X-Pagewright") !== "1") {
    return error(403, "csrf", "不正なリクエストです。");
  }
  if (!db.loggedIn) return error(401, "unauthenticated", "ログインしてください。");
  if (site && !db.me.currentSiteId) return error(403, "no_site", "サイトを選んでください。");
  const shown = request.headers.get("X-Pagewright-Site");
  if (site && shown && shown !== db.me.currentSiteId) {
    return error(409, "site_changed", "別の画面で管理するサイトが切り替えられました。");
  }
  return null;
}

const fieldError = (code: string, message: string, field: string) =>
  HttpResponse.json({ error: { code, message, field } }, { status: 422 });

const currentRole = () => db.me.sites.find((s) => s.id === db.me.currentSiteId)?.role;

/** Like guard, plus the admin-only check of the settings and contact endpoints. */
function adminGuard(request: Request, message: string) {
  return guard(request) ?? (currentRole() === "admin" ? null : error(403, "forbidden", message));
}

const siteKeys = new Set([
  "name",
  "logoMediaId",
  "primaryColor",
  "footerColor",
  "font",
  "timeZone",
  "contactEmail",
  "menu",
  "theme",
  "frontPageId",
  "privacyPageId",
  "themeSettings",
]);

const isPage = (id: unknown) =>
  typeof id === "number" && db.contents.some((c) => c.id === id && c.type === "page");

/** Mirrors checkThemeSettings in internal/api/site.go and CheckSettings in internal/build. */
function checkThemeSettings(all: Record<string, Record<string, unknown>>) {
  for (const [name, values] of Object.entries(all)) {
    const info = mockThemes.find((t) => t.name === name);
    if (!info)
      return fieldError("invalid_setting", "入っていないテーマの設定です", "themeSettings");
    for (const [key, v] of Object.entries(values)) {
      const field = `themeSettings.${key}`;
      const d = info.settings.find((s) => s.key === key);
      if (!d) return fieldError("invalid_setting", "このテーマにはない設定です", field);
      if (v == null) continue;
      if (d.type === "page") {
        if (!isPage(v)) return fieldError("invalid_setting", "固定ページを選んでください", field);
        continue;
      }
      if (typeof v !== "string") {
        return fieldError("invalid_setting", `${d.label}は文字で入力してください`, field);
      }
      const t = v.trim();
      const max = d.maxLength || 100;
      if ([...t].length > max) {
        return fieldError(
          "invalid_setting",
          `${d.label}は改行なしの${max}文字以内で入力してください`,
          field,
        );
      }
      if (t && d.type === "tel" && !/^[0-9+()\- ]{1,30}$/.test(t)) {
        return fieldError(
          "invalid_setting",
          `${d.label}は数字とハイフンで入力してください（例：03-1234-5678）`,
          field,
        );
      }
      if (t && d.type === "url" && !/^(https?:\/\/|\/(?!\/))/.test(t)) {
        return fieldError(
          "invalid_setting",
          `${d.label}は https:// か / で始まるURLで入力してください`,
          field,
        );
      }
    }
  }
  return null;
}

/** Mirrors putSite in internal/api/site.go and Validate in internal/site/settings.go. */
function validateSite(input: Record<string, unknown>) {
  // The server rejects unknown keys, including the output-only `logo`.
  if (Object.keys(input).some((k) => !siteKeys.has(k))) {
    return error(422, "invalid_json", "送信された内容を読み取れませんでした");
  }
  const s = input as Partial<SiteSettings>;
  const name = (s.name ?? "").trim();
  if (!name || [...name].length > 100) {
    return fieldError("invalid_name", "サイトの名前を100文字以内で入力してください", "name");
  }
  for (const [field, label] of [
    ["primaryColor", "テーマカラー"],
    ["footerColor", "フッターの色"],
  ] as const) {
    const v = s[field];
    if (!v) continue;
    if (!isHexColor(v)) {
      return fieldError(
        "invalid_setting",
        `${label}は #1a73e8 のような6桁の色コードで指定してください`,
        field,
      );
    }
    const r = contrast(v);
    if (r < MIN_CONTRAST) {
      return fieldError(
        "invalid_setting",
        `${label}が明るすぎて、白い文字や背景との差が足りません（コントラスト比 ${r.toFixed(1)}:1、4.5:1 以上が必要です）。もっと濃い色を選んでください`,
        field,
      );
    }
  }
  if (s.font && s.font !== "gothic" && s.font !== "mincho") {
    return fieldError(
      "invalid_setting",
      "文字の書体はゴシック体か明朝体から選んでください",
      "font",
    );
  }
  if (s.timeZone) {
    try {
      new Intl.DateTimeFormat("ja-JP", { timeZone: s.timeZone });
    } catch {
      return fieldError("invalid_setting", "タイムゾーンが正しくありません", "timeZone");
    }
  }
  if (s.theme && !mockThemes.some((t) => t.name === s.theme)) {
    return fieldError("invalid_setting", "そのテーマは入っていません", "theme");
  }
  if (s.contactEmail && !/^[^\s@<>]+@[^\s@<>]+$/.test(s.contactEmail)) {
    return fieldError(
      "invalid_setting",
      "お問い合わせの送り先メールアドレスの形式が正しくありません",
      "contactEmail",
    );
  }
  const menu = s.menu ?? [];
  if (menu.length > 20) return fieldError("invalid_menu", "メニューは20項目までです", "menu");
  for (const [i, it] of menu.entries()) {
    const n = i + 1;
    if ((it.contentId == null) === (it.url == null)) {
      return fieldError(
        "invalid_menu",
        `メニューの${n}番目は、ページかURLのどちらか一方を指定してください`,
        "menu",
      );
    }
    if (it.label != null && (!it.label.trim() || [...it.label.trim()].length > 50)) {
      return fieldError(
        "invalid_menu",
        `メニューの${n}番目の表示名を50文字以内で入力してください`,
        "menu",
      );
    }
    if (it.url != null && (!/^(\/|https?:\/\/)\S*$/.test(it.url) || it.label == null)) {
      return fieldError(
        "invalid_menu",
        `メニューの${n}番目のURLは / か https:// で始め、表示名も入れてください`,
        "menu",
      );
    }
    if (it.contentId != null && !db.contents.some((c) => c.id === it.contentId)) {
      return error(422, "invalid_reference", "指定したものが見つかりません");
    }
  }
  if (s.logoMediaId != null && !db.media.some((m) => m.id === s.logoMediaId)) {
    return fieldError("invalid_logo", "ロゴの画像が見つかりません", "logoMediaId");
  }
  const themeProblem = checkThemeSettings(s.themeSettings ?? {});
  if (themeProblem) return themeProblem;
  if (s.frontPageId && !isPage(s.frontPageId)) {
    return error(422, "invalid_front_page", "トップページには固定ページを選んでください");
  }
  if (s.privacyPageId && !isPage(s.privacyPageId)) {
    return error(
      422,
      "invalid_privacy_page",
      "個人情報の取り扱いのページには固定ページを選んでください",
    );
  }
  return null;
}

const siteResponse = (): SiteSettings => ({
  ...db.site,
  logo: db.media.find((m) => m.id === db.site.logoMediaId) ?? null,
  themes: mockThemes,
});

const invalidSlug = () =>
  error(422, "invalid_slug", "ページのURLは英小文字・数字・ハイフンで入力してください");

function findContent(id: string | readonly string[] | undefined) {
  return db.contents.find((c) => c.id === Number(id));
}

export const handlers = [
  http.post("/api/auth/login-link", async ({ request }) => {
    const denied = guard(request, { site: false });
    if (denied && denied.status !== 401) return denied;
    return new HttpResponse(null, { status: 204 });
  }),

  http.post("/api/auth/verify", async ({ request }) => {
    if (request.headers.get("X-Pagewright") !== "1") return error(403, "csrf", "不正です。");
    const { token } = (await request.json()) as { token: string };
    if (token !== "valid-token") {
      return error(
        400,
        "invalid_token",
        "ログイン用のリンクが無効か、期限が切れています。もう一度リンクを送ってください。",
      );
    }
    db.loggedIn = true;
    return HttpResponse.json(db.me);
  }),

  http.post("/api/auth/logout", ({ request }) => {
    db.loggedIn = false;
    return guard(request, { site: false }) ?? new HttpResponse(null, { status: 204 });
  }),

  http.get("/api/me", ({ request }) => guard(request, { site: false }) ?? HttpResponse.json(db.me)),

  http.post("/api/me/site", async ({ request }) => {
    const denied = guard(request, { site: false });
    if (denied) return denied;
    const { siteId } = (await request.json()) as { siteId: string };
    if (!db.me.sites.some((s) => s.id === siteId)) {
      return error(403, "forbidden", "このサイトを管理する権限がありません。");
    }
    db.me = { ...db.me, currentSiteId: siteId };
    return HttpResponse.json(db.me);
  }),

  http.get(
    "/api/me/sessions",
    ({ request }) => guard(request, { site: false }) ?? HttpResponse.json(db.sessions),
  ),

  http.delete("/api/me/sessions/:id", ({ request, params }) => {
    const denied = guard(request, { site: false });
    if (denied) return denied;
    // The current session is not found here, as on the server (it ends with logout).
    if (!db.sessions.some((s) => s.id === params.id && !s.current)) {
      return error(404, "not_found", "見つかりません。");
    }
    db.sessions = db.sessions.filter((s) => s.id !== params.id);
    return new HttpResponse(null, { status: 204 });
  }),

  http.post("/api/me/sessions/logout-others", ({ request }) => {
    const denied = guard(request, { site: false });
    if (denied) return denied;
    const revoked = db.sessions.filter((s) => !s.current).length;
    db.sessions = db.sessions.filter((s) => s.current);
    return HttpResponse.json({ revoked });
  }),

  http.get("/api/contents", ({ request }) => {
    const denied = guard(request);
    if (denied) return denied;
    const params = new URL(request.url).searchParams;
    const type = params.get("type");
    const q = params.get("q")?.trim().toLowerCase();
    const status = params.get("status");
    if (q && [...q].length > 100) {
      return error(422, "invalid_q", "検索する言葉は100文字以内で入力してください");
    }
    if (status && !["draft", "scheduled", "published", "changed"].includes(status)) {
      return error(422, "invalid_status", "状態の指定が正しくありません");
    }
    const before = params.has("before") ? Number(params.get("before")) : undefined;
    const limit = params.has("limit") ? Number(params.get("limit")) : 1000;
    if (before !== undefined && !(Number.isInteger(before) && before > 0)) {
      return error(422, "invalid_before", "ページの指定が正しくありません");
    }
    if (!(Number.isInteger(limit) && limit >= 1 && limit <= 1000)) {
      return error(422, "invalid_limit", "件数は1から1000のあいだで指定してください");
    }
    const sort = params.get("sort");
    if (sort && (sort !== "updated" || q || status || before !== undefined)) {
      return error(422, "invalid_sort", "並べ替えの指定が正しくありません");
    }
    const order =
      sort === "updated"
        ? (a: ContentDetail, b: ContentDetail) =>
            Date.parse(b.updatedAt) - Date.parse(a.updatedAt) || b.number - a.number
        : (a: ContentDetail, b: ContentDetail) => b.number - a.number;
    return HttpResponse.json(
      db.contents
        .filter((c) => c.type === type)
        .filter((c) => before === undefined || c.number < before)
        .filter(
          (c) =>
            !q || c.title.toLowerCase().includes(q) || (c.slug ?? "").toLowerCase().includes(q),
        )
        .filter((c) =>
          status === "changed"
            ? c.status === "published" && c.hasUnpublishedChanges
            : !status || c.status === status,
        )
        .sort(order)
        .slice(0, limit)
        .map(summary),
    );
  }),

  http.get("/api/contents/counts", ({ request }) => {
    const denied = guard(request);
    if (denied) return denied;
    const count = (type: ContentType): ContentCounts => {
      const of = db.contents.filter((c) => c.type === type);
      const scheduled = of.filter((c) => c.status === "scheduled");
      return {
        draft: of.filter((c) => c.status === "draft").length,
        scheduled: scheduled.length,
        published: of.filter((c) => c.status === "published").length,
        changed: of.filter((c) => c.hasUnpublishedChanges).length,
        nextScheduledAt:
          scheduled
            .map((c) => c.scheduledAt)
            .filter((d): d is string => d !== null)
            .sort((a, b) => Date.parse(a) - Date.parse(b))[0] ?? null,
      };
    };
    return HttpResponse.json({ post: count("post"), page: count("page") });
  }),

  http.post("/api/contents", async ({ request }) => {
    const denied = guard(request);
    if (denied) return denied;
    const { type } = (await request.json()) as { type: ContentType };
    const id = db.nextId++;
    const number = Math.max(0, ...db.contents.map((c) => c.number)) + 1;
    const c: ContentDetail = {
      id,
      latestRevisionId: id * 100,
      number,
      type,
      title: "",
      slug: null,
      status: "draft",
      hasUnpublishedChanges: false,
      url: null,
      publishedAt: null,
      scheduledAt: null,
      updatedAt: now(),
      parentId: null,
      categoryIds: [],
      tagIds: [],
      revision: {
        id: id * 100,
        title: "",
        body: { type: "doc", content: [] },
        bodySchemaVersion: 1,
        fields: {},
        createdAt: now(),
      },
      publishedRevisionId: null,
      scheduledRevisionId: null,
      pendingSettings: false,
    };
    db.contents.push(c);
    return HttpResponse.json(c, { status: 201 });
  }),

  http.get("/api/contents/:id", ({ request, params }) => {
    const denied = guard(request);
    if (denied) return denied;
    const c = findContent(params.id);
    return c ? HttpResponse.json(c) : error(404, "not_found", "見つかりません。");
  }),

  http.put("/api/contents/:id/draft", async ({ request, params }) => {
    const denied = guard(request);
    if (denied) return denied;
    const c = findContent(params.id);
    if (!c) return error(404, "not_found", "見つかりません。");
    const input = (await request.json()) as {
      title: string;
      body: ContentDetail["revision"]["body"];
      fields?: Record<string, unknown>;
      baseRevisionId?: number;
    };
    if (staleBase(c, input.baseRevisionId)) return editConflict();
    c.revision = {
      id: c.revision.id + 1,
      title: input.title,
      body: input.body,
      bodySchemaVersion: 1,
      fields: input.fields ?? c.revision.fields,
      createdAt: now(),
    };
    c.title = input.title;
    refresh(c);
    return HttpResponse.json(c);
  }),

  http.patch("/api/contents/:id", async ({ request, params }) => {
    const denied = guard(request);
    if (denied) return denied;
    const c = findContent(params.id);
    if (!c) return error(404, "not_found", "見つかりません。");
    const input = (await request.json()) as Partial<ContentDetail>;
    if (input.slug !== undefined && input.slug !== null) {
      if (slugProblem(input.slug)) return invalidSlug();
      const taken = (o: ContentDetail) => o.slug === input.slug || live(o).slug === input.slug;
      if (db.contents.some((o) => o.id !== c.id && o.type === c.type && taken(o))) {
        return error(409, "conflict", "このURLはすでに使われています。別のURLにしてください。");
      }
    }
    if (input.publishedAt !== undefined && input.publishedAt !== null) {
      const at = new Date(input.publishedAt);
      if (Number.isNaN(at.getTime()) || at.getTime() > Date.now() + 60_000) {
        return error(
          422,
          "invalid_published_at",
          "公開日は今より前の日時にしてください。先の日時に公開するときは「日時を指定して公開」を使ってください",
        );
      }
    }
    // A live content keeps its live settings until 「変更を公開する」.
    const before = db.liveSettings[c.id] ?? settingsOf(c);
    if (input.publishedAt !== undefined && input.publishedAt !== null) {
      c.publishedAt = new Date(input.publishedAt).toISOString().replace(".000Z", "Z");
    }
    Object.assign(c, {
      ...("slug" in input && { slug: input.slug }),
      ...("parentId" in input && { parentId: input.parentId }),
      ...(input.categoryIds && { categoryIds: input.categoryIds }),
      ...(input.tagIds && { tagIds: input.tagIds }),
    });
    if (c.publishedRevisionId) {
      const same = JSON.stringify(settingsOf(c)) === JSON.stringify(before);
      if (same) delete db.liveSettings[c.id];
      else db.liveSettings[c.id] = before;
    }
    refresh(c);
    return HttpResponse.json(c);
  }),

  http.post("/api/contents/:id/publish", async ({ request, params }) => {
    const denied = guard(request);
    if (denied) return denied;
    const c = findContent(params.id);
    if (!c) return error(404, "not_found", "見つかりません。");
    const text = await request.text();
    const input = (text ? JSON.parse(text) : {}) as { baseRevisionId?: number };
    if (staleBase(c, input.baseRevisionId)) return editConflict();
    // Like the server, publishing now cancels a pending schedule and applies saved settings.
    delete db.liveSettings[c.id];
    Object.assign(c, {
      publishedRevisionId: c.revision.id,
      publishedAt: c.publishedAt ?? now(),
      scheduledAt: null,
      scheduledRevisionId: null,
    });
    refresh(c);
    db.builds++;
    return HttpResponse.json(c);
  }),

  http.post("/api/contents/:id/unpublish", ({ request, params }) => {
    const denied = guard(request);
    if (denied) return denied;
    const c = findContent(params.id);
    if (!c) return error(404, "not_found", "見つかりません。");
    // Unpublishing also cancels a pending schedule; saved settings are kept as the draft's.
    delete db.liveSettings[c.id];
    Object.assign(c, { publishedRevisionId: null, scheduledAt: null, scheduledRevisionId: null });
    refresh(c);
    db.builds++;
    return HttpResponse.json(c);
  }),

  http.post("/api/contents/:id/schedule", async ({ request, params }) => {
    const denied = guard(request);
    if (denied) return denied;
    const c = findContent(params.id);
    if (!c) return error(404, "not_found", "見つかりません。");
    const { publishAt, baseRevisionId } = (await request.json()) as {
      publishAt: string;
      baseRevisionId?: number;
    };
    if (staleBase(c, baseRevisionId)) return editConflict();
    const at = new Date(publishAt);
    if (Number.isNaN(at.getTime()) || at.getTime() < Date.now() + 60_000) {
      return error(422, "invalid_publish_at", "公開する日時は、今から1分以上あとにしてください");
    }
    c.scheduledRevisionId = c.revision.id;
    c.scheduledAt = at.toISOString().replace(".000Z", "Z");
    refresh(c);
    return HttpResponse.json(c);
  }),

  http.delete("/api/contents/:id/schedule", ({ request, params }) => {
    const denied = guard(request);
    if (denied) return denied;
    const c = findContent(params.id);
    if (!c) return error(404, "not_found", "見つかりません。");
    c.scheduledRevisionId = null;
    c.scheduledAt = null;
    refresh(c);
    return HttpResponse.json(c);
  }),

  http.delete("/api/contents/:id", ({ request, params }) => {
    const denied = guard(request);
    if (denied) return denied;
    db.contents = db.contents.filter((c) => c.id !== Number(params.id));
    db.previewLinks = db.previewLinks.filter((l) => l.contentId !== Number(params.id));
    db.builds++;
    return new HttpResponse(null, { status: 204 });
  }),

  http.post("/api/contents/:id/preview-links", async ({ request, params }) => {
    const denied = guard(request);
    if (denied) return denied;
    const c = findContent(params.id);
    if (!c) return error(404, "not_found", "見つかりません。");
    const { days = 3 } = (await request.json()) as { days?: number };
    if (!Number.isInteger(days) || days < 1 || days > 30) {
      return error(422, "invalid_days", "有効期間は1日から30日のあいだで指定してください");
    }
    const id = db.nextId++;
    const token = `token-${id}`;
    const created = new Date();
    const link = {
      id,
      contentId: c.id,
      token,
      createdAt: created.toISOString(),
      expiresAt: new Date(created.getTime() + days * 24 * 60 * 60 * 1000).toISOString(),
    };
    db.previewLinks.push(link);
    const out: PreviewLink = {
      id,
      url: `http://localhost:8080/p/${token}/`,
      expiresAt: link.expiresAt,
      createdAt: link.createdAt,
    };
    return HttpResponse.json(out, { status: 201 });
  }),

  http.get("/api/contents/:id/preview-links", ({ request, params }) => {
    const denied = guard(request);
    if (denied) return denied;
    const c = findContent(params.id);
    if (!c) return error(404, "not_found", "見つかりません。");
    return HttpResponse.json(
      db.previewLinks
        .filter((l) => l.contentId === c.id && new Date(l.expiresAt).getTime() > Date.now())
        .sort((a, b) => b.id - a.id)
        .map(({ id, expiresAt, createdAt }): PreviewLink => ({ id, expiresAt, createdAt })),
    );
  }),

  http.delete("/api/preview-links/:id", ({ request, params }) => {
    const denied = guard(request);
    if (denied) return denied;
    const id = Number(params.id);
    if (!db.previewLinks.some((l) => l.id === id)) {
      return error(404, "not_found", "見つかりません。");
    }
    db.previewLinks = db.previewLinks.filter((l) => l.id !== id);
    return new HttpResponse(null, { status: 204 });
  }),

  http.get("/api/categories", ({ request }) => guard(request) ?? HttpResponse.json(db.categories)),

  http.post("/api/categories", async ({ request }) => {
    const denied = guard(request);
    if (denied) return denied;
    const input = (await request.json()) as Omit<Category, "id" | "position">;
    if (slugProblem(input.slug)) return invalidSlug();
    if (db.categories.some((c) => c.slug === input.slug)) {
      return error(409, "conflict", "このURLはすでに使われています。");
    }
    const c: Category = {
      ...input,
      parentId: input.parentId ?? null,
      id: db.nextId++,
      position: 0,
    };
    db.categories.push(c);
    return HttpResponse.json(c, { status: 201 });
  }),

  http.patch("/api/categories/:id", async ({ request, params }) => {
    const denied = guard(request);
    if (denied) return denied;
    const c = db.categories.find((x) => x.id === Number(params.id));
    if (!c) return error(404, "not_found", "見つかりません。");
    const input = (await request.json()) as Partial<Category>;
    if (input.slug !== undefined && slugProblem(input.slug)) return invalidSlug();
    Object.assign(c, input);
    return HttpResponse.json(c);
  }),

  http.delete("/api/categories/:id", ({ request, params }) => {
    const denied = guard(request);
    if (denied) return denied;
    const gone = db.categories.find((c) => c.id === Number(params.id));
    if (!gone) return error(404, "not_found", "見つかりません。");
    // Child categories move up one level.
    for (const c of db.categories) if (c.parentId === gone.id) c.parentId = gone.parentId;
    db.categories = db.categories.filter((c) => c.id !== gone.id);
    return new HttpResponse(null, { status: 204 });
  }),

  http.get("/api/tags", ({ request }) => guard(request) ?? HttpResponse.json(db.tags)),

  http.post("/api/tags", async ({ request }) => {
    const denied = guard(request);
    if (denied) return denied;
    const input = (await request.json()) as Omit<Tag, "id">;
    if (slugProblem(input.slug)) return invalidSlug();
    if (db.tags.some((t) => t.slug === input.slug)) {
      return error(409, "conflict", "このURLはすでに使われています。");
    }
    const t: Tag = { ...input, id: db.nextId++ };
    db.tags.push(t);
    return HttpResponse.json(t, { status: 201 });
  }),

  http.patch("/api/tags/:id", async ({ request, params }) => {
    const denied = guard(request);
    if (denied) return denied;
    const t = db.tags.find((x) => x.id === Number(params.id));
    if (!t) return error(404, "not_found", "見つかりません。");
    const input = (await request.json()) as Partial<Tag>;
    if (input.name !== undefined && !input.name.trim()) {
      return error(422, "invalid_name", "名前を100文字以内で入力してください");
    }
    if (input.slug !== undefined) {
      if (slugProblem(input.slug)) return invalidSlug();
      if (db.tags.some((o) => o.id !== t.id && o.slug === input.slug)) {
        return error(409, "conflict", "このURLはすでに使われています。");
      }
    }
    Object.assign(t, input);
    return HttpResponse.json(t);
  }),

  http.delete("/api/tags/:id", ({ request, params }) => {
    const denied = guard(request);
    if (denied) return denied;
    db.tags = db.tags.filter((t) => t.id !== Number(params.id));
    return new HttpResponse(null, { status: 204 });
  }),

  http.get(
    "/api/media",
    ({ request }) => guard(request) ?? HttpResponse.json([...db.media].reverse()),
  ),

  http.post("/api/media", async ({ request }) => {
    const denied = guard(request);
    if (denied) return denied;
    const upload = await readUpload(request);
    if (!upload) return error(422, "file_required", "画像ファイルを選んでください");
    if (!upload.altText.trim()) {
      return error(
        422,
        "alt_required",
        "画像の説明（代替テキスト）を500文字以内で入力してください",
      );
    }
    const id = db.nextId++;
    const m: Media = {
      id,
      url: `/media/${id}/${upload.name}`,
      previewUrl: `/api/media/${id}/file`,
      filename: upload.name,
      contentType: upload.type,
      sizeBytes: upload.size,
      width: null,
      height: null,
      altText: upload.altText,
      createdAt: now(),
    };
    db.media.push(m);
    return HttpResponse.json(m, { status: 201 });
  }),

  http.delete("/api/media/:id", ({ request, params }) => {
    const denied = guard(request);
    if (denied) return denied;
    const id = Number(params.id);
    if (!db.media.some((m) => m.id === id)) return error(404, "not_found", "見つかりません。");
    if (db.contents.some((c) => usesMedia(c, id))) {
      return error(
        409,
        "media_in_use",
        "この画像は記事やページ、またはロゴで使われているため削除できません。先に本文から外して保存してください。公開中・予約中の版で使っている場合は、本文から外して「変更を公開する」まで押すと削除できます。",
      );
    }
    db.media = db.media.filter((m) => m.id !== id);
    return new HttpResponse(null, { status: 204 });
  }),

  http.get("/api/media/:id/file", () => {
    // A 1x1 gray PNG.
    const png = Uint8Array.from(
      atob(
        "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mN89+7dfwAJYAPOIzP4ZAAAAABJRU5ErkJggg==",
      ),
      (ch) => ch.charCodeAt(0),
    );
    return new HttpResponse(png, { headers: { "Content-Type": "image/png" } });
  }),

  http.get("/api/site", ({ request }) => guard(request) ?? HttpResponse.json(siteResponse())),

  http.put("/api/site", async ({ request }) => {
    const denied = adminGuard(request, "サイトの設定を変えられるのは管理者だけです");
    if (denied) return denied;
    const input = (await request.json()) as Record<string, unknown>;
    const invalid = validateSite(input);
    if (invalid) return invalid;
    const s = input as unknown as SiteSettings;
    // Like the server's omitempty, empty optional settings are dropped.
    db.site = {
      name: s.name.trim(),
      logoMediaId: s.logoMediaId ?? null,
      logo: null,
      ...(s.primaryColor && { primaryColor: s.primaryColor }),
      ...(s.footerColor && { footerColor: s.footerColor }),
      ...(s.font && { font: s.font }),
      ...(s.timeZone && { timeZone: s.timeZone }),
      ...(s.contactEmail && { contactEmail: s.contactEmail }),
      ...(s.theme && { theme: s.theme }),
      ...(s.frontPageId && { frontPageId: s.frontPageId }),
      ...(s.privacyPageId && { privacyPageId: s.privacyPageId }),
      ...(s.themeSettings &&
        Object.keys(s.themeSettings).length > 0 && { themeSettings: s.themeSettings }),
      themes: mockThemes,
      publicBaseUrl: db.site.publicBaseUrl,
      menu: (s.menu ?? []).map(
        (it): MenuItem => ({
          label: it.label?.trim() ?? null,
          contentId: it.contentId ?? null,
          url: it.url ?? null,
        }),
      ),
    };
    db.builds++;
    return HttpResponse.json(siteResponse());
  }),

  http.get("/api/forms/submissions", ({ request }) => {
    const denied = adminGuard(request, "お問い合わせの内容を見られるのは管理者だけです");
    if (denied) return denied;
    const params = new URL(request.url).searchParams;
    const before = Number(params.get("before") ?? Number.POSITIVE_INFINITY);
    const handled = params.get("handled");
    if (handled !== null && handled !== "true" && handled !== "false") {
      return error(422, "invalid_handled", "対応の状態の指定が正しくありません");
    }
    return HttpResponse.json(
      db.submissions
        .filter((f) => f.id < before)
        .filter((f) => handled === null || (f.handledAt !== null) === (handled === "true"))
        .sort((a, b) => b.id - a.id)
        .slice(0, 100),
    );
  }),

  http.patch("/api/forms/submissions/:id", async ({ request, params }) => {
    const denied = adminGuard(request, "お問い合わせの内容を見られるのは管理者だけです");
    if (denied) return denied;
    const sub = db.submissions.find((f) => f.id === Number(params.id));
    if (!sub) return error(404, "not_found", "見つかりません。");
    const { handled } = (await request.json()) as { handled?: unknown };
    if (typeof handled !== "boolean") {
      return error(422, "invalid_handled", "対応の状態の指定が正しくありません");
    }
    if (!handled) {
      sub.handledAt = null;
      sub.handledBy = null;
    } else if (!sub.handledAt) {
      sub.handledAt = now();
      sub.handledBy = db.me.user.displayName || db.me.user.email;
    }
    return HttpResponse.json(sub);
  }),

  http.delete("/api/forms/submissions/:id", ({ request, params }) => {
    const denied = adminGuard(request, "お問い合わせの内容を見られるのは管理者だけです");
    if (denied) return denied;
    const id = Number(params.id);
    if (!db.submissions.some((f) => f.id === id))
      return error(404, "not_found", "見つかりません。");
    db.submissions = db.submissions.filter((f) => f.id !== id);
    return new HttpResponse(null, { status: 204 });
  }),

  http.post("/api/site/build", ({ request }) => {
    const denied = guard(request);
    if (denied) return denied;
    db.builds++;
    // The mock finishes the build at once, so the home screen can show the new result.
    const at = now();
    db.buildLog.unshift({
      id: db.nextId++,
      startedAt: at,
      finishedAt: at,
      files: 24,
      changed: 0,
      removed: 0,
      warnings: [],
      error: null,
    });
    return new HttpResponse(null, { status: 202 });
  }),

  http.get("/api/site/builds", ({ request }) => {
    const denied = guard(request);
    if (denied) return denied;
    const limit = Number(new URL(request.url).searchParams.get("limit") ?? 5);
    if (!Number.isInteger(limit) || limit < 1 || limit > 50) {
      return error(422, "invalid_limit", "件数は1から50で指定してください");
    }
    return HttpResponse.json(db.buildLog.slice(0, limit));
  }),
];

/** Someone saved a newer revision than the one the editor sent as its base. */
const staleBase = (c: ContentDetail, base: number | undefined) =>
  base !== undefined && base !== c.revision.id;
const editConflict = () =>
  error(
    409,
    "edit_conflict",
    "ほかの人がこの内容を先に保存しました。いまの編集内容を控えてから、画面を読み込み直してください",
  );

async function readUpload(request: Request) {
  try {
    const form = await request.clone().formData();
    const file = form.get("file");
    if (!(file instanceof Blob)) return null;
    return {
      name: (file as File).name,
      type: file.type,
      size: file.size,
      altText: String(form.get("altText") ?? ""),
    };
  } catch {
    // Under jsdom (unit tests) Node cannot build File objects; read the raw multipart text.
    const text = await request.text();
    const name = /filename="([^"]*)"/.exec(text)?.[1];
    if (!name) return null;
    return {
      name,
      type: /filename="[^"]*"\r\nContent-Type: ([^\r\n]+)/i.exec(text)?.[1] ?? "",
      size: 0,
      altText: /name="altText"\r\n\r\n([^\r]*)\r\n/.exec(text)?.[1] ?? "",
    };
  }
}

// The mock keeps only the latest revision, so only that body is checked.
function usesMedia(c: ContentDetail, mediaId: number) {
  const walk = (n: ContentDetail["revision"]["body"]): boolean =>
    (n.type === "image" && n.attrs?.mediaId === mediaId) || (n.content ?? []).some(walk);
  return walk(c.revision.body);
}

function contentUrl(c: ContentDetail) {
  const last = c.slug ?? String(c.number);
  if (c.type === "post") return `/posts/${last}/`;
  const found = db.contents.find((p) => p.id === c.parentId);
  const parent = found && live(found);
  return parent ? `/${parent.slug ?? parent.number}/${last}/` : `/${last}/`;
}
