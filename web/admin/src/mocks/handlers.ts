import { HttpResponse, http } from "msw";
import type {
  Category,
  ContentDetail,
  ContentSummary,
  ContentType,
  Me,
  Media,
  Tag,
} from "../api/types";
import { slugProblem } from "../lib/slug";

// In-memory mock of docs/dev/admin-api.md, used by the unit tests and `pnpm dev:mock`.

type Db = {
  loggedIn: boolean;
  me: Me;
  contents: ContentDetail[];
  categories: Category[];
  tags: Tag[];
  media: Media[];
  builds: number;
  nextId: number;
};

const now = () => new Date().toISOString();

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
    builds: 0,
    nextId: 100,
    ...overrides,
  };
}

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
  alt_required: "altText",
  file_required: "file",
};

const error = (status: number, code: string, message: string) =>
  HttpResponse.json(
    { error: { code, message, ...(errorFields[code] && { field: errorFields[code] }) } },
    { status },
  );

const summary = (c: ContentDetail): ContentSummary => {
  const { categoryIds, tagIds, revision, publishedRevisionId, scheduledRevisionId, ...s } = c;
  return s;
};

/** Recomputes the derived fields after a change, as the server does. */
function refresh(c: ContentDetail) {
  c.status = c.publishedRevisionId ? "published" : c.scheduledRevisionId ? "scheduled" : "draft";
  c.hasUnpublishedChanges =
    c.publishedRevisionId !== null && c.publishedRevisionId !== c.revision.id;
  c.url = c.publishedRevisionId ? contentUrl(c) : null;
  c.updatedAt = now();
}

/** Auth + CSRF + site checks shared by every handler that needs them. */
function guard(request: Request, { site = true } = {}) {
  if (request.method !== "GET" && request.headers.get("X-Pagewright") !== "1") {
    return error(403, "csrf", "不正なリクエストです。");
  }
  if (!db.loggedIn) return error(401, "unauthenticated", "ログインしてください。");
  if (site && !db.me.currentSiteId) return error(403, "no_site", "サイトを選んでください。");
  return null;
}

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

  http.get("/api/contents", ({ request }) => {
    const denied = guard(request);
    if (denied) return denied;
    const type = new URL(request.url).searchParams.get("type");
    return HttpResponse.json(
      db.contents
        .filter((c) => c.type === type)
        .sort((a, b) => b.number - a.number)
        .map(summary),
    );
  }),

  http.post("/api/contents", async ({ request }) => {
    const denied = guard(request);
    if (denied) return denied;
    const { type } = (await request.json()) as { type: ContentType };
    const id = db.nextId++;
    const number = Math.max(0, ...db.contents.map((c) => c.number)) + 1;
    const c: ContentDetail = {
      id,
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
    if (input.baseRevisionId !== undefined && input.baseRevisionId !== c.revision.id) {
      return error(
        409,
        "edit_conflict",
        "ほかの人がこの内容を先に保存しました。いまの編集内容を控えてから、画面を読み込み直してください",
      );
    }
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
      if (db.contents.some((o) => o.id !== c.id && o.type === c.type && o.slug === input.slug)) {
        return error(409, "conflict", "このURLはすでに使われています。別のURLにしてください。");
      }
    }
    Object.assign(c, {
      ...("slug" in input && { slug: input.slug }),
      ...("parentId" in input && { parentId: input.parentId }),
      ...(input.categoryIds && { categoryIds: input.categoryIds }),
      ...(input.tagIds && { tagIds: input.tagIds }),
    });
    refresh(c);
    return HttpResponse.json(c);
  }),

  http.post("/api/contents/:id/publish", ({ request, params }) => {
    const denied = guard(request);
    if (denied) return denied;
    const c = findContent(params.id);
    if (!c) return error(404, "not_found", "見つかりません。");
    // Like the server, publishing now leaves an existing schedule in place.
    Object.assign(c, { publishedRevisionId: c.revision.id, publishedAt: c.publishedAt ?? now() });
    refresh(c);
    db.builds++;
    return HttpResponse.json(c);
  }),

  http.post("/api/contents/:id/unpublish", ({ request, params }) => {
    const denied = guard(request);
    if (denied) return denied;
    const c = findContent(params.id);
    if (!c) return error(404, "not_found", "見つかりません。");
    c.publishedRevisionId = null;
    refresh(c);
    db.builds++;
    return HttpResponse.json(c);
  }),

  http.post("/api/contents/:id/schedule", async ({ request, params }) => {
    const denied = guard(request);
    if (denied) return denied;
    const c = findContent(params.id);
    if (!c) return error(404, "not_found", "見つかりません。");
    const { publishAt } = (await request.json()) as { publishAt: string };
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
    db.builds++;
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
        "この画像は記事やページで使われているため削除できません。先に本文から外してください",
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

  http.post("/api/site/build", ({ request }) => {
    const denied = guard(request);
    if (denied) return denied;
    db.builds++;
    return new HttpResponse(null, { status: 202 });
  }),
];

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
  const parent = db.contents.find((p) => p.id === c.parentId);
  return parent ? `/${parent.slug ?? parent.number}/${last}/` : `/${last}/`;
}
