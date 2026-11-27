import { screen, waitFor, within } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { db } from "../mocks/handlers";
import { expectNoAxeViolations, failWith, recordRequests, renderApp } from "../test/utils";

describe("categories", () => {
  it("lists categories as a tree", async () => {
    renderApp("/categories");
    const table = await screen.findByRole("table");
    const names = within(table)
      .getAllByRole("row")
      .slice(1)
      .map((r) => within(r).getAllByRole("cell")[0].textContent);
    expect(names).toEqual(["お知らせ", "施工事例", "└リフォーム"]);
    await expectNoAxeViolations();
  });

  it("adds a category with a parent and validates the URL", async () => {
    const { user } = renderApp("/categories");
    await user.click(await screen.findByRole("button", { name: "カテゴリを追加" }));
    const form = await screen.findByRole("dialog", { name: "カテゴリを追加" });
    await user.type(within(form).getByLabelText(/カテゴリの名前/), "新築");
    const slug = within(form).getByLabelText(/カテゴリのURL/);
    // An empty URL is asked for by the field's name.
    await user.click(within(form).getByRole("button", { name: "追加する" }));
    await waitFor(() =>
      expect(slug).toHaveAccessibleDescription(/カテゴリのURL（英数字）を入力してください。/),
    );
    await user.type(slug, "しんちく");
    expect(slug).toHaveAccessibleDescription(/日本語や全角文字は使えません/);
    await user.clear(slug);
    await user.type(slug, "new-house");
    await user.selectOptions(within(form).getByLabelText("親カテゴリ"), "施工事例");
    await user.click(within(form).getByRole("button", { name: "追加する" }));
    expect(await screen.findByText("カテゴリ「新築」を追加しました。")).toBeInTheDocument();
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
    expect(await screen.findByRole("cell", { name: "新築" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "カテゴリを追加" })).toHaveFocus();
    expect(db.categories.at(-1)).toMatchObject({ name: "新築", slug: "new-house", parentId: 2 });
  });

  it("shows a duplicate URL error from the server on the field", async () => {
    const { user } = renderApp("/categories");
    await user.click(await screen.findByRole("button", { name: "カテゴリを追加" }));
    const form = await screen.findByRole("dialog", { name: "カテゴリを追加" });
    await user.type(within(form).getByLabelText(/カテゴリの名前/), "ニュース");
    await user.type(within(form).getByLabelText(/カテゴリのURL/), "news");
    await user.click(within(form).getByRole("button", { name: "追加する" }));
    await waitFor(() =>
      expect(within(form).getByLabelText(/カテゴリのURL/)).toHaveAccessibleDescription(
        /すでに使われています/,
      ),
    );
  });

  it("shows a name error from the server on the name field", async () => {
    failWith(
      "post",
      "/api/categories",
      422,
      "invalid_name",
      "名前を100文字以内で入力してください",
      "name",
    );
    const { user } = renderApp("/categories");
    await user.click(await screen.findByRole("button", { name: "カテゴリを追加" }));
    const form = await screen.findByRole("dialog", { name: "カテゴリを追加" });
    await user.type(within(form).getByLabelText(/カテゴリの名前/), "長い名前");
    await user.type(within(form).getByLabelText(/カテゴリのURL/), "long");
    await user.click(within(form).getByRole("button", { name: "追加する" }));
    const name = within(form).getByLabelText(/カテゴリの名前/);
    await waitFor(() => expect(name).toHaveAccessibleDescription(/100文字以内/));
    expect(name).toHaveFocus();
  });

  it("edits a category without offering itself or its children as parent", async () => {
    const { user } = renderApp("/categories");
    await user.click(await screen.findByRole("button", { name: "「施工事例」を編集" }));
    const dialog = await screen.findByRole("dialog", { name: "カテゴリを編集" });
    const parent = within(dialog).getByLabelText("親カテゴリ");
    const options = within(parent)
      .getAllByRole("option")
      .map((o) => o.textContent);
    expect(options).toEqual(["なし（いちばん上の階層）", "お知らせ"]);
    const name = within(dialog).getByLabelText(/カテゴリの名前/);
    await user.clear(name);
    await user.type(name, "施工実績");
    await user.click(within(dialog).getByRole("button", { name: "保存する" }));
    await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());
    expect(db.categories.find((c) => c.id === 2)?.name).toBe("施工実績");
  });

  it("explains that child categories move up one level when deleting", async () => {
    const { user } = renderApp("/categories");
    await user.click(await screen.findByRole("button", { name: "「施工事例」を削除" }));
    const dialog = await screen.findByRole("alertdialog", { name: "カテゴリを削除しますか？" });
    expect(dialog).toHaveAccessibleDescription(
      /子カテゴリ（「リフォーム」）は削除されず、一つ上の階層に移ります。/,
    );
    await user.click(within(dialog).getByRole("button", { name: "削除する" }));
    await screen.findByText("カテゴリ「施工事例」を削除しました。");
    expect(db.categories.find((c) => c.id === 3)?.parentId).toBeNull();
  });

  it("deletes after confirmation", async () => {
    const { user } = renderApp("/categories");
    await user.click(await screen.findByRole("button", { name: "「お知らせ」を削除" }));
    const dialog = await screen.findByRole("alertdialog", { name: "カテゴリを削除しますか？" });
    expect(dialog).not.toHaveAccessibleDescription(/子カテゴリ/);
    await user.click(within(dialog).getByRole("button", { name: "削除する" }));
    expect(await screen.findByText("カテゴリ「お知らせ」を削除しました。")).toBeInTheDocument();
    expect(db.categories.some((c) => c.id === 1)).toBe(false);
  });
});

