import {
  Alert,
  Badge,
  Box,
  Button,
  Grid,
  Group,
  List,
  Paper,
  SimpleGrid,
  Stack,
  Text,
  ThemeIcon,
  Title,
} from "@mantine/core";
import {
  useInfiniteQuery,
  useMutation,
  useQuery,
  useQueryClient,
  useSuspenseQuery,
} from "@tanstack/react-query";
import { CircleAlert, CircleCheck, CircleDashed, RefreshCw, TriangleAlert } from "lucide-react";
import { type ReactNode, useState } from "react";
import { api, errorMessage } from "../api/client";
import { buildsQuery, contentsQuery, meQuery, submissionsQuery } from "../api/queries";
import type { ContentSummary, ContentType } from "../api/types";
import { NewContentButton } from "../components/NewContentButton";
import { AnchorLink } from "../components/ui/AnchorLink";
import { ExternalLink } from "../components/ui/ExternalLink";
import { ErrorMessage, Loading } from "../components/ui/feedback";
import { notify } from "../components/ui/notify";
import { PageHeader } from "../components/ui/PageHeader";
import { ResponsiveList } from "../components/ui/ResponsiveList";
import { Section } from "../components/ui/Section";
import { ContentStatusBadges } from "../components/ui/StatusBadge";
import { formatDateTime, formatJapanDateTime } from "../lib/datetime";
import { useIsAdmin } from "../lib/useIsAdmin";
import { usePublicUrl } from "../lib/usePublicUrl";
import type { ListStatus } from "./ContentListPage";

const RECENT = 5;
const WEEK_MS = 7 * 24 * 60 * 60 * 1000;
const typeLabel = { post: "記事", page: "固定ページ" } as const;

/** The things a site owner may want to follow up, by the list filter that shows them. */
const tiles: {
  status: ListStatus;
  title: string;
  text: string;
  match: (c: ContentSummary) => boolean;
}[] = [
  {
    status: "draft",
    title: "下書き",
    text: "まだ公開していないもの",
    match: (c) => c.status === "draft",
  },
  {
    status: "scheduled",
    title: "公開の予約",
    text: "公開する日時を決めたもの",
    match: (c) => c.status === "scheduled",
  },
  {
    status: "changed",
    title: "未公開の変更あり",
    text: "変更がまだサイトに出ていないもの",
    match: (c) => c.hasUnpublishedChanges,
  },
];

export function HomePage() {
  const { data: me } = useSuspenseQuery(meQuery);
  const isAdmin = useIsAdmin();
  const posts = useQuery(contentsQuery("post"));
  const pages = useQuery(contentsQuery("page"));
  const siteName = me.sites.find((s) => s.id === me.currentSiteId)?.name;

  const loaded = posts.data && pages.data;
  const all = [...(posts.data ?? []), ...(pages.data ?? [])];
  const published = all.some((c) => c.status === "published");

  return (
    <>
      <PageHeader
        title={me.user.displayName ? `ようこそ、${me.user.displayName} さん` : "ようこそ"}
        documentTitle="ホーム"
        description={`${siteName ? `「${siteName}」の` : ""}いまの状況と、次にやることをまとめています。`}
        actions={
          <>
            <NewContentButton type="page" label="固定ページを作る" />
            <NewContentButton type="post" label="記事を書く" primary />
          </>
        }
      />
      <Stack gap="xl">
        {posts.isPending || pages.isPending ? (
          <Loading />
        ) : !loaded ? (
          <ErrorMessage>{errorMessage(posts.error ?? pages.error)}</ErrorMessage>
        ) : (
          <>
            {!published && (
              <GettingStarted
                isAdmin={isAdmin}
                hasPage={pages.data.length > 0}
                hasPost={posts.data.length > 0}
              />
            )}
            {all.length > 0 && (
              <Section title="記事とページの状況" variant="plain">
                <SimpleGrid
                  component="ul"
                  cols={{ base: 2, lg: isAdmin ? 4 : 3 }}
                  spacing="sm"
                  m={0}
                  p={0}
                >
                  {tiles.map((t) => (
                    <StatusTile key={t.status} {...t} posts={posts.data} pages={pages.data} />
                  ))}
                  {isAdmin && <InquiriesTile />}
                </SimpleGrid>
              </Section>
            )}
          </>
        )}
        {/* The site update comes first on phones (a failure matters most), on the right on PCs. */}
        <Grid gap="xl">
          <Grid.Col
            span={{ base: 12, md: all.length > 0 ? 5 : 12, lg: all.length > 0 ? 4 : 12 }}
            order={{ base: 1, md: 2 }}
          >
            <SiteUpdate />
          </Grid.Col>
          {all.length > 0 && (
            <Grid.Col span={{ base: 12, md: 7, lg: 8 }} order={{ base: 2, md: 1 }}>
              <RecentEdits contents={all} />
            </Grid.Col>
          )}
        </Grid>
      </Stack>
    </>
  );
}

