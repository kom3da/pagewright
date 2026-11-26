import { screen, waitFor, within } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { db } from "../mocks/handlers";
import { expectNoAxeViolations, renderApp } from "../test/utils";

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
    await screen.findByRole("table");
    const form = screen.getByRole("region", { name: "カテゴリを追加" });
    await user.type(within(form).getByLabelText(/カテゴリの名前/), "新築");
    const slug = within(form).getByLabelText(/カテゴリのURL/);
    await user.type(slug, "しんちく");
    expect(slug).toHaveAccessibleDescription(/日本語や全角文字は使えません/);
    await user.clear(slug);
    await user.type(slug, "new-house");
    await user.selectOptions(within(form).getByLabelText("親カテゴリ"), "施工事例");
    await user.click(within(form).getByRole("button", { name: "追加する" }));
    expect(await screen.findByText("カテゴリ「新築」を追加しました。")).toBeInTheDocument();
    expect(db.categories.at(-1)).toMatchObject({ name: "新築", slug: "new-house", parentId: 2 });
  });

  it("shows a duplicate URL error from the server on the field", async () => {
    const { user } = renderApp("/categories");
    await screen.findByRole("table");
    const form = screen.getByRole("region", { name: "カテゴリを追加" });
    await user.type(within(form).getByLabelText(/カテゴリの名前/), "ニュース");
    await user.type(within(form).getByLabelText(/カテゴリのURL/), "news");
    await user.click(within(form).getByRole("button", { name: "追加する" }));
    await waitFor(() =>
      expect(within(form).getByLabelText(/カテゴリのURL/)).toHaveAccessibleDescription(
        /すでに使われています/,
      ),
    );
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

  it("deletes after confirmation", async () => {
    const { user } = renderApp("/categories");
    await user.click(await screen.findByRole("button", { name: "「お知らせ」を削除" }));
    const dialog = await screen.findByRole("alertdialog", { name: "カテゴリを削除しますか？" });
    await user.click(within(dialog).getByRole("button", { name: "削除する" }));
    expect(await screen.findByText("カテゴリ「お知らせ」を削除しました。")).toBeInTheDocument();
    expect(db.categories.some((c) => c.id === 1)).toBe(false);
  });
});

describe("tags", () => {
  it("adds and deletes tags", async () => {
    const { user } = renderApp("/tags");
    await screen.findByRole("table");
    await user.type(screen.getByLabelText(/タグの名前/), "求人");
    await user.type(screen.getByLabelText(/タグのURL/), "jobs");
    await user.click(screen.getByRole("button", { name: "追加する" }));
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
