import { Button, DataList, EmptyState, Group, Paper, Stack, Text, Title } from "@mantine/core";
import {
  useInfiniteQuery,
  useMutation,
  useQueryClient,
  useSuspenseQuery,
} from "@tanstack/react-query";
import { Inbox, Mail } from "lucide-react";
import { useState } from "react";
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
import { formatDateTime } from "../lib/datetime";

const TITLE = "お問い合わせ";

const firstLine = (s: string) => {
  const line = s.split(/\r?\n/).find((l) => l.trim()) ?? "";
  return line.length > 40 ? `${line.slice(0, 40)}…` : line;
};

export function SubmissionsPage() {
  return (
    <AdminOnly title={TITLE}>
      <Submissions />
    </AdminOnly>
  );
}

function Submissions() {
  // Personal data: kept in memory only while shown (see submissionsQuery), never in storage.
  const query = useInfiniteQuery(submissionsQuery);
  const { isPending, error } = query;
  const data = query.data?.pages.flat() ?? [];
  const queryClient = useQueryClient();
  const [open, setOpen] = useState<Submission | null>(null);

  const remove = useMutation({
    mutationFn: (s: Submission) => api.deleteSubmission(s.id),
    onSuccess: (_, s) => {
      setOpen(null);
      // Not refetched: that would reload every page already shown.
      queryClient.setQueryData(
        submissionsQuery.queryKey,
        (d) => d && { ...d, pages: d.pages.map((p) => p.filter((x) => x.id !== s.id)) },
      );
      notify(`${s.name} さんからのお問い合わせを削除しました。`);
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

  return (
    <>
      <PageHeader
        title={TITLE}
        description="公開サイトのお問い合わせフォームから届いた内容です（新しい順）。届くたびに、サイトの設定で決めた送り先にもメールで知らせます。"
      />
      <Stack gap="lg">
        <Notice kind="warning">
          お名前やメールアドレスなどの個人情報が含まれます。取り扱いに注意し、対応が済んで不要になったものは削除してください。
        </Notice>
        {isPending ? (
          <Loading />
        ) : error ? (
          <ErrorMessage>{errorMessage(error)}</ErrorMessage>
        ) : data.length === 0 ? (
          <EmptyState
            icon={<Inbox aria-hidden="true" />}
            title="まだお問い合わせはありません"
            description="受け付けるには、固定ページの本文にツールバーの「お問い合わせフォームを入れる」でフォームを入れて公開してください。"
          />
        ) : (
          <ResponsiveList
            items={data}
            getKey={(s) => s.id}
            caption={`お問い合わせの一覧（${data.length}件を表示）`}
            columns={[
              {
                key: "createdAt",
                header: "受け付けた日時",
                nowrap: true,
                render: (s) => formatDateTime(s.createdAt),
              },
              { key: "name", header: "お名前", render: (s) => s.name },
              { key: "email", header: "メールアドレス", render: (s) => s.email },
              { key: "message", header: "内容", render: (s) => firstLine(s.message) },
            ]}
            actions={(s) => (
              <Button
                size="sm"
                onClick={() => setOpen(s)}
                aria-label={`${s.name} さんからのお問い合わせを開く`}
              >
                開く
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
      >
        {open && <SubmissionDetail s={open} onDelete={() => askDelete(open)} />}
      </Dialog>
    </>
  );
}

function SubmissionDetail({ s, onDelete }: { s: Submission; onDelete: () => void }) {
  const { data: me } = useSuspenseQuery(meQuery);
  const siteName = me.sites.find((x) => x.id === me.currentSiteId)?.name ?? "";
  const subject = `${siteName ? `【${siteName}】` : ""}お問い合わせありがとうございます`;
  const rows: [string, string][] = [
    ["受け付けた日時", formatDateTime(s.createdAt)],
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
        <Button variant="subtle" color="red" ml="auto" onClick={onDelete}>
          このお問い合わせを削除
        </Button>
      </Group>
      <Text size="sm" c="dimmed">
        「メールで返信する」を押すと、お使いのメールソフトが開きます。
      </Text>
    </Stack>
  );
}
