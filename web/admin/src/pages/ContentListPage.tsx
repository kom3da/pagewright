import { EmptyState, Stack, Text } from "@mantine/core";
import { useQuery } from "@tanstack/react-query";
import { FileText, Newspaper } from "lucide-react";
import { errorMessage } from "../api/client";
import { contentsQuery } from "../api/queries";
import type { ContentType } from "../api/types";
import { NewContentButton } from "../components/NewContentButton";
import { AnchorLink } from "../components/ui/AnchorLink";
import { ExternalLink } from "../components/ui/ExternalLink";
import { ErrorMessage, Loading } from "../components/ui/feedback";
import { PageHeader } from "../components/ui/PageHeader";
import { ResponsiveList } from "../components/ui/ResponsiveList";
import { ContentStatusBadges } from "../components/ui/StatusBadge";
import { publicUrl } from "../config";
import { formatDateTime, formatJapanDateTime } from "../lib/datetime";
import { type PageRow, pageRows } from "../lib/pageTree";

const labels = {
  post: {
    title: "記事",
    description: "お知らせやブログの記事です。新しい順に並びます。",
    empty: "まだ記事がありません",
    emptyText: "「新規作成」から最初の記事を書きましょう。",
    icon: Newspaper,
  },
  page: {
    title: "固定ページ",
    description: "会社概要など、いつも置いておくページです。親ページの下に子ページが並びます。",
    empty: "まだ固定ページがありません",
    emptyText: "「新規作成」から最初のページを作りましょう。",
    icon: FileText,
  },
} as const;

export function ContentListPage({ type }: { type: ContentType }) {
  const { data, isPending, error } = useQuery(contentsQuery(type));
  const label = labels[type];
  // Pages are shown as a tree (parents first); posts keep the API order.
  const rows: PageRow[] =
    type === "page"
      ? pageRows(data ?? [])
      : (data ?? []).map((c) => ({ ...c, depth: 0, ancestors: [] }));

  return (
    <>
      <PageHeader
        title={`${label.title}の一覧`}
        description={label.description}
        actions={<NewContentButton type={type} label="新規作成" primary />}
      />
      {isPending ? (
        <Loading />
      ) : error ? (
        <ErrorMessage>{errorMessage(error)}</ErrorMessage>
      ) : rows.length === 0 ? (
        <EmptyState
          icon={<label.icon aria-hidden="true" />}
          title={label.empty}
          description={label.emptyText}
        />
      ) : (
        <ResponsiveList
          items={rows}
          getKey={(c) => c.id}
          caption={`${label.title}の一覧（${type === "page" ? "親ページの下に子ページ" : "番号の新しい順"}）`}
          columns={[
            {
              key: "title",
              header: "タイトル",
              render: (c) => (
                <div style={{ paddingLeft: `${c.depth * 1.5}rem` }}>
                  {c.ancestors.length > 0 && (
                    <Text component="span" size="sm" c="dimmed" mr={4}>
                      {c.ancestors.map((a) => `${a.title || "（タイトルなし）"} › `).join("")}
                    </Text>
                  )}
                  <AnchorLink
                    to={type === "post" ? "/posts/$id" : "/pages/$id"}
                    params={{ id: String(c.id) }}
                    fw={700}
                  >
                    {c.title || "（タイトルなし）"}
                  </AnchorLink>
                </div>
              ),
            },
            {
              key: "status",
              header: "状態",
              render: (c) => (
                <Stack gap={4}>
                  <ContentStatusBadges {...c} />
                  {c.scheduledAt && (
                    <Text size="sm" style={{ whiteSpace: "nowrap" }}>
                      {formatJapanDateTime(c.scheduledAt)}に
                      {c.status === "published" ? "更新" : "公開"}
                    </Text>
                  )}
                </Stack>
              ),
            },
            {
              key: "url",
              header: "公開中のURL",
              render: (c) =>
                c.url ? (
                  <ExternalLink href={publicUrl(c.url)} size="sm">
                    {c.url}
                  </ExternalLink>
                ) : (
                  <Text size="sm" c="dimmed">
                    未公開
                  </Text>
                ),
            },
            {
              key: "updatedAt",
              header: "最終更新",
              nowrap: true,
              render: (c) => <Text size="sm">{formatDateTime(c.updatedAt)}</Text>,
            },
          ]}
        />
      )}
    </>
  );
}
