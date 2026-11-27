import { screen, waitFor, within } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { db } from "../mocks/handlers";
import { expectNoAxeViolations, recordRequests, renderApp } from "../test/utils";

describe("account: logged-in devices", () => {
  it("is opened from the account menu and lists this device first", async () => {
    const { user, history } = renderApp("/");
    await user.click(await screen.findByRole("button", { name: /アカウントのメニュー/ }));
    await user.click(await screen.findByRole("menuitem", { name: "ログイン中の端末" }));
    expect(await screen.findByRole("heading", { level: 1, name: "アカウント" })).toBeVisible();
    expect(history.location.pathname).toBe("/admin/account");
    const table = await screen.findByRole("table", { name: "ログイン中の端末（3台）" });
    const rows = within(table)
      .getAllByRole("row")
      .slice(1)
      .map((r) =>
        within(r)
          .getAllByRole("cell")
          .slice(0, 3)
          .map((c) => c.textContent),
      );
    expect(rows).toEqual([
      ["Windows の Chromeこの端末", "いま使用中", "2026/11/20 9:00"],
      ["iPhone の Safari", "2026/11/20 21:00", "2026/11/18 18:30"],
      ["名前のわからない端末", "2026/11/02 9:00", "2026/11/01 9:00"],
    ]);
    // This device has no button: it ends with ログアウト.
    expect(within(table).getAllByRole("button", { name: /ログアウトさせる/ })).toHaveLength(2);
    await expectNoAxeViolations();
  });

  it("logs one other device out after confirmation", async () => {
    const requests = recordRequests();
    const { user } = renderApp("/account");
    await user.click(
      await screen.findByRole("button", {
        name: "「iPhone の Safari」（最後に使った日時：2026/11/20 21:00）をログアウトさせる",
      }),
    );
    const dialog = await screen.findByRole("alertdialog", {
      name: "この端末をログアウトさせますか？",
    });
    expect(within(dialog).getByRole("button", { name: "キャンセル" })).toHaveFocus();
    await user.click(within(dialog).getByRole("button", { name: "ログアウトさせる" }));
    expect(
      await screen.findByText("「iPhone の Safari」をログアウトさせました。"),
    ).toBeInTheDocument();
    await screen.findByRole("table", { name: "ログイン中の端末（2台）" });
    expect(
      screen.getByRole("button", { name: /「名前のわからない端末」.*をログアウトさせる/ }),
    ).toHaveFocus();
    expect(requests.filter((r) => r.method === "DELETE").map((r) => r.path)).toEqual([
      `/api/me/sessions/${"b".repeat(64)}`,
    ]);
  });

  it("logs every other device out with ほかの端末からログアウト, keeping this one", async () => {
    const requests = recordRequests();
    const { user } = renderApp("/account");
    await user.click(await screen.findByRole("button", { name: "ほかの端末からログアウト" }));
    const dialog = await screen.findByRole("alertdialog", {
      name: "ほかの端末からログアウトしますか？",
    });
    expect(dialog).toHaveTextContent("この端末のほかの2台はログアウトします。");
    await user.click(within(dialog).getByRole("button", { name: "ログアウトする" }));
    expect(
      await screen.findByText(
        "ほかの端末（2台）からログアウトしました。この端末はログインしたままです。",
      ),
    ).toBeInTheDocument();
    await screen.findByRole("table", { name: "ログイン中の端末（1台）" });
    expect(screen.queryByRole("button", { name: "ほかの端末からログアウト" })).toBeNull();
    await waitFor(() =>
      expect(screen.getByRole("heading", { level: 2, name: "ログイン中の端末" })).toHaveFocus(),
    );
    expect(requests.filter((r) => r.method === "POST").map((r) => r.path)).toEqual([
      "/api/me/sessions/logout-others",
    ]);
    expect(db.sessions.map((s) => s.current)).toEqual([true]);
    expect(db.loggedIn).toBe(true);
  });

  it("offers no ほかの端末からログアウト when this is the only device", async () => {
    db.sessions = db.sessions.filter((s) => s.current);
    renderApp("/account");
    await screen.findByRole("table", { name: "ログイン中の端末（1台）" });
    expect(screen.queryByRole("button", { name: "ほかの端末からログアウト" })).toBeNull();
  });
});
