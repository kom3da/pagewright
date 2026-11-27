import { screen, waitFor, within } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { db } from "../mocks/handlers";
import { expectNoAxeViolations, failWith, recordRequests, renderApp } from "../test/utils";

const rowsOf = (table: HTMLElement) =>
  within(table)
    .getAllByRole("row")
    .slice(1)
    .map((r) =>
      within(r)
        .getAllByRole("cell")
        .slice(0, 4)
        .map((c) => c.textContent),
    );

describe("members", () => {
  it("is in the menu for admins and lists members and invitations", async () => {
    const { user, history } = renderApp("/");
    const nav = await screen.findByRole("navigation", { name: "メインメニュー" });
    await user.click(within(nav).getByRole("link", { name: "メンバー" }));
    expect(await screen.findByRole("heading", { level: 1, name: "メンバー" })).toBeVisible();
    expect(history.location.pathname).toBe("/admin/members");

    const members = await screen.findByRole("table", { name: "メンバーの一覧（3人）" });
    expect(rowsOf(members)).toEqual([
      ["山田太郎あなた", "taro@example.jp", "管理者", "2026/10/01"],
      ["佐藤花子", "hanako@example.jp", "編集者", "2026/10/15"],
      ["名前なし", "staff@example.jp", "編集者", "2026/11/02"],
    ]);
    const invitations = await screen.findByRole("table", { name: "返事待ちの招待（2件）" });
    expect(rowsOf(invitations)).toEqual([
      ["jiro@example.jp", "編集者", "返事待ち", "2026/12/04 9:00"],
      ["old@example.jp", "管理者", "期限切れ", "2026/11/08 9:00"],
    ]);
    await expectNoAxeViolations();
  });

  it("is not in the menu for editors, and the screen says it is for admins", async () => {
    db.me.currentSiteId = db.me.sites[1].id; // 山田カフェ, editor
    renderApp("/members");
    expect(await screen.findByRole("heading", { level: 1, name: "メンバー" })).toBeVisible();
    expect(screen.getByRole("note")).toHaveTextContent("サイトの管理者だけが使えます");
    const nav = screen.getByRole("navigation", { name: "メインメニュー" });
    expect(within(nav).queryByRole("link", { name: "メンバー" })).toBeNull();
  });

  it("invites someone by email with a role", async () => {
    const requests = recordRequests();
    const { user } = renderApp("/members");
    await user.click(await screen.findByRole("button", { name: "メンバーを招待" }));
    const dialog = await screen.findByRole("dialog", { name: "メンバーを招待" });
    await expectNoAxeViolations();

    await user.click(within(dialog).getByRole("button", { name: "招待のメールを送る" }));
    expect(within(dialog).getByLabelText(/メールアドレス/)).toHaveAccessibleDescription(
      /メールアドレスを入力してください。/,
    );
    expect(within(dialog).getByRole("radio", { name: /編集者/ })).toBeChecked();

    await user.type(within(dialog).getByLabelText(/メールアドレス/), "Ichiro@Example.jp");
    await user.click(within(dialog).getByRole("radio", { name: /管理者/ }));
    await user.click(within(dialog).getByRole("button", { name: "招待のメールを送る" }));
    expect(
      await screen.findByText(
        "ichiro@example.jp に招待のメールを送りました。リンクは7日間使えます。",
      ),
    ).toBeInTheDocument();
    expect(screen.queryByRole("dialog")).toBeNull();
    expect(requests.find((r) => r.method === "POST")?.body).toEqual({
      email: "Ichiro@Example.jp",
      role: "admin",
    });
    await screen.findByRole("table", { name: "返事待ちの招待（3件）" });
  });

  it("shows at the field that the address is already a member", async () => {
    const { user } = renderApp("/members");
    await user.click(await screen.findByRole("button", { name: "メンバーを招待" }));
    const dialog = await screen.findByRole("dialog", { name: "メンバーを招待" });
    const email = within(dialog).getByLabelText(/メールアドレス/);
    await user.type(email, "hanako@example.jp");
    await user.click(within(dialog).getByRole("button", { name: "招待のメールを送る" }));
    await waitFor(() =>
      expect(email).toHaveAccessibleDescription(/すでにこのサイトのメンバーです/),
    );
    expect(email).toHaveFocus();
  });

  it("sends an invitation again and cancels one after confirmation", async () => {
    const { user } = renderApp("/members");
    await user.click(
      await screen.findByRole("button", { name: "old@example.jp に招待のメールをもう一度送る" }),
    );
    expect(
      await screen.findByText(
        "old@example.jp に招待のメールをもう一度送りました。前に送ったリンクは使えなくなりました。",
      ),
    ).toBeInTheDocument();
    await waitFor(() => expect(db.invitations.find((i) => i.id === 1)?.expired).toBe(false));

    await user.click(screen.getByRole("button", { name: "jiro@example.jp への招待を取り消す" }));
    const dialog = await screen.findByRole("alertdialog", { name: "招待を取り消しますか？" });
    await user.click(within(dialog).getByRole("button", { name: "取り消す" }));
    expect(
      await screen.findByText("jiro@example.jp への招待を取り消しました。"),
    ).toBeInTheDocument();
    await screen.findByRole("table", { name: "返事待ちの招待（1件）" });
    expect(screen.getByRole("heading", { level: 2, name: "返事待ちの招待" })).toHaveFocus();
  });

  it("changes a member's role", async () => {
    const { user } = renderApp("/members");
    await user.click(await screen.findByRole("button", { name: "「佐藤花子」を管理者にする" }));
    expect(await screen.findByText("「佐藤花子」を管理者にしました。")).toBeInTheDocument();
    expect(db.members[1].role).toBe("admin");
    expect(await screen.findByRole("button", { name: "「佐藤花子」を編集者にする" })).toBeVisible();
  });

  it("explains why the last admin cannot step down", async () => {
    const { user } = renderApp("/members");
    await user.click(await screen.findByRole("button", { name: "「山田太郎」を編集者にする" }));
    const dialog = await screen.findByRole("alertdialog", { name: "あなたを編集者にしますか？" });
    await user.click(within(dialog).getByRole("button", { name: "編集者になる" }));
    expect(await screen.findByRole("alert")).toHaveTextContent(
      "管理者がいなくなるため、役割を変えられません。",
    );
    expect(db.members[0].role).toBe("admin");
  });

  it("lets an admin step down when another admin remains; the screen then closes to them", async () => {
    db.members[1].role = "admin";
    const { user } = renderApp("/members");
    await user.click(await screen.findByRole("button", { name: "「山田太郎」を編集者にする" }));
    await user.click(
      within(await screen.findByRole("alertdialog")).getByRole("button", { name: "編集者になる" }),
    );
    expect(await screen.findByText("あなたを編集者にしました。")).toBeInTheDocument();
    expect(await screen.findByRole("note")).toHaveTextContent("サイトの管理者だけが使えます");
  });

  it("removes a member after confirmation, saying their work stays", async () => {
    const requests = recordRequests();
    const { user } = renderApp("/members");
    await user.click(await screen.findByRole("button", { name: "「佐藤花子」をサイトから外す" }));
    const dialog = await screen.findByRole("alertdialog", {
      name: "「佐藤花子」をこのサイトから外しますか？",
    });
    expect(dialog).toHaveTextContent(
      "この人が書いた記事・固定ページや、入れた画像はそのまま残ります。",
    );
    expect(within(dialog).getByRole("button", { name: "キャンセル" })).toHaveFocus();
    await user.click(within(dialog).getByRole("button", { name: "外す" }));
    expect(await screen.findByText("「佐藤花子」をこのサイトから外しました。")).toBeInTheDocument();
    await screen.findByRole("table", { name: "メンバーの一覧（2人）" });
    expect(screen.getByRole("heading", { level: 2, name: "メンバーの一覧" })).toHaveFocus();
    expect(requests.filter((r) => r.method === "DELETE").map((r) => r.path)).toEqual([
      "/api/members/u2",
    ]);
  });

  it("shows the server's message when removing fails", async () => {
    failWith(
      "delete",
      "/api/members/:id",
      409,
      "last_admin",
      "このサイトの管理者はこの人だけのため、外せません。",
    );
    const { user } = renderApp("/members");
    await user.click(await screen.findByRole("button", { name: "このサイトから抜ける" }));
    await user.click(
      within(
        await screen.findByRole("alertdialog", { name: "このサイトから抜けますか？" }),
      ).getByRole("button", { name: "抜ける" }),
    );
    expect(await screen.findByRole("alert")).toHaveTextContent("この人だけのため、外せません。");
  });
});
