import { useSuspenseQuery } from "@tanstack/react-query";
import { Link } from "@tanstack/react-router";
import { FileText, FolderTree, Inbox, Newspaper, Settings, Tags } from "lucide-react";
import { meQuery } from "../api/queries";
import { NewContentButton } from "../components/NewContentButton";
import { PageHeading } from "../components/ui/misc";
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
    <div className="space-y-8">
      <PageHeading title="ホーム">
        ようこそ、{me.user.displayName || me.user.email} さん
      </PageHeading>
      <section aria-labelledby="quick-start" className="space-y-3">
        <h2 id="quick-start" className="text-lg font-bold">
          すぐに始める
        </h2>
        <div className="flex flex-wrap gap-3">
          <NewContentButton type="post" label="記事を書く" />
          <NewContentButton type="page" label="固定ページを作る" />
        </div>
      </section>
      <section aria-labelledby="menu" className="space-y-3">
        <h2 id="menu" className="text-lg font-bold">
          メニュー
        </h2>
        <ul className="grid gap-3 sm:grid-cols-2">
          {links
            .filter((l) => isAdmin || !("adminOnly" in l))
            .map(({ to, label, icon: Icon, text }) => (
              <li key={to}>
                <Link
                  to={to}
                  className="flex items-start gap-3 rounded-lg border border-slate-300 bg-white p-4 hover:border-blue-700"
                >
                  <Icon aria-hidden="true" className="mt-1 text-blue-700" />
                  <span>
                    <span className="block font-bold text-blue-800 underline">{label}</span>
                    <span className="text-sm text-slate-700">{text}</span>
                  </span>
                </Link>
              </li>
            ))}
        </ul>
      </section>
    </div>
  );
}
