import { useMutation, useQueryClient, useSuspenseQuery } from "@tanstack/react-query";
import { Link, Outlet, useNavigate } from "@tanstack/react-router";
import { LogOut, RefreshCw } from "lucide-react";
import { type FormEvent, useId, useState } from "react";
import { api, errorMessage } from "../api/client";
import { meQuery } from "../api/queries";
import { useIsAdmin } from "../lib/useIsAdmin";
import { useNotify } from "./Toaster";
import { Button } from "./ui/Button";

const navItems = [
  { to: "/", label: "ホーム", exact: true },
  { to: "/posts", label: "記事" },
  { to: "/pages", label: "固定ページ" },
  { to: "/categories", label: "カテゴリ" },
  { to: "/tags", label: "タグ" },
  { to: "/inquiries", label: "お問い合わせ", adminOnly: true },
  { to: "/settings", label: "サイトの設定", adminOnly: true },
] as const;

export function AppLayout() {
  const { data: me } = useSuspenseQuery(meQuery);
  const queryClient = useQueryClient();
  const navigate = useNavigate();
  const notify = useNotify();
  const isAdmin = useIsAdmin();

  const logout = useMutation({
    mutationFn: api.logout,
    onSuccess: async () => {
      await navigate({ to: "/login" });
      queryClient.clear();
    },
    onError: (e) => notify(errorMessage(e), "error"),
  });

  const build = useMutation({
    mutationFn: api.buildSite,
    onSuccess: () =>
      notify("サイトの更新を始めました。公開サイトに反映されるまで少しお待ちください。"),
    onError: (e) => notify(errorMessage(e), "error"),
  });

  const site = me.sites.find((s) => s.id === me.currentSiteId);

  return (
    <div className="flex min-h-screen flex-col">
      <a
        href="#main"
        className="sr-only focus:not-sr-only focus:absolute focus:top-2 focus:left-2 focus:z-50 focus:rounded focus:bg-white focus:p-2"
      >
        本文へ移動
      </a>
      <header className="border-b border-slate-300 bg-white">
        <div className="mx-auto flex max-w-6xl flex-wrap items-center gap-x-6 gap-y-2 px-4 py-3">
          <p className="font-bold">
            {site?.name ?? "サイト"}
            <span className="ml-2 text-sm font-normal text-slate-700">の管理画面</span>
          </p>
          <div className="ml-auto flex flex-wrap items-center gap-3">
            <Button
              variant="primary"
              onClick={() => build.mutate()}
              disabled={build.isPending}
              aria-describedby="build-help"
            >
              <RefreshCw aria-hidden="true" size={16} />
              {build.isPending ? "更新しています…" : "サイトを更新"}
            </Button>
            <span id="build-help" className="sr-only">
              公開サイト全体を作り直します。ふだんは公開するたびに自動で更新されます。
            </span>
            {me.sites.length > 1 && <SiteSwitcher />}
            <span className="text-sm text-slate-700">{me.user.displayName || me.user.email}</span>
            <Button variant="ghost" onClick={() => logout.mutate()} disabled={logout.isPending}>
              <LogOut aria-hidden="true" size={16} />
              ログアウト
            </Button>
          </div>
        </div>
        <nav aria-label="メインメニュー" className="mx-auto max-w-6xl px-4">
          <ul className="flex flex-wrap gap-1">
            {navItems
              .filter((item) => isAdmin || !("adminOnly" in item))
              .map((item) => (
                <li key={item.to}>
                  <Link
                    to={item.to}
                    activeOptions={{ exact: "exact" in item }}
                    className="block border-b-4 border-transparent px-3 py-2 font-semibold text-slate-800 hover:bg-slate-100 aria-[current=page]:border-blue-700 aria-[current=page]:text-blue-800"
                  >
                    {item.label}
                  </Link>
                </li>
              ))}
          </ul>
        </nav>
      </header>
      <main id="main" tabIndex={-1} className="mx-auto w-full max-w-6xl flex-1 px-4 py-6">
        <Outlet />
      </main>
    </div>
  );
}

function SiteSwitcher() {
  const { data: me } = useSuspenseQuery(meQuery);
  const queryClient = useQueryClient();
  const navigate = useNavigate();
  const notify = useNotify();
  const id = useId();
  const [siteId, setSiteId] = useState(me.currentSiteId ?? "");

  const select = useMutation({
    mutationFn: api.selectSite,
    onSuccess: (next) => {
      queryClient.clear();
      queryClient.setQueryData(meQuery.queryKey, next);
      void navigate({ to: "/" });
      notify(`「${next.sites.find((s) => s.id === next.currentSiteId)?.name}」に切り替えました。`);
    },
    onError: (e) => notify(errorMessage(e), "error"),
  });

  const onSubmit = (e: FormEvent) => {
    e.preventDefault();
    if (siteId && siteId !== me.currentSiteId) select.mutate(siteId);
  };

  return (
    <form onSubmit={onSubmit} className="flex items-center gap-2">
      <label htmlFor={id} className="text-sm font-semibold">
        サイト
      </label>
      <select
        id={id}
        value={siteId}
        onChange={(e) => setSiteId(e.target.value)}
        className="rounded-md border border-slate-500 bg-white px-2 py-2 text-sm"
      >
        {me.sites.map((s) => (
          <option key={s.id} value={s.id}>
            {s.name}
          </option>
        ))}
      </select>
      <Button type="submit" disabled={select.isPending || siteId === me.currentSiteId}>
        切り替える
      </Button>
    </form>
  );
}
