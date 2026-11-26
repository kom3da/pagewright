import { screen, waitFor, within } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { db } from "../mocks/handlers";
import { expectNoAxeViolations, failWith, recordRequests, renderApp } from "../test/utils";

describe("content list", () => {
  it("shows status badges, unpublished changes and the public URL", async () => {
    renderApp("/posts");
    const table = await screen.findByRole("table");
    const rows = within(table).getAllByRole("row").slice(1);
    expect(rows.map((r) => within(r).getAllByRole("cell")[0].textContent)).toEqual([
      "年末年始の営業について",
      "新しい施工事例を追加しました",
      "ホームページを公開しました",
    ]);
    expect(within(rows[0]).getByText("公開中")).toBeInTheDocument();
    expect(within(rows[0]).getByText("未公開の変更あり")).toBeInTheDocument();
    expect(within(rows[1]).getByText("予約")).toBeInTheDocument();
    expect(within(rows[2]).getByText("下書き")).toBeInTheDocument();
    expect(within(rows[0]).getByRole("link", { name: /\/posts\/new-year\// })).toHaveAttribute(
      "href",
      "http://localhost:8081/posts/new-year/",
    );
    await expectNoAxeViolations();
  });

  it("creates a new page and opens the editor", async () => {
    const { user } = renderApp("/pages");
    await user.click(await screen.findByRole("button", { name: "新規作成" }));
    expect(await screen.findByRole("heading", { name: "固定ページの編集" })).toBeInTheDocument();
    expect(db.contents.some((c) => c.type === "page" && c.title === "")).toBe(true);
  });

  it("shows API errors", async () => {
    failWith("get", "/api/contents", 500, "internal", "サーバーでエラーが起きました。");
    renderApp("/posts");
    // 5xx errors are retried twice with backoff before being shown.
    expect(
      await screen.findByText("サーバーでエラーが起きました。", {}, { timeout: 5000 }),
    ).toHaveAttribute("role", "alert");
  });
});

const openEditor = async (path = "/posts/1") => {
  const r = renderApp(path);
  await screen.findByRole("textbox", { name: "本文" });
  return r;
};

describe("editor", () => {
  it("saves the draft with title and body JSON", async () => {
    const requests = recordRequests();
    const { user } = await openEditor();
    const title = screen.getByLabelText("タイトル");
    await user.clear(title);
    await user.type(title, "新しいタイトル");
    expect(screen.getByText("保存していない変更があります")).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "下書き保存" }));
    expect(await screen.findByText("下書きを保存しました。")).toBeInTheDocument();
    const put = requests.find((r) => r.method === "PUT");
    expect(put?.path).toBe("/api/contents/1/draft");
    expect(put?.headers.get("X-Pagewright")).toBe("1");
    expect(put?.body).toEqual({
      title: "新しいタイトル",
      body: {
        type: "doc",
        content: [
          {
            type: "paragraph",
            content: [{ type: "text", text: "ホームページを公開しましたの本文です。" }],
          },
        ],
      },
      fields: {},
    });
    expect(screen.queryByText("保存していない変更があります")).not.toBeInTheDocument();
  });

  it("applies bold from the toolbar and saves it", async () => {
    const requests = recordRequests();
    const { user } = await openEditor();
    const body = screen.getByRole("textbox", { name: "本文" });
    await user.click(body);
    await user.keyboard("{Control>}a{/Control}");
    await user.click(screen.getByRole("button", { name: "太字" }));
    expect(screen.getByRole("button", { name: "太字" })).toHaveAttribute("aria-pressed", "true");
    await user.click(screen.getByRole("button", { name: "下書き保存" }));
    await screen.findByText("下書きを保存しました。");
    const put = requests.find((r) => r.method === "PUT");
    expect(JSON.stringify(put?.body)).toContain('"marks":[{"type":"bold"}]');
  });

  it("guides immediately when Japanese is typed in the page URL", async () => {
    const { user } = await openEditor();
    await user.click(screen.getByText("詳細設定"));
    const slug = screen.getByLabelText("ページのURL（英数字）");
    await user.type(slug, "会社");
    expect(slug).toHaveAttribute("aria-invalid", "true");
    expect(slug).toHaveAccessibleDescription(/日本語や全角文字は使えません/);

    await user.clear(slug);
    await user.type(slug, "123");
    expect(slug).toHaveAccessibleDescription(/数字だけのURLは使えません/);

    await user.clear(slug);
    await user.type(slug, "about-us");
    expect(slug).not.toHaveAttribute("aria-invalid");
    expect(screen.getByText("/posts/about-us/")).toBeInTheDocument();
  });

  it("saves settings with PATCH and shows a URL conflict on the field", async () => {
    const requests = recordRequests();
    const { user } = await openEditor("/posts/2");
    await user.click(screen.getByText("詳細設定"));
    await user.type(screen.getByLabelText("ページのURL（英数字）"), "new-year");
    await user.click(screen.getByRole("checkbox", { name: "リフォーム" }));
    await user.click(screen.getByRole("checkbox", { name: "イベント" }));
    await user.click(screen.getByRole("button", { name: "下書き保存" }));
    const slug = screen.getByLabelText("ページのURL（英数字）");
    await waitFor(() => expect(slug).toHaveAccessibleDescription(/すでに使われています/));
    expect(slug).toHaveFocus();

    await user.clear(slug);
    await user.type(slug, "works-2026");
    await user.click(screen.getByRole("button", { name: "下書き保存" }));
    expect(await screen.findByText("下書きを保存しました。")).toBeInTheDocument();
    expect(requests.filter((r) => r.method === "PATCH").at(-1)?.body).toEqual({
      slug: "works-2026",
      parentId: null,
      categoryIds: [3],
      tagIds: [1],
    });
    expect(db.contents.find((c) => c.id === 2)?.slug).toBe("works-2026");
  });

  it("creates a tag inline and selects it", async () => {
    const { user } = await openEditor();
    await user.click(screen.getByText("詳細設定"));
    await user.type(screen.getByLabelText("タグの名前"), "採用");
    await user.type(screen.getByLabelText("タグのURL（英数字）"), "recruit");
    await user.click(screen.getByRole("button", { name: "タグを追加して選ぶ" }));
    expect(await screen.findByRole("checkbox", { name: "採用" })).toBeChecked();
  });

  it("publishes, then unpublishes after confirmation", async () => {
    const { user } = await openEditor();
    await user.click(screen.getByRole("button", { name: "公開する" }));
    expect(await screen.findByText(/公開サイトに反映されるまで/)).toBeInTheDocument();
    expect(screen.getByText("公開中")).toBeInTheDocument();
    expect(db.contents.find((c) => c.id === 1)?.status).toBe("published");

    await user.click(screen.getByRole("button", { name: "公開をやめる" }));
    const dialog = await screen.findByRole("alertdialog", { name: "公開をやめますか？" });
    await user.click(within(dialog).getByRole("button", { name: "公開をやめる" }));
    expect(await screen.findByText("公開をやめました。")).toBeInTheDocument();
    expect(db.contents.find((c) => c.id === 1)?.status).toBe("draft");
  });

  it("requires a title to publish", async () => {
    const { user } = await openEditor();
    await user.clear(screen.getByLabelText("タイトル"));
    await user.click(screen.getByRole("button", { name: "公開する" }));
    expect(screen.getByLabelText("タイトル")).toHaveAccessibleDescription(
      "公開するにはタイトルを入力してください。",
    );
    expect(screen.getByLabelText("タイトル")).toHaveFocus();
  });

  it("deletes after confirming in a dialog", async () => {
    const { user } = await openEditor();
    await user.click(screen.getByRole("button", { name: "削除" }));
    const dialog = await screen.findByRole("alertdialog", { name: "この記事を削除しますか？" });
    expect(within(dialog).getByRole("button", { name: "キャンセル" })).toHaveFocus();
    await user.click(within(dialog).getByRole("button", { name: "削除する" }));
    expect(await screen.findByRole("heading", { name: "記事の一覧" })).toBeInTheDocument();
    expect(db.contents.some((c) => c.id === 1)).toBe(false);
  });

  it("warns before leaving with unsaved changes", async () => {
    const { user } = await openEditor();
    await user.type(screen.getByLabelText("タイトル"), "追記");
    await user.click(screen.getByRole("link", { name: "カテゴリ" }));
    const dialog = await screen.findByRole("alertdialog", { name: "保存していない変更があります" });
    await user.click(within(dialog).getByRole("button", { name: "このページにとどまる" }));
    expect(screen.getByRole("heading", { name: "記事の編集" })).toBeInTheDocument();

    await user.click(screen.getByRole("link", { name: "カテゴリ" }));
    await user.click(
      within(await screen.findByRole("alertdialog")).getByRole("button", {
        name: "変更を捨てて移動する",
      }),
    );
    expect(await screen.findByRole("heading", { level: 1, name: "カテゴリ" })).toBeInTheDocument();
  });

  it("shows the parent page choice only for pages", async () => {
    await openEditor("/pages/4");
    expect(screen.getByLabelText("親ページ")).toBeInTheDocument();
    expect(screen.queryByRole("group", { name: "カテゴリ" })).not.toBeInTheDocument();
  });

  it("has no detectable accessibility violations", async () => {
    const { user } = await openEditor();
    await user.click(screen.getByText("詳細設定"));
    await screen.findByRole("checkbox", { name: "お知らせ" });
    await expectNoAxeViolations();
  });
});

