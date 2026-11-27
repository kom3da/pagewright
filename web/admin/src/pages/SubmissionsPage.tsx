import {
  Anchor,
  Button,
  DataList,
  EmptyState,
  Group,
  NativeSelect,
  Paper,
  Stack,
  Text,
  Title,
} from "@mantine/core";
import {
  type InfiniteData,
  useInfiniteQuery,
  useMutation,
  useQueryClient,
  useSuspenseQuery,
} from "@tanstack/react-query";
import { useNavigate } from "@tanstack/react-router";
import { Inbox, Mail } from "lucide-react";
import { useRef, useState } from "react";
import { api, errorMessage } from "../api/client";
import { meQuery, submissionsQuery } from "../api/queries";
import type { Submission } from "../api/types";
import { AdminOnly } from "../components/AdminOnly";
import { confirm } from "../components/ui/confirm";
import { Dialog } from "../components/ui/Dialog";
import { ErrorMessage, Loading, Notice } from "../components/ui/feedback";
import { notify } from "../components/ui/notify";
import { PageHeader } from "../components/ui/PageHeader";
import { ResponsiveList } from "../components/ui/ResponsiveList";
import { StatusBadge } from "../components/ui/StatusBadge";
import { formatDateTime } from "../lib/datetime";

const TITLE = "お問い合わせ";
const FILTER_ID = "submissions-filter";

/** Which messages are shown. 未対応 (the default) is left out of the URL. */
type Show = "unhandled" | "handled" | "all";
export type SubmissionsSearch = { show?: Exclude<Show, "unhandled"> };

export const validateSubmissionsSearch = (search: Record<string, unknown>): SubmissionsSearch =>
  search.show === "handled" || search.show === "all" ? { show: search.show } : {};

const showOptions: { value: Show; label: string }[] = [
  { value: "unhandled", label: "未対応だけ" },
  { value: "handled", label: "対応済みだけ" },
  { value: "all", label: "すべて" },
];

const handledFilter = { unhandled: false, handled: true, all: undefined } as const;

const firstLine = (s: string) => {
  const line = s.split(/\r?\n/).find((l) => l.trim()) ?? "";
  return line.length > 40 ? `${line.slice(0, 40)}…` : line;
};

const openButtonId = (id: number) => `submission-open-${id}`;
const handledLabel = (s: Submission) => (s.handledAt ? "未対応に戻す" : "対応済みにする");

type Page = { items: Submission[]; more: boolean; next?: number };

export function SubmissionsPage({ search }: { search: SubmissionsSearch }) {
  return (
    <AdminOnly title={TITLE}>
      <Submissions show={search.show ?? "unhandled"} />
    </AdminOnly>
  );
}

