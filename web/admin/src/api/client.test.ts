import { describe, expect, it, vi } from "vitest";
import { db } from "../mocks/handlers";
import { failWith, recordRequests } from "../test/utils";
import { ApiError, api, errorField } from "./client";

describe("api client", () => {
  it("sends X-Pagewright only on non-GET requests, with same-origin credentials", async () => {
    const fetchSpy = vi.spyOn(globalThis, "fetch");
    const requests = recordRequests();
    await api.me();
    await api.buildSite();
    expect(requests.map((r) => [r.method, r.path, r.headers.get("X-Pagewright")])).toEqual([
      ["GET", "/api/me", null],
      ["POST", "/api/site/build", "1"],
    ]);
    for (const [, init] of fetchSpy.mock.calls) expect(init?.credentials).toBe("same-origin");
    fetchSpy.mockRestore();
  });

  it("sends the site it shows, so a tab left on another site is refused", async () => {
    const requests = recordRequests();
    const me = await api.me();
    await api.listTags();
    const other = me.sites[1].id;
    // Another tab switches the session's site.
    db.me.currentSiteId = other;
    expect(await api.deleteTag(1).catch((e) => e)).toMatchObject({
      status: 409,
      code: "site_changed",
    });
    await api.selectSite(other);
    await api.listTags();
    expect(requests.map((r) => [r.method, r.path, r.headers.get("X-Pagewright-Site")])).toEqual([
      ["GET", "/api/me", null],
      ["GET", "/api/tags", me.currentSiteId],
      ["DELETE", "/api/tags/1", me.currentSiteId],
      ["POST", "/api/me/site", me.currentSiteId],
      ["GET", "/api/tags", other],
    ]);
  });

  it("throws ApiError with the server's code and Japanese message", async () => {
    failWith(
      "patch",
      "/api/contents/1",
      422,
      "invalid_slug",
      "ページのURLは英小文字・数字・ハイフンで入力してください",
    );
    const err = await api.patchContent(1, { slug: "会社" }).catch((e) => e);
    expect(err).toBeInstanceOf(ApiError);
    expect(err).toMatchObject({
      status: 422,
      code: "invalid_slug",
      message: "ページのURLは英小文字・数字・ハイフンで入力してください",
    });
  });

  it("reads error.field, falling back to the code when it is missing", async () => {
    const err = await api.schedule(2, "2000-01-01T00:00:00Z").catch((e) => e);
    expect(err).toMatchObject({ status: 422, code: "invalid_publish_at", field: "publishAt" });
    expect(errorField(err)).toBe("publishAt");

    // failWith sends no field, as an older server would.
    failWith("post", "/api/tags", 409, "conflict", "このURLはすでに使われています。");
    const old = await api.createTag({ name: "a", slug: "a" }).catch((e) => e);
    expect(old.field).toBeUndefined();
    expect(errorField(old)).toBe("slug");
    expect(errorField(new ApiError(409, "edit_conflict", "x"))).toBeUndefined();
    expect(errorField(new Error("x"))).toBeUndefined();
  });

  it("calls the schedule, tag and media endpoints", async () => {
    const requests = recordRequests();
    const scheduled = await api.schedule(1, "2099-12-01T00:00:00Z");
    expect(scheduled).toMatchObject({ status: "scheduled", scheduledAt: "2099-12-01T00:00:00Z" });
    expect((await api.unschedule(1)).scheduledAt).toBeNull();
    expect(await api.patchTag(1, { name: "催し" })).toMatchObject({ id: 1, name: "催し" });
    expect(requests.map((r) => [r.method, r.path, r.body])).toEqual([
      ["POST", "/api/contents/1/schedule", { publishAt: "2099-12-01T00:00:00Z" }],
      ["DELETE", "/api/contents/1/schedule", undefined],
      ["PATCH", "/api/tags/1", { name: "催し" }],
    ]);
  });

  it("creates, lists and deletes preview links; the URL comes only on create", async () => {
    const requests = recordRequests();
    const link = await api.createPreviewLink(1, 7);
    expect(link.url).toMatch(/\/p\/[^/]+\/$/);
    const days = (Date.parse(link.expiresAt) - Date.parse(link.createdAt)) / 86_400_000;
    expect(days).toBe(7);
    const listed = await api.listPreviewLinks(1);
    expect(listed).toEqual([{ id: link.id, expiresAt: link.expiresAt, createdAt: link.createdAt }]);
    await api.deletePreviewLink(link.id);
    expect(await api.listPreviewLinks(1)).toEqual([]);
    expect(await api.deletePreviewLink(link.id).catch((e) => e)).toMatchObject({ status: 404 });
    const invalid = await api.createPreviewLink(1, 31).catch((e) => e);
    expect(invalid).toMatchObject({ status: 422, code: "invalid_days" });
    expect(errorField(invalid)).toBe("days");
    expect(await api.createPreviewLink(999, 3).catch((e) => e)).toMatchObject({ status: 404 });
    expect(requests.slice(0, 3).map((r) => [r.method, r.path, r.body])).toEqual([
      ["POST", "/api/contents/1/preview-links", { days: 7 }],
      ["GET", "/api/contents/1/preview-links", undefined],
      ["DELETE", "/api/preview-links/100", undefined],
    ]);
  });

  it("reads and replaces the site settings", async () => {
    const requests = recordRequests();
    const site = await api.getSite();
    expect(site).toMatchObject({ name: "山田工務店", logo: null, menu: [] });
    const { logo: _, themes: __, publicBaseUrl: ___, ...input } = site;
    const saved = await api.saveSite({
      ...input,
      primaryColor: "#0a4f8f",
      menu: [{ label: null, contentId: 4, url: null }],
    });
    expect(saved).toMatchObject({ primaryColor: "#0a4f8f", menu: [{ contentId: 4 }] });
    expect(requests.map((r) => [r.method, r.path, r.headers.get("X-Pagewright")])).toEqual([
      ["GET", "/api/site", null],
      ["PUT", "/api/site", "1"],
    ]);
    expect(requests[1].body).not.toHaveProperty("logo");
  });

  it("returns setting errors with the field, and 403 for editors", async () => {
    const site = await api.getSite();
    const { logo: _, themes: __, publicBaseUrl: ___, ...input } = site;
    const err = await api.saveSite({ ...input, footerColor: "#7fb3ff" }).catch((e) => e);
    expect(err).toMatchObject({ status: 422, code: "invalid_setting", field: "footerColor" });
    expect(err.message).toMatch(/コントラスト比 2\.\d:1/);
    const menuErr = await api
      .saveSite({ ...input, menu: [{ label: null, contentId: null, url: "/a/" }] })
      .catch((e) => e);
    expect(errorField(menuErr)).toBe("menu");

    await api.selectSite("22222222-2222-4222-8222-222222222222");
    expect(await api.saveSite(input).catch((e) => e)).toMatchObject({
      status: 403,
      code: "forbidden",
    });
    expect(await api.listSubmissions().catch((e) => e)).toMatchObject({ status: 403 });
  });

  it("lists and deletes contact messages", async () => {
    const list = await api.listSubmissions();
    expect(list.map((s) => s.id)).toEqual([2, 1]);
    const requests = recordRequests();
    await api.deleteSubmission(2);
    expect(requests.map((r) => [r.method, r.path])).toEqual([
      ["DELETE", "/api/forms/submissions/2"],
    ]);
    expect((await api.listSubmissions()).map((s) => s.id)).toEqual([1]);
  });

  it("pages contact messages with ?before", async () => {
    const requests = recordRequests();
    expect((await api.listSubmissions(2)).map((s) => s.id)).toEqual([1]);
    expect(requests.map((r) => r.path)).toEqual(["/api/forms/submissions?before=2"]);
  });

  it("sends media as multipart with the alt text", async () => {
    const fetchSpy = vi.spyOn(globalThis, "fetch");
    const file = new File(["x"], "photo.png", { type: "image/png" });
    const media = await api.uploadMedia(file, "会社の外観");
    expect(media).toMatchObject({ altText: "会社の外観", filename: "photo.png" });
    const init = fetchSpy.mock.calls[0][1];
    expect(init?.method).toBe("POST");
    const headers = new Headers(init?.headers);
    expect(headers.get("X-Pagewright")).toBe("1");
    // No explicit Content-Type: the browser sets the multipart boundary itself.
    expect(headers.has("Content-Type")).toBe(false);
    const body = init?.body as FormData;
    expect(body.get("altText")).toBe("会社の外観");
    expect((body.get("file") as File).name).toBe("photo.png");
    fetchSpy.mockRestore();
  });
});