describe("images", () => {
  it("requires alt text, uploads and inserts the image with mediaId", async () => {
    const requests = recordRequests();
    const { user } = await openEditor();
    await user.click(screen.getByRole("textbox", { name: "本文" }));
    await user.click(screen.getByRole("button", { name: "画像を入れる" }));
    const dialog = await screen.findByRole("dialog", { name: "画像を入れる" });

    const file = new File(["png"], "office.png", { type: "image/png" });
    await user.upload(within(dialog).getByLabelText(/画像ファイル/), file);
    await user.click(within(dialog).getByRole("button", { name: "アップロードして入れる" }));
    expect(within(dialog).getByLabelText(/画像の説明/)).toHaveAccessibleDescription(
      /代替テキスト）を入力してください/,
    );

    await user.type(within(dialog).getByLabelText(/画像の説明/), "事務所の外観");
    await user.click(within(dialog).getByRole("button", { name: "アップロードして入れる" }));
    await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());

    const img = screen.getByRole("img", { name: "事務所の外観" });
    const media = db.media[0];
    expect(img).toHaveAttribute("src", `/api/media/${media.id}/file`);

    await user.click(screen.getByRole("button", { name: "下書き保存" }));
    await screen.findByText("下書きを保存しました。");
    const put = requests.find((r) => r.method === "PUT")?.body as { body: { content: unknown[] } };
    expect(put.body.content).toContainEqual({
      type: "image",
      attrs: { src: media.url, alt: "事務所の外観", mediaId: media.id },
    });
  });

  it("picks an uploaded image from the library", async () => {
    db.media.push({
      id: 50,
      url: "/media/50/shop.jpg",
      previewUrl: "/api/media/50/file",
      filename: "shop.jpg",
      contentType: "image/jpeg",
      sizeBytes: 100,
      width: 10,
      height: 10,
      altText: "店内の様子",
      createdAt: "2026-11-01T00:00:00Z",
    });
    const { user } = await openEditor();
    await user.click(screen.getByRole("textbox", { name: "本文" }));
    await user.click(screen.getByRole("button", { name: "画像を入れる" }));
    const dialog = await screen.findByRole("dialog");
    await user.click(await within(dialog).findByRole("button", { name: "店内の様子" }));
    await user.click(within(dialog).getByRole("button", { name: "この画像を入れる" }));
    expect(await screen.findByRole("img", { name: "店内の様子" })).toHaveAttribute(
      "src",
      "/api/media/50/file",
    );
  });
});