function Submissions({ show }: { show: Show }) {
  // Personal data: kept in memory only while shown (see submissionsQuery), never in storage.
  const query = useInfiniteQuery(submissionsQuery(handledFilter[show]));
  const { isPending, error } = query;
  const data = query.data?.pages.flatMap((p) => p.items) ?? [];
  const queryClient = useQueryClient();
  const navigate = useNavigate();
  const [open, setOpen] = useState<Submission | null>(null);
  // Where the focus goes when the dialog closes (the opener may have left the list).
  const closeFocus = useRef<number | null>(null);

  /** Applies a change to every cached list (each filter): replaced, or removed where it no longer belongs. */
  const updateLists = (id: number, next: Submission | null) => {
    for (const [key] of queryClient.getQueriesData({ queryKey: ["submissions"] })) {
      const keep = (s: Submission) =>
        key[1] === "all" || (key[1] === true) === (s.handledAt !== null);
      // Not refetched: that would reload every page already shown.
      queryClient.setQueryData<InfiniteData<Page, number | undefined>>(
        key,
        (d) =>
          d && {
            ...d,
            pages: d.pages.map((p) => ({
              ...p,
              items: p.items.flatMap((x) => (x.id !== id ? [x] : next && keep(next) ? [next] : [])),
            })),
          },
      );
    }
  };

  /** The message that takes the place of `id` once it leaves this list. */
  const neighbor = (id: number) => {
    const i = data.findIndex((s) => s.id === id);
    return (data[i + 1] ?? data[i - 1])?.id ?? null;
  };

  const focusOn = (id: number | null) =>
    document.getElementById(id === null ? FILTER_ID : openButtonId(id))?.focus();

  const remove = useMutation({
    mutationFn: (s: Submission) => api.deleteSubmission(s.id),
    onSuccess: (_, s) => {
      closeFocus.current = neighbor(s.id);
      setOpen(null);
      updateLists(s.id, null);
      notify(`${s.name} さんからのお問い合わせを削除しました。`);
    },
    onError: (e) => notify(errorMessage(e), "error"),
  });

  const toggle = useMutation({
    mutationFn: (s: Submission) => api.setSubmissionHandled(s.id, !s.handledAt),
    onSuccess: (next, s) => {
      const leaves = show !== "all";
      const target = leaves ? neighbor(s.id) : s.id;
      if (open) {
        closeFocus.current = target;
        setOpen(null);
      } else if (leaves) {
        // The row's button disappears: move to the next message (it stays in the page).
        focusOn(target);
      }
      updateLists(s.id, next);
      notify(
        next.handledAt
          ? `${s.name} さんからのお問い合わせを対応済みにしました。`
          : `${s.name} さんからのお問い合わせを未対応に戻しました。`,
      );
    },
    onError: (e) => notify(errorMessage(e), "error"),
  });

  const askDelete = async (s: Submission) => {
    const ok = await confirm({
      title: "このお問い合わせを削除しますか？",
      message: `${s.name} さん（${formatDateTime(s.createdAt)}）からのお問い合わせを削除します。元に戻せません。`,
      confirmLabel: "削除する",
      danger: true,
    });
    if (ok) remove.mutate(s);
  };

  const openMessage = (s: Submission) => {
    closeFocus.current = s.id;
    setOpen(s);
  };

  const showLabel = showOptions.find((o) => o.value === show)?.label ?? "";

  return (
    <>
      <PageHeader
        title={TITLE}
        description="公開サイトのお問い合わせフォームから届いた内容です（新しい順）。返信などが済んだら「対応済みにする」を押すと、未対応のものだけを見られます。"
      />
      <Stack gap="lg">
        <Notice kind="warning">
          お名前やメールアドレスなどの個人情報が含まれます。取り扱いに注意し、対応が済んで不要になったものは削除してください（お名前を押して開いた画面の「削除する」で削除できます）。
        </Notice>
        <NativeSelect
          id={FILTER_ID}
          label="表示"
          value={show}
          onChange={(e) => {
            const value = e.currentTarget.value as Show;
            void navigate({
              to: "/inquiries",
              search: value === "unhandled" ? {} : { show: value },
            });
          }}
          maw="14rem"
        >
          {showOptions.map((o) => (
            <option key={o.value} value={o.value}>
              {o.label}
            </option>
          ))}
        </NativeSelect>
        {isPending ? (
          <Loading />
        ) : error ? (
          <ErrorMessage>{errorMessage(error)}</ErrorMessage>
        ) : data.length === 0 ? (
          <EmptyMessages show={show} />
        ) : (
          <ResponsiveList
            items={data}
            getKey={(s) => s.id}
            caption={`お問い合わせの一覧（${showLabel}、${data.length}件を表示）`}
            columns={[
              {
                key: "name",
                header: "お名前",
                render: (s) => (
                  <Anchor
                    component="button"
                    type="button"
                    id={openButtonId(s.id)}
                    fw={700}
                    // A 44px row, like the 対応済み button next to it.
                    className="pw-hit-target"
                    mih="2.75rem"
                    ta="start"
                    onClick={() => openMessage(s)}
                    aria-label={`${s.name} さんからのお問い合わせを開く`}
                  >
                    {s.name}
                  </Anchor>
                ),
              },
              {
                key: "status",
                header: "状態",
                nowrap: true,
                render: (s) => <StatusBadge status={s.handledAt ? "handled" : "unhandled"} />,
              },
              {
                key: "createdAt",
                header: "受付日時",
                nowrap: true,
                render: (s) => formatDateTime(s.createdAt),
              },
              { key: "message", header: "内容", render: (s) => firstLine(s.message) },
            ]}
            actions={(s) => (
              <Button
                size="sm"
                onClick={() => toggle.mutate(s)}
                loading={toggle.isPending && toggle.variables?.id === s.id}
                aria-label={`${s.name} さんからのお問い合わせを${handledLabel(s)}`}
              >
                {handledLabel(s)}
              </Button>
            )}
          />
        )}
        {query.hasNextPage && (
          <Stack gap={4} align="flex-start">
            <Button onClick={() => query.fetchNextPage()} loading={query.isFetchingNextPage}>
              もっと見る
            </Button>
            <Text size="sm" c="dimmed">
              {data.length}件を表示しています。もっと前のお問い合わせを読み込みます。
            </Text>
          </Stack>
        )}
        {query.isFetchNextPageError && <ErrorMessage>{errorMessage(query.error)}</ErrorMessage>}
      </Stack>

      <Dialog
        open={open !== null}
        onOpenChange={(o) => !o && setOpen(null)}
        title="お問い合わせの内容"
        wide
        onCloseFocus={() => focusOn(closeFocus.current)}
      >
        {open && (
          <SubmissionDetail
            s={open}
            onToggle={() => toggle.mutate(open)}
            toggling={toggle.isPending}
            onDelete={() => askDelete(open)}
          />
        )}
      </Dialog>
    </>
  );
}

