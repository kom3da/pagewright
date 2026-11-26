import { screen, waitFor, within } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { db } from "../mocks/handlers";
import { expectNoAxeViolations, failWith, recordRequests, renderApp } from "../test/utils";

const openPreviewDialog = async (path = "/posts/1") => {
  const r = renderApp(path);
  await screen.findByRole("textbox", { name: "本文" });
  await r.user.click(screen.getByRole("button", { name: "確認用のリンクを作る" }));
  const dialog = await screen.findByRole("dialog", { name: "確認用のリンク" });
  return { ...r, dialog };
};

const addLink = (contentId: number, id: number, createdAt: string, expiresAt: string) =>
  db.previewLinks.push({ id, contentId, token: `t${id}`, createdAt, expiresAt });

describe("preview links", () => {
  it("creates a 3-day link, shows its URL once and copies it", async () => {
    const requests = recordRequests();
    const { user, dialog } = await openPreviewDialog();
    expect(dialog).toHaveAccessibleDescription(/ログインしなくても.*最新の保存内容/);
    expect(within(dialog).getByRole("radio", { name: "3日" })).toBeChecked();
    expect(await within(dialog).findByText("有効なリンクはありません。")).toBeInTheDocument();
    await expectNoAxeViolations(dialog);

    await user.click(within(dialog).getByRole("button", { name: "リンクを作る" }));
    const url = await within(dialog).findByRole("textbox", { name: "確認用のリンク（URL）" });
    expect(url).toHaveValue("http://localhost:8080/p/token-100/");
    expect(url).toHaveAttribute("readonly");
    expect(url).toHaveAccessibleDescription(/今だけ表示されます/);
    await waitFor(() => expect(url).toHaveFocus());
    expect(requests.find((r) => r.method === "POST")).toMatchObject({
      path: "/api/contents/1/preview-links",
      body: { days: 3 },
    });
    // No unsaved changes: nothing is saved first.
    expect(requests.some((r) => r.method === "PUT")).toBe(false);

    const list = await within(dialog).findByRole("list");
    expect(within(list).getAllByRole("listitem")).toHaveLength(1);
    expect(within(list).getByText(/いま作ったリンク/)).toBeInTheDocument();
    await expectNoAxeViolations(dialog);

    await user.click(within(dialog).getByRole("button", { name: "コピー" }));
    expect(await within(dialog).findByText(/コピーしました。/)).toBeInTheDocument();
    expect(await navigator.clipboard.readText()).toBe("http://localhost:8080/p/token-100/");

    // Closing forgets the URL: it cannot be shown again.
    await user.click(within(dialog).getByRole("button", { name: "閉じて編集に戻る" }));
    await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());
    expect(screen.getByRole("button", { name: "確認用のリンクを作る" })).toHaveFocus();
    await user.click(screen.getByRole("button", { name: "確認用のリンクを作る" }));
    const again = await screen.findByRole("dialog", { name: "確認用のリンク" });
    expect(await within(again).findAllByRole("listitem")).toHaveLength(1);
    expect(within(again).queryByRole("textbox")).not.toBeInTheDocument();
  });

  it("sends the chosen period", async () => {
    const requests = recordRequests();
    const { user, dialog } = await openPreviewDialog();
    await user.click(within(dialog).getByRole("radio", { name: "30日" }));
    await user.click(within(dialog).getByRole("button", { name: "リンクを作る" }));
    await within(dialog).findByRole("textbox");
    expect(requests.find((r) => r.method === "POST")?.body).toEqual({ days: 30 });
  });

  it("saves unsaved changes before creating the link", async () => {
    const requests = recordRequests();
    const r = renderApp("/posts/1");
    await screen.findByRole("textbox", { name: "本文" });
    await r.user.type(screen.getByLabelText("タイトル"), "（改訂）");
    await r.user.click(screen.getByRole("button", { name: "確認用のリンクを作る" }));
    const dialog = await screen.findByRole("dialog");
    expect(within(dialog).getByText(/保存していない変更があります/)).toBeInTheDocument();
    await r.user.click(within(dialog).getByRole("button", { name: "保存してリンクを作る" }));
    await within(dialog).findByRole("textbox");

    const writes = requests.filter((x) => x.method !== "GET").map((x) => `${x.method} ${x.path}`);
    expect(writes).toEqual(["PUT /api/contents/1/draft", "POST /api/contents/1/preview-links"]);
    expect(db.contents.find((c) => c.id === 1)?.title).toBe("ホームページを公開しました（改訂）");
    expect(within(dialog).getByRole("button", { name: "リンクを作る" })).toBeInTheDocument();
  });

  it("closes and shows the conflict dialog when saving first fails", async () => {
    const r = renderApp("/posts/1");
    await screen.findByRole("textbox", { name: "本文" });
    await r.user.type(screen.getByLabelText("タイトル"), "！");
    const c = db.contents.find((x) => x.id === 1);
    if (c) c.revision.id = 999; // someone else saved meanwhile
    await r.user.click(screen.getByRole("button", { name: "確認用のリンクを作る" }));
    await r.user.click(await screen.findByRole("button", { name: "保存してリンクを作る" }));
    expect(
      await screen.findByRole("dialog", { name: "ほかの人が先に保存しました" }),
    ).toBeInTheDocument();
    expect(screen.queryByRole("dialog", { name: "確認用のリンク" })).not.toBeInTheDocument();
    expect(db.previewLinks).toEqual([]);
  });

  it("shows server errors in the dialog", async () => {
    failWith(
      "post",
      "/api/contents/1/preview-links",
      422,
      "invalid_days",
      "有効期間は1日から30日のあいだで指定してください",
      "days",
    );
    const { user, dialog } = await openPreviewDialog();
    await user.click(within(dialog).getByRole("button", { name: "リンクを作る" }));
    expect(await within(dialog).findByRole("alert")).toHaveTextContent(
      "有効期間は1日から30日のあいだで指定してください",
    );
  });

  it("asks to copy by hand when the clipboard is not available", async () => {
    const { user, dialog } = await openPreviewDialog();
    await user.click(within(dialog).getByRole("button", { name: "リンクを作る" }));
    const url = await within(dialog).findByRole("textbox");
    const spy = vi.spyOn(navigator.clipboard, "writeText").mockRejectedValue(new Error("denied"));
    await user.click(within(dialog).getByRole("button", { name: "コピー" }));
    expect(await within(dialog).findByText(/自動でコピーできませんでした/)).toBeInTheDocument();
    expect(url).toHaveFocus();
    spy.mockRestore();
  });

  it("lists active links in Japan time and revokes one after confirming", async () => {
    addLink(1, 50, "2026-11-20T01:00:00Z", "2099-11-23T01:00:00Z");
    addLink(1, 51, "2026-11-21T15:30:00Z", "2099-12-21T15:30:00Z");
    addLink(1, 49, "2026-11-01T00:00:00Z", "2026-11-02T00:00:00Z"); // expired
    addLink(2, 52, "2026-11-21T00:00:00Z", "2099-11-24T00:00:00Z"); // another post
    const requests = recordRequests();
    const { user, dialog } = await openPreviewDialog();
    const items = await within(dialog).findAllByRole("listitem");
    expect(items.map((i) => i.textContent)).toEqual([
      "2099年12月22日 0:30まで有効作った日時：2026年11月22日 0:30取り消す",
      "2099年11月23日 10:00まで有効作った日時：2026年11月20日 10:00取り消す",
    ]);
    expect(within(dialog).getByText(/URLは、あとから表示できません/)).toBeInTheDocument();
    expect(within(dialog).queryByRole("textbox")).not.toBeInTheDocument();

    const revoke = within(items[1]).getByRole("button", {
      name: "2099年11月23日 10:00まで有効なリンクを取り消す",
    });
    // Cancel keeps the link.
    await user.click(revoke);
    let confirm = await screen.findByRole("alertdialog", { name: "このリンクを取り消しますか？" });
    await user.click(within(confirm).getByRole("button", { name: "キャンセル" }));
    expect(db.previewLinks.some((l) => l.id === 50)).toBe(true);

    await user.click(revoke);
    confirm = await screen.findByRole("alertdialog", { name: "このリンクを取り消しますか？" });
    await expectNoAxeViolations(confirm);
    await user.click(within(confirm).getByRole("button", { name: "取り消す" }));
    expect(await within(dialog).findByText(/リンクを取り消しました。/)).toBeInTheDocument();
    await waitFor(() => expect(within(dialog).getAllByRole("listitem")).toHaveLength(1));
    expect(requests.some((r) => r.method === "DELETE" && r.path === "/api/preview-links/50")).toBe(
      true,
    );
    expect(db.previewLinks.some((l) => l.id === 50)).toBe(false);
    await waitFor(() =>
      expect(within(dialog).getByRole("heading", { name: "有効なリンク" })).toHaveFocus(),
    );
  });

  it("explains when the link was already gone", async () => {
    addLink(1, 50, "2026-11-20T01:00:00Z", "2099-11-23T01:00:00Z");
    const { user, dialog } = await openPreviewDialog();
    await user.click(await within(dialog).findByRole("button", { name: /取り消す$/ }));
    db.previewLinks = [];
    const confirm = await screen.findByRole("alertdialog");
    await user.click(within(confirm).getByRole("button", { name: "取り消す" }));
    expect(await within(dialog).findByText(/すでに取り消されているか/)).toBeInTheDocument();
    expect(await within(dialog).findByText("有効なリンクはありません。")).toBeInTheDocument();
  });
});