function GettingStarted({
  isAdmin,
  hasPage,
  hasPost,
}: {
  isAdmin: boolean;
  hasPage: boolean;
  hasPost: boolean;
}) {
  const steps: { title: string; text: ReactNode; done?: boolean }[] = [
    ...(isAdmin
      ? [
          {
            title: "サイトの名前・ロゴ・色を決める",
            text: (
              <>
                <AnchorLink to="/settings">サイトの設定</AnchorLink>
                で、サイトの名前やロゴ、色、メニューを決めます。
              </>
            ),
          },
        ]
      : []),
    {
      title: "会社概要などの固定ページを作る",
      text: "「固定ページを作る」から、会社概要やアクセスなど、いつも置いておくページを作ります。",
      done: hasPage,
    },
    {
      title: "最初の記事を書く",
      text: "「記事を書く」から、お知らせやブログの記事を書きます。",
      done: hasPost,
    },
  ];
  return (
    <Section
      title="はじめに"
      description="公開サイトを始めるまでの手順です。記事やページは「公開する」を押すまで、公開サイトには出ません。"
    >
      <List type="ordered" spacing="sm">
        {steps.map((s) => (
          <List.Item key={s.title}>
            <Group gap="xs">
              <Text fw={700}>{s.title}</Text>
              {s.done && <DoneBadge />}
            </Group>
            <Text size="sm" c="dimmed">
              {s.text}
            </Text>
          </List.Item>
        ))}
      </List>
    </Section>
  );
}

const DoneBadge = () => (
  <Badge color="green" leftSection={<CircleCheck aria-hidden="true" size={13} strokeWidth={2.5} />}>
    済み
  </Badge>
);

/** A count with its links at the bottom (aligned across a row of tiles). */
function Tile({
  title,
  children,
  links,
}: {
  title: string;
  children: ReactNode;
  links: ReactNode;
}) {
  return (
    <Paper component="li" withBorder p="md" style={{ listStyle: "none" }}>
      <Stack gap={4} h="100%">
        <Title order={3} size="sm" c="dimmed">
          {title}
        </Title>
        {children}
        <Group gap="xs" mt="auto" pt={4}>
          {links}
        </Group>
      </Stack>
    </Paper>
  );
}

const Count = ({ n, more }: { n: number; more?: boolean }) => (
  <Text fz={28} fw={700} lh={1.2}>
    {n}
    <Text span size="md" fw={400} ml={2}>
      件{more && "以上"}
    </Text>
  </Text>
);

function StatusTile({
  status,
  title,
  text,
  match,
  posts,
  pages,
}: (typeof tiles)[number] & { posts: ContentSummary[]; pages: ContentSummary[] }) {
  const counts = { post: posts.filter(match).length, page: pages.filter(match).length };
  const next = [...posts, ...pages]
    .filter(match)
    .map((c) => c.scheduledAt)
    .filter((d): d is string => d !== null)
    .sort()[0];
  return (
    <Tile
      title={title}
      links={(["post", "page"] as const).map((type) => (
        <TileLink key={type} type={type} status={status} title={title} n={counts[type]} />
      ))}
    >
      <Count n={counts.post + counts.page} />
      <Text size="sm" c="dimmed">
        {text}
        {status === "scheduled" && next && `。次は${formatDateTime(next)}`}
      </Text>
    </Tile>
  );
}

function TileLink({
  type,
  status,
  title,
  n,
}: {
  type: ContentType;
  status: ListStatus;
  title: string;
  n: number;
}) {
  const text = `${typeLabel[type]} ${n}件`;
  if (n === 0) {
    return (
      <Text size="sm" c="dimmed">
        {text}
      </Text>
    );
  }
  return (
    <AnchorLink
      to={type === "post" ? "/posts" : "/pages"}
      search={{ status }}
      size="sm"
      fw={700}
      aria-label={`${title}の${text}を見る`}
    >
      {text}
    </AnchorLink>
  );
}

function InquiriesTile() {
  // Only the first page (newest 100) is needed to count the last 7 days.
  const { data, isPending, error } = useInfiniteQuery(submissionsQuery);
  const firstPage = data?.pages[0];
  const first = firstPage?.items ?? [];
  const since = Date.now() - WEEK_MS;
  const recent = first.filter((s) => new Date(s.createdAt).getTime() >= since).length;
  return (
    <Tile
      title="お問い合わせ"
      links={
        <AnchorLink to="/inquiries" size="sm" fw={700}>
          お問い合わせを見る
        </AnchorLink>
      }
    >
      {isPending ? (
        <Loading />
      ) : error ? (
        <Text size="sm" c="red.7">
          {errorMessage(error)}
        </Text>
      ) : (
        <>
          <Count n={recent} more={recent === first.length && firstPage?.more} />
          <Text size="sm" c="dimmed">
            この7日間に届いたもの
            {first[0] && `。最新は${formatDateTime(first[0].createdAt)}`}
          </Text>
        </>
      )}
    </Tile>
  );
}

