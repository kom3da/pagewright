import { screen, waitFor, within } from "@testing-library/react";
import { HttpResponse, http } from "msw";
import { describe, expect, it } from "vitest";
import { db } from "../mocks/handlers";
import { server } from "../test/server";
import { expectNoAxeViolations, recordRequests, renderApp } from "../test/utils";

const daysAgo = (n: number) => new Date(Date.now() - n * 24 * 60 * 60 * 1000).toISOString();
const tile = (name: string) =>
  screen.getByRole("heading", { level: 3, name }).closest("li") as HTMLElement;

describe("home dashboard", () => {
  it("counts drafts, schedules and unpublished changes with links to the filtered lists", async () => {
    const { user, history } = renderApp("/");
    await screen.findByRole("region", { name: "記事とページの状況" });
    expect(tile("下書き")).toHaveTextContent("2件");
    expect(tile("公開の予約")).toHaveTextContent("1件");
    expect(tile("公開の予約")).toHaveTextContent("次は2026/12/01 9:00");
    expect(tile("未公開の変更あり")).toHaveTextContent("1件");
    // A type with nothing to show is plain text, not a link.
    expect(within(tile("公開の予約")).getByText("固定ページ 0件")).not.toHaveAttribute("href");
    await expectNoAxeViolations();

    await user.click(screen.getByRole("link", { name: "下書きの固定ページ 1件を見る" }));
    expect(
      await screen.findByRole("heading", { level: 1, name: "固定ページの一覧" }),
    ).toBeVisible();
    expect(history.location.search).toBe("?status=draft");
    expect(screen.getByLabelText("状態")).toHaveValue("draft");
    await waitFor(() => expect(screen.getAllByRole("row")).toHaveLength(2));
  });

  it("takes the numbers from the server's counts, not from the lists (big sites)", async () => {
    const requests = recordRequests();
    server.use(
      http.get("/api/contents/counts", () =>
        HttpResponse.json({
          post: { draft: 1500, scheduled: 0, published: 3000, changed: 12, nextScheduledAt: null },
          page: {
            draft: 2,
            scheduled: 1,
            published: 40,
            changed: 0,
            nextScheduledAt: "2026-12-24T00:00:00Z",
          },
        }),
      ),
    );
    renderApp("/");
    await screen.findByRole("region", { name: "記事とページの状況" });
    expect(tile("下書き")).toHaveTextContent("1502件");
    expect(
      within(tile("下書き")).getByRole("link", { name: "下書きの記事 1500件を見る" }),
    ).toBeVisible();
    expect(tile("公開の予約")).toHaveTextContent("次は2026/12/24 9:00");
    expect(tile("未公開の変更あり")).toHaveTextContent("12件");
    await screen.findByRole("table", { name: /最近編集した記事・ページ/ });
    // Only short lists: the 5 last edited of each type.
    const lists = requests.filter((r) => r.method === "GET" && r.path.startsWith("/api/contents?"));
    expect(lists.map((r) => r.path).sort()).toEqual([
      "/api/contents?type=page&sort=updated&limit=5",
      "/api/contents?type=post&sort=updated&limit=5",
    ]);
  });

  it("lists recently edited posts and pages, newest first", async () => {
    for (const c of db.contents) if (c.id === 4) c.updatedAt = daysAgo(0);
    renderApp("/");
    const table = await screen.findByRole("table", { name: /最近編集した記事・ページ/ });
    const rows = within(table).getAllByRole("row").slice(1);
    expect(rows).toHaveLength(5);
    expect(within(rows[0]).getByRole("link", { name: "会社概要" })).toHaveAttribute(
      "href",
      "/admin/pages/4",
    );
    expect(rows[0]).toHaveTextContent("固定ページ");
  });

  it("counts the inquiries of the last 7 days for admins", async () => {
    db.submissions[0].createdAt = daysAgo(1);
    db.submissions[1].createdAt = daysAgo(10);
    renderApp("/");
    await waitFor(() => expect(tile("お問い合わせ")).toHaveTextContent("1件"));
    expect(
      within(tile("お問い合わせ")).getByRole("link", { name: "お問い合わせを見る" }),
    ).toBeVisible();
  });

  it("shows no inquiries to editors", async () => {
    db.me.currentSiteId = "22222222-2222-4222-8222-222222222222";
    const requests = recordRequests();
    renderApp("/");
    await screen.findByRole("region", { name: "記事とページの状況" });
    expect(screen.queryByRole("heading", { name: "お問い合わせ" })).not.toBeInTheDocument();
    expect(requests.some((r) => r.path.startsWith("/api/forms"))).toBe(false);
  });

  it("shows when the public site was last updated", async () => {
    renderApp("/");
    const section = await screen.findByRole("region", { name: "公開サイトの更新" });
    expect(await within(section).findByText("前回の更新はうまくいきました")).toBeVisible();
    expect(section).toHaveTextContent("2026年11月20日 10:05");
  });

  it("explains a failed update and its warnings in plain words", async () => {
    db.buildLog = [
      {
        id: 2,
        startedAt: "2026-11-21T00:00:00Z",
        finishedAt: "2026-11-21T00:00:05Z",
        files: 0,
        changed: 0,
        removed: 0,
        warnings: ["記事「年末年始の営業について」の画像が見つかりません"],
        error: "テーマを読み込めませんでした",
      },
    ];
    renderApp("/");
    const section = await screen.findByRole("region", { name: "公開サイトの更新" });
    expect(await within(section).findByText("前回の更新がうまくいきませんでした")).toBeVisible();
    expect(section).toHaveTextContent("「サイトを更新」をもう一度押してください");
    expect(section).toHaveTextContent("くわしい内容：テーマを読み込めませんでした");
    expect(within(section).getByText("気をつけること（1件）")).toBeVisible();
    expect(within(section).getByRole("listitem")).toHaveTextContent("画像が見つかりません");
    await expectNoAxeViolations();
  });

  it("rebuilds the site from the update card and shows the new result", async () => {
    const { user } = renderApp("/");
    const section = await screen.findByRole("region", { name: "公開サイトの更新" });
    expect(await within(section).findByText(/2026年11月20日 10:05/)).toBeVisible();
    // Not in the sidebar any more.
    const nav = screen.getByRole("navigation", { name: "メインメニュー" });
    expect(within(nav).queryByRole("button", { name: "サイトを更新" })).toBeNull();

    await user.click(within(section).getByRole("button", { name: "サイトを更新" }));
    expect(await screen.findByText(/サイトの更新を始めました/)).toBeInTheDocument();
    expect(db.builds).toBe(1);
    await waitFor(() =>
      expect(within(section).queryByText(/2026年11月20日 10:05/)).not.toBeInTheDocument(),
    );
    expect(within(section).getByText("前回の更新はうまくいきました")).toBeVisible();
  });

  it("links to the public site the server names (publicBaseUrl)", async () => {
    db.site.publicBaseUrl = "https://www.example.jp";
    renderApp("/");
    const section = await screen.findByRole("region", { name: "公開サイトの更新" });
    await waitFor(() =>
      expect(within(section).getByRole("link", { name: /公開サイトを見る/ })).toHaveAttribute(
        "href",
        "https://www.example.jp/",
      ),
    );
    const nav = screen.getByRole("navigation", { name: "メインメニュー" });
    expect(within(nav).getByRole("link", { name: /公開サイトを見る/ })).toHaveAttribute(
      "href",
      "https://www.example.jp/",
    );
  });

  it("guides the first steps on a brand-new site", async () => {
    db.contents = [];
    db.buildLog = [];
    renderApp("/");
    const steps = await screen.findByRole("region", { name: "はじめに" });
    expect(
      within(steps)
        .getAllByRole("listitem")
        .map((li) => li.querySelector("p")?.textContent),
    ).toEqual([
      "サイトの名前・ロゴ・色を決める",
      "会社概要などの固定ページを作る",
      "最初の記事を書く",
    ]);
    expect(within(steps).getByRole("link", { name: "サイトの設定" })).toHaveAttribute(
      "href",
      "/admin/settings",
    );
    expect(within(steps).queryByText("済み")).not.toBeInTheDocument();
    expect(screen.queryByRole("region", { name: "記事とページの状況" })).not.toBeInTheDocument();
    expect(await screen.findByText(/公開サイトはまだできていません/)).toBeVisible();
    // One primary action: 記事を書く.
    expect(screen.getByRole("button", { name: "記事を書く" })).toBeVisible();
    await expectNoAxeViolations();
  });

  it("marks the steps already done until something is published", async () => {
    db.contents = db.contents.filter((c) => c.status !== "published");
    renderApp("/");
    const steps = await screen.findByRole("region", { name: "はじめに" });
    expect(within(steps).getAllByText("済み")).toHaveLength(2);
  });
});
