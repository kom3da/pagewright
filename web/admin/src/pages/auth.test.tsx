import { screen, waitFor } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { db, resetDb } from "../mocks/handlers";
import { expectNoAxeViolations, failWith, recordRequests, renderApp } from "../test/utils";

describe("login", () => {
  it("sends a login link and shows that the mail was sent", async () => {
    db.loggedIn = false;
    const requests = recordRequests();
    const { user } = renderApp("/login");
    expect(await screen.findByRole("heading", { level: 1, name: "ログイン" })).toBeInTheDocument();
    expect(document.title).toBe("ログイン | Pagewright 管理画面");

    await user.click(screen.getByRole("button", { name: "ログイン用のリンクを送る" }));
    const email = screen.getByLabelText(/メールアドレス/);
    expect(email).toHaveAccessibleDescription("メールアドレスを入力してください。");
    expect(email).toHaveAttribute("aria-invalid", "true");

    await user.type(email, "taro@example.jp");
    await user.click(screen.getByRole("button", { name: "ログイン用のリンクを送る" }));
    expect(await screen.findByRole("heading", { name: "メールを送りました" })).toBeInTheDocument();
    expect(requests.find((r) => r.path === "/api/auth/login-link")?.body).toEqual({
      email: "taro@example.jp",
    });
  });

  it("has no detectable accessibility violations", async () => {
    db.loggedIn = false;
    renderApp("/login");
    await screen.findByRole("heading", { name: "ログイン" });
    await expectNoAxeViolations();
  });
});

describe("verify", () => {
  it("does not use the token until the button is pressed, then opens the dashboard", async () => {
    db.loggedIn = false;
    const requests = recordRequests();
    const { user } = renderApp("/login/verify?token=valid-token");
    const button = await screen.findByRole("button", { name: "ログイン" });
    expect(requests.some((r) => r.path === "/api/auth/verify")).toBe(false);

    await user.click(button);
    expect(await screen.findByRole("heading", { name: /ようこそ/ })).toBeInTheDocument();
    expect(requests.find((r) => r.path === "/api/auth/verify")?.body).toEqual({
      token: "valid-token",
    });
  });

  it("shows the server message for an expired link", async () => {
    db.loggedIn = false;
    const { user } = renderApp("/login/verify?token=old");
    await user.click(await screen.findByRole("button", { name: "ログイン" }));
    expect(await screen.findByText(/期限が切れています/)).toHaveAttribute("role", "alert");
  });
});

describe("session and sites", () => {
  it("goes to the login screen on 401", async () => {
    db.loggedIn = false;
    renderApp("/posts");
    expect(await screen.findByRole("heading", { name: "ログイン" })).toBeInTheDocument();
  });

  it("goes to the login screen when an API call returns 401 later", async () => {
    failWith("get", "/api/contents", 401, "unauthenticated", "ログインしてください。");
    renderApp("/posts");
    expect(await screen.findByRole("heading", { name: "ログイン" })).toBeInTheDocument();
  });

  it("asks to choose a site when none is selected, then opens it", async () => {
    resetDb().me.currentSiteId = null;
    const { user } = renderApp("/posts");
    expect(await screen.findByRole("heading", { name: "サイトを選ぶ" })).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: /山田カフェ/ }));
    expect(await screen.findByRole("heading", { name: /ようこそ/ })).toBeInTheDocument();
    expect(db.me.currentSiteId).toBe(db.me.sites[1].id);
    await expectNoAxeViolations();
  });

  it("goes to site selection when the API answers 403 no_site", async () => {
    failWith("get", "/api/contents", 403, "no_site", "サイトを選んでください。");
    renderApp("/posts");
    expect(await screen.findByRole("heading", { name: "サイトを選ぶ" })).toBeInTheDocument();
  });

  it("switches sites from the header and logs out", async () => {
    const { user } = renderApp("/");
    await screen.findByRole("heading", { name: /ようこそ/ });
    await user.selectOptions(screen.getByLabelText("サイト"), "山田カフェ");
    await user.click(screen.getByRole("button", { name: "切り替える" }));
    await waitFor(() => expect(db.me.currentSiteId).toBe(db.me.sites[1].id));
    expect(await screen.findByText("「山田カフェ」に切り替えました。")).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "ログアウト" }));
    expect(await screen.findByRole("heading", { name: "ログイン" })).toBeInTheDocument();
    expect(db.loggedIn).toBe(false);
  });

  it("hides the site switcher when the user has one site", async () => {
    db.me.sites = [db.me.sites[0]];
    renderApp("/");
    await screen.findByRole("heading", { name: /ようこそ/ });
    expect(screen.queryByLabelText("サイト")).not.toBeInTheDocument();
  });

  it("starts a site build from the header", async () => {
    const { user } = renderApp("/");
    await user.click(await screen.findByRole("button", { name: "サイトを更新" }));
    expect(await screen.findByText(/サイトの更新を始めました/)).toBeInTheDocument();
    expect(db.builds).toBe(1);
  });
});
