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

  it("shows schedules: 予約 with the date, and 公開中 with 更新を予約", async () => {
    const c = db.contents.find((x) => x.id === 3);
    Object.assign(c ?? {}, { scheduledAt: "2026-12-03T01:00:00Z", scheduledRevisionId: 300 });
    renderApp("/posts");
    const rows = within(await screen.findByRole("table"))
      .getAllByRole("row")
      .slice(1);
    const status = (r: HTMLElement) => within(r).getAllByRole("cell")[1];
    expect(status(rows[0])).toHaveTextContent("公開中更新を予約2026年12月3日 10:00に更新");
    expect(within(rows[0]).queryByText("未公開の変更あり")).not.toBeInTheDocument();
    expect(status(rows[1])).toHaveTextContent("予約2026年12月1日 9:00に公開");
    expect(status(rows[2])).toHaveTextContent(/^下書き$/);
  });

  it("shows pages as a tree with their parents", async () => {
    renderApp("/pages");
    const table = await screen.findByRole("table");
    const titles = within(table)
      .getAllByRole("row")
      .slice(1)
      .map((r) => within(r).getAllByRole("cell")[0].textContent);
    expect(titles).toEqual(["お問い合わせ", "会社概要", "会社概要 › 沿革"]);
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
    expect(await screen.findByRole("alert", {}, { timeout: 5000 })).toHaveTextContent(
      "サーバーでエラーが起きました。",
    );
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
      baseRevisionId: 100,
    });
    expect(screen.queryByText("保存していない変更があります")).not.toBeInTheDocument();
  });

  it("sends the revision of the last save as baseRevisionId", async () => {
    const requests = recordRequests();
    const { user } = await openEditor();
    const title = screen.getByLabelText("タイトル");
    for (const text of ["1", "2"]) {
      await user.type(title, text);
      await user.click(screen.getByRole("button", { name: "下書き保存" }));
      await waitFor(() =>
        expect(screen.queryByText("保存していない変更があります")).not.toBeInTheDocument(),
      );
    }
    const bases = requests
      .filter((r) => r.method === "PUT")
      .map((r) => (r.body as { baseRevisionId: number }).baseRevisionId);
    expect(bases).toEqual([100, 101]);
  });

  /** Someone else saves post 1 after the editor loaded it. */
  const saveElsewhere = () => {
    const c = db.contents.find((x) => x.id === 1);
    if (!c) throw new Error("missing seed");
    c.title = "ほかの人のタイトル";
    c.revision = {
      ...c.revision,
      id: 150,
      title: "ほかの人のタイトル",
      body: {
        type: "doc",
        content: [{ type: "paragraph", content: [{ type: "text", text: "ほかの人の本文" }] }],
      },
    };
  };

  it("on an edit conflict, keeps the user's text and can overwrite with it", async () => {
    const requests = recordRequests();
    const { user } = await openEditor();
    saveElsewhere();
    const title = screen.getByLabelText("タイトル");
    await user.clear(title);
    await user.type(title, "わたしのタイトル");
    await user.click(screen.getByRole("button", { name: "下書き保存" }));

    const dialog = await screen.findByRole("dialog", { name: "ほかの人が先に保存しました" });
    expect(within(dialog).getByLabelText("あなたの編集内容")).toHaveValue(
      "わたしのタイトル\n\nホームページを公開しましたの本文です。",
    );
    expect(screen.getByLabelText("タイトル")).toHaveValue("わたしのタイトル");
    expect(within(dialog).getByText(/編集内容は捨てられ/)).toBeInTheDocument();
    await expectNoAxeViolations(dialog);

    await user.click(within(dialog).getByRole("button", { name: "自分の内容で上書きする" }));
    expect(await screen.findByText("自分の内容で上書きして保存しました。")).toBeInTheDocument();
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
    const puts = requests.filter((r) => r.method === "PUT");
    expect(puts.map((r) => (r.body as { baseRevisionId: number }).baseRevisionId)).toEqual([
      100, 150,
    ]);
    expect(db.contents.find((c) => c.id === 1)?.title).toBe("わたしのタイトル");
  });

  it("on an edit conflict, can load the latest content instead", async () => {
    const { user } = await openEditor();
    saveElsewhere();
    await user.type(screen.getByLabelText("タイトル"), "追記");
    await user.click(screen.getByRole("button", { name: "下書き保存" }));
    const dialog = await screen.findByRole("dialog", { name: "ほかの人が先に保存しました" });
    await user.click(within(dialog).getByRole("button", { name: "最新の内容を読み込む" }));
    expect(await screen.findByText("最新の内容を読み込みました。")).toBeInTheDocument();
    expect(screen.getByLabelText("タイトル")).toHaveValue("ほかの人のタイトル");
    expect(screen.getByRole("textbox", { name: "本文" })).toHaveTextContent("ほかの人の本文");
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

  it("inserts the contact form from the toolbar and saves it as a contactForm block", async () => {
    const requests = recordRequests();
    const { user } = await openEditor();
    await user.click(screen.getByRole("textbox", { name: "本文" }));
    await user.click(screen.getByRole("button", { name: "お問い合わせフォームを入れる" }));
    expect(screen.getByText(/ここにお問い合わせフォーム/)).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "下書き保存" }));
    await screen.findByText("下書きを保存しました。");
    const put = requests.find((r) => r.method === "PUT");
    expect(JSON.stringify(put?.body)).toContain('{"type":"contactForm"}');
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

  it("shows server errors at the field named by error.field", async () => {
    failWith(
      "put",
      "/api/contents/1/draft",
      422,
      "invalid_title",
      "タイトルは200文字以内で",
      "title",
    );
    const { user } = await openEditor();
    await user.type(screen.getByLabelText("タイトル"), "追記");
    await user.click(screen.getByRole("button", { name: "下書き保存" }));
    const title = screen.getByLabelText("タイトル");
    await waitFor(() => expect(title).toHaveAccessibleDescription("タイトルは200文字以内で"));
    expect(title).toHaveFocus();
    expect(screen.getAllByText("タイトルは200文字以内で")).toHaveLength(1);
  });

  it("shows a parent page error on the parent field", async () => {
    failWith(
      "patch",
      "/api/contents/6",
      422,
      "invalid_parent",
      "この親ページは選べません",
      "parentId",
    );
    const { user } = await openEditor("/pages/6");
    await screen.findByRole("option", { name: "会社概要" });
    await user.selectOptions(screen.getByLabelText("親ページ"), "会社概要");
    await user.click(screen.getByRole("button", { name: "下書き保存" }));
    const parent = screen.getByLabelText("親ページ");
    await waitFor(() => expect(parent).toHaveAccessibleDescription(/この親ページは選べません/));
    expect(parent).toHaveAttribute("aria-invalid", "true");
    expect(parent).toHaveFocus();
  });

  it("shows an inline tag error at the field from error.field", async () => {
    failWith(
      "post",
      "/api/tags",
      422,
      "invalid_name",
      "名前を100文字以内で入力してください",
      "name",
    );
    const { user } = await openEditor();
    await user.click(screen.getByText("詳細設定"));
    await user.type(screen.getByLabelText("タグの名前"), "採用");
    await user.type(screen.getByLabelText("タグのURL（英数字）"), "recruit");
    await user.click(screen.getByRole("button", { name: "タグを追加して選ぶ" }));
    const name = screen.getByLabelText("タグの名前");
    await waitFor(() => expect(name).toHaveAccessibleDescription(/100文字以内/));
    expect(name).toHaveFocus();
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

  it("does not offer the page itself or its descendants as parent", async () => {
    await openEditor("/pages/4");
    const parent = screen.getByLabelText("親ページ");
    await within(parent).findByRole("option", { name: "お問い合わせ" });
    expect(
      within(parent)
        .getAllByRole("option")
        .map((o) => o.textContent),
    ).toEqual(["なし（いちばん上の階層）", "お問い合わせ"]);
  });

  it("indents child pages in the parent choices", async () => {
    await openEditor("/pages/6");
    const parent = screen.getByLabelText("親ページ");
    await within(parent).findByRole("option", { name: "会社概要" });
    expect(
      within(parent)
        .getAllByRole("option")
        .map((o) => o.textContent),
    ).toEqual(["なし（いちばん上の階層）", "会社概要", "　沿革"]);
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

describe("scheduled publishing", () => {
  const fillSchedule = async (
    user: ReturnType<typeof renderApp>["user"],
    dialog: HTMLElement,
    date: string,
    time: string,
  ) => {
    const dateInput = within(dialog).getByLabelText("日付");
    const timeInput = within(dialog).getByLabelText("時刻");
    await user.clear(dateInput);
    await user.type(dateInput, date);
    await user.clear(timeInput);
    await user.type(timeInput, time);
  };

  it("schedules with a Japan-time date and time and shows the schedule", async () => {
    const requests = recordRequests();
    const { user } = await openEditor();
    await user.click(screen.getByRole("button", { name: "日時を指定して公開" }));
    const dialog = await screen.findByRole("dialog", { name: "日時を指定して公開" });
    expect(within(dialog).getByRole("group", { name: "公開する日時（日本時間）" })).toBeVisible();
    await expectNoAxeViolations(dialog);
    await fillSchedule(user, dialog, "2099-12-01", "09:00");
    await user.click(within(dialog).getByRole("button", { name: "この日時で予約する" }));

    expect(await screen.findByText("2099年12月1日 9:00に公開するよう予約しました。")).toBeVisible();
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
    expect(screen.getByText("予約：2099年12月1日 9:00に公開")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "予約し直す" })).toBeInTheDocument();
    expect(requests.find((r) => r.path === "/api/contents/1/schedule")?.body).toEqual({
      publishAt: "2099-12-01T00:00:00Z",
    });
    expect(db.contents.find((c) => c.id === 1)).toMatchObject({
      status: "scheduled",
      scheduledAt: "2099-12-01T00:00:00Z",
    });
  });

  it("checks the date on the client before sending", async () => {
    const { user } = await openEditor();
    await user.click(screen.getByRole("button", { name: "日時を指定して公開" }));
    const dialog = await screen.findByRole("dialog");
    await fillSchedule(user, dialog, "2020-01-01", "09:00");
    await user.click(within(dialog).getByRole("button", { name: "この日時で予約する" }));
    const date = within(dialog).getByLabelText("日付");
    expect(date).toHaveAccessibleDescription(/今から1分以上あとにしてください。$/);
    expect(date).toHaveFocus();
    expect(db.contents.find((c) => c.id === 1)?.scheduledAt).toBeNull();
  });

  it("shows invalid_publish_at from the server at the date and time fields", async () => {
    failWith(
      "post",
      "/api/contents/1/schedule",
      422,
      "invalid_publish_at",
      "公開する日時は、今から1分以上あとにしてください",
      "publishAt",
    );
    const { user } = await openEditor();
    await user.click(screen.getByRole("button", { name: "日時を指定して公開" }));
    const dialog = await screen.findByRole("dialog");
    await fillSchedule(user, dialog, "2099-12-01", "09:00");
    await user.click(within(dialog).getByRole("button", { name: "この日時で予約する" }));
    const date = within(dialog).getByLabelText("日付");
    await waitFor(() => expect(date).toHaveAttribute("aria-invalid", "true"));
    expect(date).toHaveAccessibleDescription(/1分以上あとにしてください$/);
    expect(within(dialog).getByLabelText("時刻")).toHaveAttribute("aria-invalid", "true");
    expect(date).toHaveFocus();
    expect(screen.getByRole("dialog")).toBeInTheDocument();
  });

  it("shows an existing schedule and cancels it", async () => {
    const requests = recordRequests();
    const { user } = await openEditor("/posts/2");
    expect(screen.getByText("予約：2026年12月1日 9:00に公開")).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "予約を取り消す" }));
    expect(await screen.findByText("予約を取り消しました。")).toBeInTheDocument();
    expect(screen.queryByText(/予約：/)).not.toBeInTheDocument();
    expect(screen.getByText("下書き")).toBeInTheDocument();
    expect(
      requests.some((r) => r.method === "DELETE" && r.path === "/api/contents/2/schedule"),
    ).toBe(true);
  });

  it("notes changes saved after scheduling", async () => {
    const { user } = await openEditor("/posts/2");
    await user.type(screen.getByLabelText("タイトル"), "（改訂）");
    await user.click(screen.getByRole("button", { name: "下書き保存" }));
    expect(await screen.findByText(/予約に含まれていません/)).toBeInTheDocument();
  });

  it("shows a scheduled update of a published post", async () => {
    const c = db.contents.find((x) => x.id === 3);
    Object.assign(c ?? {}, { scheduledAt: "2026-12-01T00:00:00Z", scheduledRevisionId: 300 });
    await openEditor("/posts/3");
    expect(screen.getByText("公開中")).toBeInTheDocument();
    expect(screen.getByText("更新を予約")).toBeInTheDocument();
    expect(screen.getByText("更新の予約：2026年12月1日 9:00に公開")).toBeInTheDocument();
    // The unpublished changes are the scheduled ones.
    expect(screen.queryByText("未公開の変更あり")).not.toBeInTheDocument();
  });

  it("publishing now cancels the schedule", async () => {
    const { user } = await openEditor("/posts/2");
    await user.click(screen.getByRole("button", { name: "公開する" }));
    expect(await screen.findByText(/予約は取り消しました。/)).toBeInTheDocument();
    expect(db.contents.find((c) => c.id === 2)).toMatchObject({
      status: "published",
      scheduledAt: null,
    });
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

  it("shows an upload error at the file field", async () => {
    failWith("post", "/api/media", 422, "unsupported_type", "この形式の画像は使えません", "file");
    const { user } = await openEditor();
    await user.click(screen.getByRole("textbox", { name: "本文" }));
    await user.click(screen.getByRole("button", { name: "画像を入れる" }));
    const dialog = await screen.findByRole("dialog", { name: "画像を入れる" });
    await user.upload(
      within(dialog).getByLabelText(/画像ファイル/),
      new File(["png"], "a.png", { type: "image/png" }),
    );
    await user.type(within(dialog).getByLabelText(/画像の説明/), "説明");
    await user.click(within(dialog).getByRole("button", { name: "アップロードして入れる" }));
    const file = within(dialog).getByLabelText(/画像ファイル/);
    await waitFor(() => expect(file).toHaveAccessibleDescription(/この形式の画像は使えません/));
    expect(file).toHaveFocus();
    await user.keyboard("{Escape}");
    await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());
  });

  const shopImage = {
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
  };

  const openLibrary = async (user: ReturnType<typeof renderApp>["user"]) => {
    await user.click(screen.getByRole("textbox", { name: "本文" }));
    await user.click(screen.getByRole("button", { name: "画像を入れる" }));
    const dialog = await screen.findByRole("dialog");
    await user.click(await within(dialog).findByRole("button", { name: "店内の様子" }));
    return dialog;
  };

  it("deletes an image from the library after confirmation", async () => {
    db.media.push({ ...shopImage });
    const requests = recordRequests();
    const { user } = await openEditor();
    const dialog = await openLibrary(user);
    await user.click(within(dialog).getByRole("button", { name: "この画像を削除" }));
    const confirm = await screen.findByRole("alertdialog", { name: "この画像を削除しますか？" });
    expect(within(confirm).getByRole("button", { name: "キャンセル" })).toHaveFocus();
    await user.click(within(confirm).getByRole("button", { name: "削除する" }));
    expect(await screen.findByText("画像「店内の様子」を削除しました。")).toBeInTheDocument();
    await waitFor(() =>
      expect(within(dialog).queryByRole("button", { name: "店内の様子" })).not.toBeInTheDocument(),
    );
    expect(db.media).toHaveLength(0);
    expect(requests.some((r) => r.method === "DELETE" && r.path === "/api/media/50")).toBe(true);
    await user.keyboard("{Escape}");
  });

  it("shows media_in_use when the image is used in a saved body", async () => {
    db.media.push({ ...shopImage });
    const post = db.contents.find((c) => c.id === 3);
    post?.revision.body.content?.push({
      type: "image",
      attrs: { src: shopImage.url, alt: "店内", mediaId: 50 },
    });
    const { user } = await openEditor();
    const dialog = await openLibrary(user);
    await user.click(within(dialog).getByRole("button", { name: "この画像を削除" }));
    const confirm = await screen.findByRole("alertdialog");
    await user.click(within(confirm).getByRole("button", { name: "削除する" }));
    expect(await within(dialog).findByRole("alert")).toHaveTextContent(
      "この画像は記事やページで使われているため削除できません。先に本文から外してください",
    );
    expect(db.media).toHaveLength(1);
    await user.keyboard("{Escape}");
  });

  it("does not delete an image used in the unsaved body", async () => {
    db.media.push({ ...shopImage });
    const { user } = await openEditor();
    let dialog = await openLibrary(user);
    await user.click(within(dialog).getByRole("button", { name: "この画像を入れる" }));
    await screen.findByRole("img", { name: "店内の様子" });
    dialog = await openLibrary(user);
    await user.click(within(dialog).getByRole("button", { name: "この画像を削除" }));
    expect(within(dialog).getByRole("alert")).toHaveTextContent(/編集中の本文で使われている/);
    expect(screen.queryByRole("alertdialog")).not.toBeInTheDocument();
    await user.keyboard("{Escape}");
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
