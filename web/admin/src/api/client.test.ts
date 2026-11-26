import { describe, expect, it, vi } from "vitest";
import { failWith, recordRequests } from "../test/utils";
import { ApiError, api } from "./client";

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