function EmptyMessages({ show }: { show: Show }) {
  const icon = <Inbox aria-hidden="true" />;
  if (show === "unhandled") {
    return (
      <EmptyState
        icon={icon}
        title="未対応のお問い合わせはありません"
        description="新しいお問い合わせが届くと、ここに出ます。対応済みにしたものは、「表示」で「対応済みだけ」か「すべて」を選ぶと見られます。"
      />
    );
  }
  if (show === "handled") {
    return (
      <EmptyState
        icon={icon}
        title="対応済みのお問い合わせはありません"
        description="返信などが済んだお問い合わせで「対応済みにする」を押すと、ここに移ります。"
      />
    );
  }
  return (
    <EmptyState
      icon={icon}
      title="まだお問い合わせはありません"
      description="受け付けるには、固定ページの本文にツールバーの「お問い合わせフォームを入れる」でフォームを入れて公開してください。"
    />
  );
}

function SubmissionDetail({
  s,
  onToggle,
  toggling,
  onDelete,
}: {
  s: Submission;
  onToggle: () => void;
  toggling: boolean;
  onDelete: () => void;
}) {
  const { data: me } = useSuspenseQuery(meQuery);
  const siteName = me.sites.find((x) => x.id === me.currentSiteId)?.name ?? "";
  const subject = `${siteName ? `【${siteName}】` : ""}お問い合わせありがとうございます`;
  const handled = s.handledAt
    ? `対応済み（${[formatDateTime(s.handledAt), s.handledBy].filter(Boolean).join("・")}）`
    : "未対応";
  const rows: [string, string][] = [
    ["受け付けた日時", formatDateTime(s.createdAt)],
    ["対応の状態", handled],
    ["お名前", s.name],
    ["ふりがな", s.kana || "（入力なし）"],
    ["メールアドレス", s.email],
    ["電話番号", s.phone || "（入力なし）"],
    ["送信したページ", s.pagePath],
  ];
  return (
    <Stack gap="lg">
      <DataList labelWidth="9rem" size="md" withDivider>
        {rows.map(([k, v]) => (
          <DataList.Item key={k}>
            <DataList.ItemLabel>{k}</DataList.ItemLabel>
            <DataList.ItemValue>{v}</DataList.ItemValue>
          </DataList.Item>
        ))}
      </DataList>
      <div>
        <Title order={3} mb={4}>
          お問い合わせ内容
        </Title>
        <Paper withBorder bg="gray.0" p="sm" style={{ whiteSpace: "pre-wrap" }}>
          {s.message}
        </Paper>
      </div>
      <Group gap="sm">
        <Button
          component="a"
          variant="filled"
          // Encoded so that characters such as "?" in the address cannot add mailto fields.
          href={`mailto:${encodeURIComponent(s.email).replace("%40", "@")}?subject=${encodeURIComponent(subject)}`}
          leftSection={<Mail aria-hidden="true" size={16} />}
        >
          メールで返信する
        </Button>
        <Button onClick={onToggle} loading={toggling}>
          {handledLabel(s)}
        </Button>
        <Button variant="subtle" color="red" ml="auto" onClick={onDelete}>
          このお問い合わせを削除
        </Button>
      </Group>
      <Text size="sm" c="dimmed">
        「メールで返信する」を押すと、お使いのメールソフトが開きます。返信が済んだら「対応済みにする」を押してください。
      </Text>
    </Stack>
  );
}
