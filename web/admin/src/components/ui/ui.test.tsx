import { Button, MantineProvider } from "@mantine/core";
import { ModalsProvider } from "@mantine/modals";
import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { type ReactNode, useState } from "react";
import { describe, expect, it } from "vitest";
import { expectNoAxeViolations } from "../../test/utils";
import { theme } from "../../theme";
import { confirm } from "./confirm";
import { Dialog } from "./Dialog";
import { ResponsiveList } from "./ResponsiveList";
import { contentStatuses, StatusBadge } from "./StatusBadge";

const wrap = (ui: ReactNode) =>
  render(
    <MantineProvider theme={theme} env="test">
      <ModalsProvider>
        <main>{ui}</main>
      </ModalsProvider>
    </MantineProvider>,
  );

const items = [
  { id: 1, name: "お知らせ", slug: "news" },
  { id: 2, name: "施工事例", slug: "works" },
];

const list = (
  <ResponsiveList
    items={items}
    getKey={(c) => c.id}
    caption="カテゴリの一覧"
    columns={[
      { key: "name", header: "名前", render: (c) => c.name },
      { key: "slug", header: "URL（英数字）", render: (c) => c.slug },
    ]}
    actions={(c) => <Button aria-label={`「${c.name}」を編集`}>編集</Button>}
  />
);

describe("ResponsiveList", () => {
  it("is a table with a caption on desktop", async () => {
    wrap(list);
    const table = screen.getByRole("table", { name: "カテゴリの一覧" });
    const rows = within(table).getAllByRole("row");
    expect(rows).toHaveLength(3);
    expect(within(rows[1]).getAllByRole("cell")[1]).toHaveTextContent("news");
    expect(within(rows[0]).getByRole("columnheader", { name: "操作" })).toBeInTheDocument();
    await expectNoAxeViolations();
  });

  it("is a list of cards with labelled values on phones", async () => {
    window.innerWidth = 390;
    wrap(list);
    expect(screen.queryByRole("table")).not.toBeInTheDocument();
    const cards = within(screen.getByRole("list", { name: "カテゴリの一覧" })).getAllByRole(
      "listitem",
    );
    expect(cards).toHaveLength(2);
    expect(within(cards[1]).getByText("URL（英数字）")).toBeInTheDocument();
    expect(within(cards[1]).getByText("works")).toBeInTheDocument();
    expect(within(cards[1]).getByRole("button", { name: "「施工事例」を編集" })).toBeVisible();
    await expectNoAxeViolations();
  });
});

describe("StatusBadge", () => {
  it("always shows the status as text", () => {
    wrap(<StatusBadge status="unpublishedChanges" />);
    expect(screen.getByText("未公開の変更あり")).toBeInTheDocument();
  });

  it("orders the badges of a content the same way everywhere", () => {
    const published = { status: "published" as const, hasUnpublishedChanges: true };
    expect(contentStatuses({ ...published, scheduledAt: null })).toEqual([
      "published",
      "unpublishedChanges",
    ]);
    // In a list, a scheduled update hides "unpublished changes" (they may be scheduled).
    expect(contentStatuses({ ...published, scheduledAt: "2099-01-01T00:00:00Z" })).toEqual([
      "published",
      "scheduledUpdate",
    ]);
    // The editor knows whether the latest revision is the scheduled one.
    expect(
      contentStatuses({
        ...published,
        scheduledAt: "2099-01-01T00:00:00Z",
        scheduledRevisionId: 1,
        revisionId: 2,
      }),
    ).toEqual(["published", "scheduledUpdate", "unpublishedChanges"]);
  });
});

function ConfirmDemo({ inDialog }: { inDialog?: boolean }) {
  const [result, setResult] = useState("");
  const [open, setOpen] = useState(!!inDialog);
  const ask = async () =>
    setResult(
      String(
        await confirm({
          title: "タグを削除しますか？",
          message: "「イベント」を削除します。",
          confirmLabel: "削除する",
          danger: true,
        }),
      ),
    );
  const button = <Button onClick={ask}>削除</Button>;
  return (
    <>
      <p>結果：{result}</p>
      {inDialog ? (
        <Dialog open={open} onOpenChange={setOpen} title="タグを編集">
          {button}
        </Dialog>
      ) : (
        button
      )}
    </>
  );
}

describe("confirm", () => {
  it("is an alertdialog that starts on cancel and resolves to the choice", async () => {
    const user = userEvent.setup();
    wrap(<ConfirmDemo />);
    await user.click(screen.getByRole("button", { name: "削除" }));
    const dialog = await screen.findByRole("alertdialog", { name: "タグを削除しますか？" });
    expect(dialog).toHaveAccessibleDescription("「イベント」を削除します。");
    await waitFor(() =>
      expect(within(dialog).getByRole("button", { name: "キャンセル" })).toHaveFocus(),
    );
    await expectNoAxeViolations();
    await user.click(within(dialog).getByRole("button", { name: "削除する" }));
    expect(await screen.findByText("結果：true")).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "削除" }));
    await screen.findByRole("alertdialog");
    await user.keyboard("{Escape}");
    expect(await screen.findByText("結果：false")).toBeInTheDocument();
    expect(screen.queryByRole("alertdialog")).not.toBeInTheDocument();
  });

  it("closes only itself on Escape when opened from a dialog", async () => {
    const user = userEvent.setup();
    wrap(<ConfirmDemo inDialog />);
    const outer = await screen.findByRole("dialog", { name: "タグを編集" });
    await user.click(within(outer).getByRole("button", { name: "削除" }));
    await screen.findByRole("alertdialog");
    await user.keyboard("{Escape}");
    expect(await screen.findByText("結果：false")).toBeInTheDocument();
    const outerAgain = screen.getByRole("dialog", { name: "タグを編集" });
    await waitFor(() =>
      expect(within(outerAgain).getByRole("button", { name: "削除" })).toHaveFocus(),
    );
    await user.keyboard("{Escape}");
    await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());
  });
});
