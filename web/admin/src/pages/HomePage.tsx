import { Card, Group, SimpleGrid, Text, ThemeIcon } from "@mantine/core";
import { useSuspenseQuery } from "@tanstack/react-query";
import { FileText, FolderTree, Inbox, Newspaper, Settings, Tags } from "lucide-react";
import { meQuery } from "../api/queries";
import { NewContentButton } from "../components/NewContentButton";
import { AnchorLink } from "../components/ui/AnchorLink";
import { PageHeader } from "../components/ui/PageHeader";
import { Section } from "../components/ui/Section";
import { useIsAdmin } from "../lib/useIsAdmin";

const links = [
  { to: "/posts", label: "記事の一覧", icon: Newspaper, text: "お知らせやブログの記事" },
  { to: "/pages", label: "固定ページの一覧", icon: FileText, text: "会社概要など、常に置くページ" },
  { to: "/categories", label: "カテゴリ", icon: FolderTree, text: "記事の分類（階層あり）" },
  { to: "/tags", label: "タグ", icon: Tags, text: "記事につけるキーワード" },
  {
    to: "/inquiries",
    label: "お問い合わせ",
    icon: Inbox,
    text: "フォームから届いた内容",
    adminOnly: true,
  },
  {
    to: "/settings",
    label: "サイトの設定",
    icon: Settings,
    text: "サイトの名前・ロゴ・色・メニューなど",
    adminOnly: true,
  },
] as const;

export function HomePage() {
  const { data: me } = useSuspenseQuery(meQuery);
  const isAdmin = useIsAdmin();
  return (
    <>
      <PageHeader
        title={`ようこそ、${me.user.displayName || me.user.email} さん`}
        documentTitle="ホーム"
        actions={
          <>
            <NewContentButton type="page" label="固定ページを作る" />
            <NewContentButton type="post" label="記事を書く" primary />
          </>
        }
      />
      <Section title="メニュー" variant="plain">
        <SimpleGrid component="ul" cols={{ base: 1, sm: 2 }} spacing="sm" m={0} p={0}>
          {links
            .filter((l) => isAdmin || !("adminOnly" in l))
            .map(({ to, label, icon: Icon, text }) => (
              <Card component="li" key={to} padding="md" style={{ listStyle: "none" }}>
                <Group gap="sm" wrap="nowrap" align="flex-start">
                  <ThemeIcon variant="light" size="lg" aria-hidden="true">
                    <Icon size={18} />
                  </ThemeIcon>
                  <div>
                    <AnchorLink to={to} fw={700}>
                      {label}
                    </AnchorLink>
                    <Text size="sm" c="dimmed">
                      {text}
                    </Text>
                  </div>
                </Group>
              </Card>
            ))}
        </SimpleGrid>
      </Section>
    </>
  );
}
