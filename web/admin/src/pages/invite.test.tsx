import { screen, within } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { db } from "../mocks/handlers";
import { expectNoAxeViolations, recordRequests, renderApp } from "../test/utils";

describe("invitation link", () => {
  it("joins as a new user with a name, only when 参加する is pressed", async () => {
    db.loggedIn = false;
    const requests = recordRequests();
    const { user, history } = renderApp("/invite/accept?token=invite-token");
    expect(await screen.findByRole("heading", { level: 1, name: "サイトへの招待" })).toBeVisible();
    await screen.findByRole("button", { name: "参加する" });
    expect(screen.getByRole("main")).toHaveTextContent(
      "サイト「山田工務店」の管理画面に、編集者として招待されています。",
    );
    expect(screen.getByText(/招待されたメールアドレス：jiro@example.jp/)).toBeInTheDocument();
    await expectNoAxeViolations();
    expect(requests.some((r) => r.path === "/api/invitations/accept")).toBe(false);
    // Stays here: a logged-out visitor is not sent to the login screen.
    expect(history.location.pathname).toBe("/admin/invite/accept");

    await user.type(screen.getByLabelText("あなたの名前"), "山田次郎");
    await user.click(screen.getByRole("button", { name: "参加する" }));
    expect(await screen.findByText("「山田工務店」に参加しました。")).toBeInTheDocument();
    expect(await screen.findByRole("heading", { level: 1, name: /ようこそ/ })).toBeVisible();
    expect(requests.find((r) => r.path === "/api/invitations/accept")?.body).toEqual({
      token: "invite-token",
      displayName: "山田次郎",
    });
    expect(db.me.user.displayName).toBe("山田次郎");
  });

  it("asks a logged-in user of another address to log out first", async () => {
    // Logged in as taro@example.jp; the invitation is for jiro@example.jp.
    const { user } = renderApp("/invite/accept?token=invite-token");
    expect(await screen.findByText(/いまは別のメールアドレス（taro@example.jp）/)).toBeVisible();
    expect(screen.queryByRole("button", { name: "参加する" })).toBeNull();
    await expectNoAxeViolations();

    await user.click(screen.getByRole("button", { name: "ログアウトする" }));
    expect(await screen.findByRole("button", { name: "参加する" })).toBeVisible();
    expect(db.loggedIn).toBe(false);
  });

  it("joins with the current login when it is the invited address", async () => {
    db.invitations[0].email = "taro@example.jp";
    const { user } = renderApp("/invite/accept?token=invite-token");
    await user.click(await screen.findByRole("button", { name: "参加する" }));
    expect(screen.queryByLabelText("あなたの名前")).toBeNull();
    expect(await screen.findByText("「山田工務店」に参加しました。")).toBeInTheDocument();
  });

  it("says when the invitation has expired", async () => {
    db.loggedIn = false;
    renderApp("/invite/accept?token=expired-token");
    expect(await screen.findByRole("alert")).toHaveTextContent(
      "「山田工務店」への招待は、有効期限（2026年11月8日 9:00）が切れています。",
    );
    expect(screen.queryByRole("button", { name: "参加する" })).toBeNull();
  });

  it("says when the link no longer works", async () => {
    db.loggedIn = false;
    renderApp("/invite/accept?token=nope");
    const alert = await screen.findByRole("alert");
    expect(alert).toHaveTextContent("この招待のリンクは使えません。");
    expect(screen.getByRole("link", { name: "ログイン画面へ" })).toBeVisible();
    await expectNoAxeViolations();
  });
});

describe("account: name", () => {
  it("changes the user's own name", async () => {
    const requests = recordRequests();
    const { user } = renderApp("/account");
    const input = await screen.findByLabelText("あなたの名前");
    expect(input).toHaveValue("山田太郎");
    await user.clear(input);
    await user.type(input, "山田 太郎");
    await user.click(screen.getByRole("button", { name: "名前を保存する" }));
    expect(await screen.findByText("名前を保存しました。")).toBeInTheDocument();
    expect(requests.find((r) => r.method === "PATCH")?.body).toEqual({ displayName: "山田 太郎" });
    const nav = screen.getByRole("navigation", { name: "メインメニュー" });
    expect(
      within(nav).getByRole("button", { name: /アカウントのメニュー：山田 太郎/ }),
    ).toBeVisible();
    await expectNoAxeViolations();
  });
});
