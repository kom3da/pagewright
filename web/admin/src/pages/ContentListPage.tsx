import {
  Box,
  Button,
  EmptyState,
  Group,
  NativeSelect,
  Stack,
  Text,
  TextInput,
} from "@mantine/core";
import { keepPreviousData, useInfiniteQuery, useQuery } from "@tanstack/react-query";
import { useNavigate } from "@tanstack/react-router";
import { FileText, Newspaper, Search } from "lucide-react";
import { type FormEvent, useEffect, useRef, useState } from "react";
import { errorMessage } from "../api/client";
import { contentsPagesQuery, contentsQuery, siteQuery } from "../api/queries";
import type { ContentFilter, ContentStatus, ContentSummary, ContentType } from "../api/types";
import { NewContentButton } from "../components/NewContentButton";
import { AnchorLink } from "../components/ui/AnchorLink";
import { ExternalLink } from "../components/ui/ExternalLink";
import { ErrorMessage, Loading } from "../components/ui/feedback";
import { PageHeader } from "../components/ui/PageHeader";
import { ResponsiveList } from "../components/ui/ResponsiveList";
import { ContentStatusBadges } from "../components/ui/StatusBadge";
import { formatDateTime, formatJapanDateTime } from "../lib/datetime";
import { flatPageRows, type PageRow, pageRows } from "../lib/pageTree";
import { usePublicUrl } from "../lib/usePublicUrl";

/** The status filter: the API's statuses plus 「未公開の変更あり」 (published, then filtered here). */
export type ListStatus = ContentStatus | "changed";
/** Search params of the post and page lists (kept in the URL so back/forward work). */
export type ListSearch = { q?: string; status?: ListStatus };

const statusOptions: { value: ListStatus; label: string }[] = [
  { value: "draft", label: "下書き" },
  { value: "scheduled", label: "予約" },
  { value: "published", label: "公開中" },
  { value: "changed", label: "未公開の変更あり" },
];

const MAX_Q = 100;

export function validateListSearch(search: Record<string, unknown>): ListSearch {
  const q = typeof search.q === "string" ? search.q.trim().slice(0, MAX_Q) : "";
  const status = statusOptions.find((o) => o.value === search.status)?.value;
  return { ...(q && { q }), ...(status && { status }) };
}

const labels = {
  post: {
    title: "記事",
    description: "お知らせやブログの記事です。新しい順に並びます。",
    empty: "まだ記事がありません",
    emptyText: "「記事を書く」から最初の記事を書きましょう。",
    newLabel: "記事を書く",
    icon: Newspaper,
  },
  page: {
    title: "固定ページ",
    description: "会社概要など、いつも置いておくページです。親ページの下に子ページが並びます。",
    empty: "まだ固定ページがありません",
    emptyText: "「固定ページを作る」から最初のページを作りましょう。",
    newLabel: "固定ページを作る",
    icon: FileText,
  },
} as const;