describe("tags", () => {
  it("guides to the add button when there are no tags", async () => {
    db.tags = [];
    const { user } = renderApp("/tags");
    expect(await screen.findByText("まだタグがありません")).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "タグを追加" }));
    const dialog = await screen.findByRole("dialog", { name: "タグを追加" });
    await user.click(within(dialog).getByRole("button", { name: "キャンセル" }));
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
    expect(db.tags).toEqual([]);
  });

  it("edits a tag's name and URL", async () => {
    const requests = recordRequests();
    const { user } = renderApp("/tags");
    await user.click(await screen.findByRole("button", { name: "「イベント」を編集" }));
    const dialog = await screen.findByRole("dialog", { name: "タグを編集" });
    expect(dialog).toHaveAccessibleDescription(/古いURLは新しいURLへ自動で転送/);
    const name = within(dialog).getByLabelText(/タグの名前/);
    const slug = within(dialog).getByLabelText(/タグのURL/);
    expect(slug).toHaveValue("event");
    await user.clear(name);
    await user.type(name, "催し");
    await user.clear(slug);
    await user.type(slug, "events");
    await user.click(within(dialog).getByRole("button", { name: "保存する" }));
    expect(await screen.findByText("タグを保存しました。")).toBeInTheDocument();
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
    expect(await screen.findByRole("cell", { name: "催し" })).toBeInTheDocument();
    expect(requests.find((r) => r.method === "PATCH")).toMatchObject({
      path: "/api/tags/1",
      body: { name: "催し", slug: "events" },
    });
  });

  it("shows a duplicate URL error when editing a tag", async () => {
    const { user } = renderApp("/tags");
    await user.click(await screen.findByRole("button", { name: "「イベント」を編集" }));
    const dialog = await screen.findByRole("dialog", { name: "タグを編集" });
    const slug = within(dialog).getByLabelText(/タグのURL/);
    await user.clear(slug);
    await user.type(slug, "campaign");
    await user.click(within(dialog).getByRole("button", { name: "保存する" }));
    await waitFor(() => expect(slug).toHaveAccessibleDescription(/すでに使われています/));
    expect(slug).toHaveFocus();
  });

  it("adds and deletes tags", async () => {
    const { user } = renderApp("/tags");
    await user.click(await screen.findByRole("button", { name: "タグを追加" }));
    const dialog = await screen.findByRole("dialog", { name: "タグを追加" });
    await user.type(within(dialog).getByLabelText(/タグの名前/), "求人");
    await user.type(within(dialog).getByLabelText(/タグのURL/), "jobs");
    await user.click(within(dialog).getByRole("button", { name: "追加する" }));
    expect(await screen.findByRole("cell", { name: "求人" })).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "「イベント」を削除" }));
    await user.click(
      within(await screen.findByRole("alertdialog")).getByRole("button", { name: "削除する" }),
    );
    await waitFor(() =>
      expect(screen.queryByRole("cell", { name: "イベント" })).not.toBeInTheDocument(),
    );
    await expectNoAxeViolations();
  });
});
