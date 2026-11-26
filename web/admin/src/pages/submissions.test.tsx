import { screen, waitFor, within } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { db } from "../mocks/handlers";
import { expectNoAxeViolations, recordRequests, renderApp } from "../test/utils";

describe("contact messages", () => {
  it("lists messages newest first with name, date, the first line and e-mail", async () => {
    renderApp("/inquiries");
    const table = await screen.findByRole("table", { name: /お問い合わせの一覧（2件を表示）/ });
    const rows = within(table).getAllByRole("row").slice(1);
    expect(
      rows.map((r) =>
        within(r)
          .getAllByRole("cell")
          .slice(0, 4)
          .map((c) => c.textContent),
      ),
    ).toEqual([
      [
        "佐藤花子",
        "2026/11/21 12:15",
        "キッチンのリフォームについて相談したいです。",
        "hanako@example.jp",
      ],
      ["鈴木一郎", "2026/11/20 8:40", "見学会の日程を教えてください。", "ichiro@example.com"],
    ]);
    expect(screen.getByText(/個人情報が含まれます/)).toBeInTheDocument();
    await expectNoAxeViolations();
  });

  it("opens a message from its card on a phone and returns focus", async () => {
    window.innerWidth = 390;
    const { user } = renderApp("/inquiries");
    const list = await screen.findByRole("list", { name: /お問い合わせの一覧/ });
    const open = within(list).getByRole("button", {
      name: "鈴木一郎 さんからのお問い合わせを開く",
    });
    await user.click(open);
    const dialog = await screen.findByRole("dialog", { name: "お問い合わせの内容" });
    await user.keyboard("{Escape}");
    await waitFor(() => expect(dialog).not.toBeInTheDocument());
    await waitFor(() => expect(open).toHaveFocus());
    await expectNoAxeViolations();
  });

  it("shows every field and a reply link", async () => {
    const { user } = renderApp("/inquiries");
    await user.click(
      await screen.findByRole("button", { name: "佐藤花子 さんからのお問い合わせを開く" }),
    );
    const dialog = await screen.findByRole("dialog", { name: "お問い合わせの内容" });
    for (const text of ["さとうはなこ", "090-1234-5678", "/contact/", "2026/11/21 12:15"]) {
      expect(within(dialog).getByText(text)).toBeInTheDocument();
    }
    expect(within(dialog).getByText(/見積もりは無料でしょうか。/)).toBeInTheDocument();
    const reply = within(dialog).getByRole("link", { name: "メールで返信する" });
    expect(reply.getAttribute("href")).toMatch(/^mailto:hanako@example\.jp\?subject=/);
    expect(decodeURIComponent(reply.getAttribute("href") ?? "")).toContain(
      "【山田工務店】お問い合わせありがとうございます",
    );
    await expectNoAxeViolations(dialog);
  });

  it("deletes a message after confirmation", async () => {
    const requests = recordRequests();
    const { user } = renderApp("/inquiries");
    await user.click(
      await screen.findByRole("button", { name: "鈴木一郎 さんからのお問い合わせを開く" }),
    );
    const dialog = await screen.findByRole("dialog", { name: "お問い合わせの内容" });
    await user.click(within(dialog).getByRole("button", { name: "このお問い合わせを削除" }));
    const confirm = await screen.findByRole("alertdialog", {
      name: "このお問い合わせを削除しますか？",
    });
    expect(within(confirm).getByRole("button", { name: "キャンセル" })).toHaveFocus();
    await user.click(within(confirm).getByRole("button", { name: "削除する" }));
    expect(
      await screen.findByText("鈴木一郎 さんからのお問い合わせを削除しました。"),
    ).toBeInTheDocument();
    expect(
      requests.some((r) => r.method === "DELETE" && r.path === "/api/forms/submissions/1"),
    ).toBe(true);
    await waitFor(() => expect(screen.queryByText("ichiro@example.com")).toBeNull());
    expect(db.submissions.map((s) => s.id)).toEqual([2]);
  });

  it("loads older messages 100 at a time with もっと見る", async () => {
    db.submissions = Array.from({ length: 150 }, (_, i) => ({
      ...db.submissions[0],
      id: i + 1,
      name: `送信者${i + 1}`,
      createdAt: new Date(Date.UTC(2026, 9, 1) + i * 3_600_000).toISOString(),
    }));
    const requests = recordRequests();
    const { user } = renderApp("/inquiries");
    const table = await screen.findByRole("table", { name: /100件を表示/ });
    expect(within(table).getAllByRole("row")).toHaveLength(101);
    await user.click(screen.getByRole("button", { name: "もっと見る" }));
    await screen.findByRole("table", { name: /150件を表示/ });
    expect(screen.getByText("送信者1")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "もっと見る" })).toBeNull();
    expect(requests.map((r) => r.path).filter((p) => p.startsWith("/api/forms"))).toEqual([
      "/api/forms/submissions",
      "/api/forms/submissions?before=51",
    ]);
  });

  it("keeps もっと見る after deleting from a full page", async () => {
    db.submissions = Array.from({ length: 150 }, (_, i) => ({
      ...db.submissions[0],
      id: i + 1,
      name: `送信者${i + 1}`,
      createdAt: new Date(Date.UTC(2026, 9, 1) + i * 3_600_000).toISOString(),
    }));
    const { user } = renderApp("/inquiries");
    await user.click(
      await screen.findByRole("button", { name: "送信者150 さんからのお問い合わせを開く" }),
    );
    const dialog = await screen.findByRole("dialog", { name: "お問い合わせの内容" });
    await user.click(within(dialog).getByRole("button", { name: "このお問い合わせを削除" }));
    const confirm = await screen.findByRole("alertdialog", {
      name: "このお問い合わせを削除しますか？",
    });
    await user.click(within(confirm).getByRole("button", { name: "削除する" }));
    await screen.findByRole("table", { name: /99件を表示/ });
    await user.click(screen.getByRole("button", { name: "もっと見る" }));
    await screen.findByRole("table", { name: /149件を表示/ });
  });

  it("explains how to start when there are no messages", async () => {
    db.submissions = [];
    renderApp("/inquiries");
    expect(await screen.findByText(/まだお問い合わせはありません/)).toBeInTheDocument();
  });

  it("does not load messages for editors", async () => {
    db.me.currentSiteId = "22222222-2222-4222-8222-222222222222";
    const requests = recordRequests();
    renderApp("/inquiries");
    expect(await screen.findByText(/サイトの管理者だけが使えます/)).toBeInTheDocument();
    expect(requests.some((r) => r.path.startsWith("/api/forms"))).toBe(false);
  });
});