function RecentEdits({ contents }: { contents: ContentSummary[] }) {
  const recent = [...contents]
    .sort((a, b) => b.updatedAt.localeCompare(a.updatedAt))
    .slice(0, RECENT);
  return (
    <Section title="最近編集した記事・ページ" variant="plain">
      <ResponsiveList
        items={recent}
        getKey={(c) => c.id}
        caption={`最近編集した記事・ページ（新しい順に${recent.length}件）`}
        columns={[
          {
            key: "title",
            header: "タイトル",
            render: (c) => (
              <AnchorLink
                to={c.type === "post" ? "/posts/$id" : "/pages/$id"}
                params={{ id: String(c.id) }}
                fw={700}
              >
                {c.title || "（タイトルなし）"}
              </AnchorLink>
            ),
          },
          { key: "type", header: "種類", nowrap: true, render: (c) => typeLabel[c.type] },
          { key: "status", header: "状態", render: (c) => <ContentStatusBadges {...c} /> },
          {
            key: "updatedAt",
            header: "最終更新",
            nowrap: true,
            render: (c) => <Text size="sm">{formatDateTime(c.updatedAt)}</Text>,
          },
        ]}
      />
    </Section>
  );
}

/** While a requested build has not finished yet, the card checks again this often. */
const BUILD_POLL_MS = 3000;

/**
 * The last export of the public site: when, and whether it worked. 「サイトを更新」 rebuilds
 * the whole site by hand (publishing normally does it); the card then follows that build.
 */
function SiteUpdate() {
  const queryClient = useQueryClient();
  const [requestedAt, setRequestedAt] = useState<string | null>(null);
  const publicUrl = usePublicUrl();
  const { data, isPending, error } = useQuery({
    ...buildsQuery,
    refetchInterval: (q) => {
      const latest = q.state.data?.[0];
      const waiting = requestedAt && (!latest || latest.startedAt < requestedAt);
      return waiting ? BUILD_POLL_MS : false;
    },
  });
  const last = data?.[0];
  const waiting = requestedAt !== null && (!last || last.startedAt < requestedAt);
  const build = useMutation({
    mutationFn: api.buildSite,
    onSuccess: () => {
      // A little earlier than now: the server's clock may be slightly behind ours.
      setRequestedAt(new Date(Date.now() - 60_000).toISOString());
      void queryClient.invalidateQueries({ queryKey: buildsQuery.queryKey });
      notify("サイトの更新を始めました。公開サイトに反映されるまで少しお待ちください。");
    },
    onError: (e) => notify(errorMessage(e), "error"),
  });
  return (
    <Section title="公開サイトの更新">
      {isPending ? (
        <Loading />
      ) : error ? (
        <ErrorMessage>{errorMessage(error)}</ErrorMessage>
      ) : !last ? (
        <Group gap="sm" wrap="nowrap" align="flex-start">
          <ThemeIcon variant="light" color="gray" aria-hidden="true">
            <CircleDashed size={18} />
          </ThemeIcon>
          <Text>公開サイトはまだできていません。記事やページを公開すると、自動で作られます。</Text>
        </Group>
      ) : last.error ? (
        <Alert
          color="red"
          title="前回の更新がうまくいきませんでした"
          icon={<CircleAlert aria-hidden="true" size={18} />}
        >
          <Stack gap="xs">
            <Text size="sm">
              {formatJapanDateTime(last.finishedAt)}
              に公開サイトを更新しようとしましたが、失敗しました。下の「サイトを更新」をもう一度押してください。何度も失敗するときは、サイトを作った会社や担当者に連絡してください。
            </Text>
            <Text size="sm">くわしい内容：{last.error}</Text>
          </Stack>
        </Alert>
      ) : (
        <Group gap="sm" wrap="nowrap" align="flex-start">
          <ThemeIcon variant="light" color="green" aria-hidden="true">
            <CircleCheck size={18} />
          </ThemeIcon>
          <Box>
            <Text fw={700}>前回の更新はうまくいきました</Text>
            <Text size="sm" c="dimmed">
              最後の更新：{formatJapanDateTime(last.finishedAt)}
            </Text>
          </Box>
        </Group>
      )}
      {last && last.warnings.length > 0 && (
        <Alert
          color="orange"
          title={`気をつけること（${last.warnings.length}件）`}
          icon={<TriangleAlert aria-hidden="true" size={18} />}
        >
          <Text size="sm" mb={4}>
            前回の更新で、うまく公開サイトに出せなかったものがあります。
          </Text>
          <List size="sm" spacing={2}>
            {last.warnings.map((w) => (
              <List.Item key={w}>{w}</List.Item>
            ))}
          </List>
        </Alert>
      )}
      <Stack gap={6} align="flex-start">
        <Button
          size="sm"
          leftSection={<RefreshCw aria-hidden="true" size={16} />}
          onClick={() => build.mutate()}
          loading={build.isPending}
          aria-describedby="build-help"
        >
          サイトを更新
        </Button>
        <Text id="build-help" size="sm" c="dimmed" aria-live="polite">
          {waiting
            ? "更新しています。終わると、ここに結果が出ます。"
            : "公開サイト全体を作り直します。ふだんは公開するたびに自動で更新されるので、表示がおかしいときだけ押してください。"}
        </Text>
      </Stack>
      <ExternalLink href={publicUrl("/")}>公開サイトを見る</ExternalLink>
    </Section>
  );
}
