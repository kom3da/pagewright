import type {
  Category,
  CategoryInput,
  CategoryPatch,
  ContentDetail,
  ContentPatch,
  ContentSummary,
  ContentType,
  DraftInput,
  Me,
  Media,
  Tag,
  TagInput,
} from "./types";

export class ApiError extends Error {
  constructor(
    readonly status: number,
    readonly code: string,
    message: string,
  ) {
    super(message);
    this.name = "ApiError";
  }
}

export const isApiError = (e: unknown, status?: number, code?: string): e is ApiError =>
  e instanceof ApiError &&
  (status === undefined || e.status === status) &&
  (code === undefined || e.code === code);

/** Turns any thrown value into a message that can be shown to the user as-is. */
export const errorMessage = (e: unknown): string =>
  e instanceof ApiError ? e.message : "エラーが起きました。時間をおいてもう一度お試しください。";

type Body = FormData | object | undefined;

async function request<T>(method: string, path: string, body?: Body): Promise<T> {
  const headers: Record<string, string> = { Accept: "application/json" };
  // Required by the API for CSRF protection on every non-GET request.
  if (method !== "GET") headers["X-Pagewright"] = "1";
  let payload: BodyInit | undefined;
  if (body instanceof FormData) {
    payload = body;
  } else if (body !== undefined) {
    headers["Content-Type"] = "application/json";
    payload = JSON.stringify(body);
  }

  let res: Response;
  try {
    res = await fetch(`/api${path}`, {
      method,
      headers,
      body: payload,
      credentials: "same-origin",
    });
  } catch {
    throw new ApiError(0, "network", "サーバーにつながりませんでした。接続を確認してください。");
  }

  if (res.ok) {
    if (res.status === 204 || res.status === 202) return undefined as T;
    const text = await res.text();
    return (text ? JSON.parse(text) : undefined) as T;
  }

  const data = await res.json().catch(() => null);
  const err = data?.error;
  if (err && typeof err.code === "string" && typeof err.message === "string") {
    throw new ApiError(res.status, err.code, err.message);
  }
  throw new ApiError(
    res.status,
    "unknown",
    `エラーが起きました（${res.status}）。時間をおいてもう一度お試しください。`,
  );
}

const get = <T>(path: string) => request<T>("GET", path);
const post = <T>(path: string, body?: Body) => request<T>("POST", path, body);
const put = <T>(path: string, body?: Body) => request<T>("PUT", path, body);
const patch = <T>(path: string, body?: Body) => request<T>("PATCH", path, body);
const del = (path: string) => request<void>("DELETE", path);

export const api = {
  requestLoginLink: (email: string) => post<void>("/auth/login-link", { email }),
  verify: (token: string) => post<Me>("/auth/verify", { token }),
  logout: () => post<void>("/auth/logout"),
  me: () => get<Me>("/me"),
  selectSite: (siteId: string) => post<Me>("/me/site", { siteId }),

  listContents: (type: ContentType) => get<ContentSummary[]>(`/contents?type=${type}`),
  createContent: (type: ContentType) => post<ContentDetail>("/contents", { type }),
  getContent: (id: number) => get<ContentDetail>(`/contents/${id}`),
  saveDraft: (id: number, input: DraftInput) => put<ContentDetail>(`/contents/${id}/draft`, input),
  patchContent: (id: number, input: ContentPatch) => patch<ContentDetail>(`/contents/${id}`, input),
  publish: (id: number) => post<ContentDetail>(`/contents/${id}/publish`),
  unpublish: (id: number) => post<ContentDetail>(`/contents/${id}/unpublish`),
  deleteContent: (id: number) => del(`/contents/${id}`),

  listCategories: () => get<Category[]>("/categories"),
  createCategory: (input: CategoryInput) => post<Category>("/categories", input),
  patchCategory: (id: number, input: CategoryPatch) => patch<Category>(`/categories/${id}`, input),
  deleteCategory: (id: number) => del(`/categories/${id}`),

  listTags: () => get<Tag[]>("/tags"),
  createTag: (input: TagInput) => post<Tag>("/tags", input),
  deleteTag: (id: number) => del(`/tags/${id}`),

  listMedia: () => get<Media[]>("/media"),
  uploadMedia: (file: File, altText: string) => {
    const form = new FormData();
    form.append("file", file);
    form.append("altText", altText);
    return post<Media>("/media", form);
  },
  patchMedia: (id: number, altText: string) => patch<Media>(`/media/${id}`, { altText }),

  buildSite: () => post<void>("/site/build"),
};

/** URL the admin uses to display a media file (requires login). */
export const mediaFileUrl = (mediaId: number) => `/api/media/${mediaId}/file`;