export function ContentListPage({ type, search }: { type: ContentType; search: ListSearch }) {
  const publicUrl = usePublicUrl();
  const label = labels[type];
  const filtered = Boolean(search.q || search.status);
  const apiFilter: ContentFilter = {
    q: search.q,
    status: search.status,
  };
  // Pages are drawn as a tree, so they come all at once; posts come a page at a time.
  // Both keep showing the previous results while the next search loads (no flashing).
  const pageList = useQuery({
    ...contentsQuery(type, apiFilter),
    placeholderData: keepPreviousData,
    enabled: type === "page",
  });
  const postList = useInfiniteQuery({
    ...contentsPagesQuery(type, apiFilter),
    placeholderData: keepPreviousData,
    enabled: type === "post",
  });
  const paged = type === "post";
  const data = paged ? postList.data?.pages.flatMap((p) => p.items) : pageList.data;
  const { isPending, isPlaceholderData } = paged ? postList : pageList;
  // A failed 「もっと見る」 keeps the rows already shown; its message goes under the button.
  const error = paged ? (postList.isFetchNextPageError ? null : postList.error) : pageList.error;
  const more = paged && postList.hasNextPage;
  // Search results lose their parents; the whole list (usually cached) supplies them.
  const all = useQuery({ ...contentsQuery("page"), enabled: type === "page" && filtered });
  // The page chosen for the top of the home page is read there, not at its own URL.
  const frontPageId = useQuery({ ...siteQuery, enabled: type === "page" }).data?.frontPageId;

  const found = data ?? [];
  // Pages are shown as a tree (parents first) unless searching; posts keep the API order.
  const rows: PageRow[] =
    type !== "page"
      ? found.map((c) => ({ ...c, depth: 0, ancestors: [] }))
      : filtered
        ? flatPageRows(found, all.data ?? found)
        : pageRows(found);

  const statusLabel = statusOptions.find((o) => o.value === search.status)?.label;
  const conditions = [search.q && `「${search.q}」`, statusLabel].filter(Boolean).join("・");

  return (
    <>
      <PageHeader
        title={`${label.title}の一覧`}
        description={label.description}
        actions={<NewContentButton type={type} label={label.newLabel} primary />}
      />
      <Stack gap="md">
        {(filtered || rows.length > 0) && <Filters type={type} search={search} />}
        <Text role="status" size="sm" c="dimmed" aria-live="polite">
          {filtered && !isPending && !error
            ? isPlaceholderData
              ? "探しています…"
              : `${conditions}の${label.title}：${rows.length}件${more ? "（続きがあります）" : ""}`
            : ""}
        </Text>
        {isPending ? (
          <Loading />
        ) : error ? (
          <ErrorMessage>{errorMessage(error)}</ErrorMessage>
        ) : rows.length === 0 && filtered ? (
          <EmptyState
            icon={<Search aria-hidden="true" />}
            title={`条件に合う${label.title}はありません`}
            description="言葉や状態を変えて、もう一度探してください。"
          />
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
            caption={`${label.title}の一覧（${
              filtered
                ? `${conditions}で絞り込み`
                : type === "page"
                  ? "親ページの下に子ページ"
                  : "番号の新しい順"
            }）`}
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
                    {/* Only published contents have a public URL. */}
                    {c.url && (
                      <Box>
                        {c.id === frontPageId ? (
                          <ExternalLink href={publicUrl("/")} size="sm" c="dimmed">
                            トップページに表示中
                          </ExternalLink>
                        ) : (
                          <ExternalLink href={publicUrl(c.url)} size="sm" c="dimmed">
                            {c.url}
                          </ExternalLink>
                        )}
                      </Box>
                    )}
                  </div>
                ),
              },
              {
                key: "status",
                header: "状態",
                render: (c) => (
                  <Stack gap={4}>
                    <ContentStatusBadges {...c} />
                    {/* A scheduled first publication is in 公開日; an update is said here. */}
                    {c.status === "published" && c.scheduledAt && (
                      <Text size="sm" style={{ whiteSpace: "nowrap" }}>
                        {formatJapanDateTime(c.scheduledAt)}に更新
                      </Text>
                    )}
                  </Stack>
                ),
              },
              {
                key: "publishedAt",
                header: "公開日",
                nowrap: true,
                render: (c) => <Text size="sm">{publishedDate(c)}</Text>,
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
        {more && !isPending && (
          <Stack gap={4} align="flex-start">
            <Button onClick={() => postList.fetchNextPage()} loading={postList.isFetchingNextPage}>
              もっと見る
            </Button>
            <Text size="sm" c="dimmed">
              {rows.length}件を表示しています。もっと前の{label.title}を読み込みます。
            </Text>
          </Stack>
        )}
        {paged && postList.isFetchNextPageError && (
          <ErrorMessage>{errorMessage(postList.error)}</ErrorMessage>
        )}
      </Stack>
    </>
  );
}

/** 公開日 of a row: the date on the site, the planned date (予定), or ― when not published. */
function publishedDate(c: ContentSummary) {
  if (c.status === "published" && c.publishedAt) return formatDateTime(c.publishedAt);
  if (c.status === "scheduled" && c.scheduledAt) return `${formatDateTime(c.scheduledAt)}（予定）`;
  return "―";
}

/** Search box and status filter. Each change is a new history entry (back undoes it). */
function Filters({ type, search }: { type: ContentType; search: ListSearch }) {
  const navigate = useNavigate();
  const [text, setText] = useState(search.q ?? "");
  const input = useRef<HTMLInputElement>(null);
  // Follow the URL when it changes from outside (back/forward, a link from the home screen).
  useEffect(() => setText(search.q ?? ""), [search.q]);

  const go = (next: ListSearch) =>
    navigate({ to: type === "post" ? "/posts" : "/pages", search: validateListSearch(next) });

  const submit = (e: FormEvent) => {
    e.preventDefault();
    void go({ ...search, q: text });
  };

  const noun = labels[type].title;
  return (
    <Box component="form" role="search" aria-label={`${noun}を探す`} onSubmit={submit}>
      <Group align="flex-end" gap="sm" wrap="wrap">
        <TextInput
          ref={input}
          type="search"
          label="タイトルやURLで探す"
          value={text}
          maxLength={MAX_Q}
          onChange={(e) => setText(e.currentTarget.value)}
          // Do not submit while a Japanese IME is converting.
          onKeyDown={(e) => e.key === "Enter" && e.nativeEvent.isComposing && e.preventDefault()}
          leftSection={<Search aria-hidden="true" size={16} />}
          style={{ flex: "1 1 16rem" }}
          maw="28rem"
        />
        <Button type="submit">探す</Button>
        <NativeSelect
          label="状態"
          value={search.status ?? ""}
          onChange={(e) =>
            void go({ ...search, q: text, status: e.currentTarget.value as ListStatus })
          }
          style={{ flex: "0 1 14rem" }}
        >
          <option value="">すべて</option>
          {statusOptions.map((o) => (
            <option key={o.value} value={o.value}>
              {o.label}
            </option>
          ))}
        </NativeSelect>
        {(search.q || search.status) && (
          <Button
            variant="subtle"
            onClick={() => {
              void go({});
              // The button disappears; keep the focus in the filters.
              input.current?.focus();
            }}
          >
            条件をクリア
          </Button>
        )}
      </Group>
    </Box>
  );
}
