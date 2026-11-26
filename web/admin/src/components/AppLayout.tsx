import {
  AppShell,
  Box,
  Button,
  Group,
  Menu,
  NavLink,
  ScrollArea,
  Stack,
  Text,
  UnstyledButton,
  VisuallyHidden,
} from "@mantine/core";
import { useDisclosure, useMediaQuery } from "@mantine/hooks";
import { useMutation, useQueryClient, useSuspenseQuery } from "@tanstack/react-query";
import { Link, Outlet, useLocation, useMatchRoute, useNavigate } from "@tanstack/react-router";
import {
  Check,
  ChevronsUpDown,
  ExternalLink,
  FileText,
  FolderTree,
  House,
  Inbox,
  LogOut,
  type LucideIcon,
  Menu as MenuIcon,
  Newspaper,
  Settings,
  Tags,
  UserRound,
  X,
} from "lucide-react";
import { useEffect } from "react";
import { api, errorMessage } from "../api/client";
import { meQuery } from "../api/queries";
import { useIsAdmin } from "../lib/useIsAdmin";
import { usePublicUrl } from "../lib/usePublicUrl";
import { DESKTOP_QUERY } from "../theme";
import classes from "./AppLayout.module.css";
import { notify } from "./ui/notify";

type NavItem = {
  to: "/" | "/posts" | "/pages" | "/categories" | "/tags" | "/inquiries" | "/settings";
  label: string;
  icon: LucideIcon;
  exact?: boolean;
  adminOnly?: boolean;
};

/** The sidebar's main menu, in order. Groups are separated by a small gap. */
const navGroups: NavItem[][] = [
  [{ to: "/", label: "ホーム", icon: House, exact: true }],
  [
    { to: "/posts", label: "記事", icon: Newspaper },
    { to: "/pages", label: "固定ページ", icon: FileText },
    { to: "/categories", label: "カテゴリ", icon: FolderTree },
    { to: "/tags", label: "タグ", icon: Tags },
  ],
  [
    { to: "/inquiries", label: "お問い合わせ", icon: Inbox, adminOnly: true },
    { to: "/settings", label: "サイトの設定", icon: Settings, adminOnly: true },
  ],
];

const NAVBAR_ID = "pw-sidebar";

/**
 * The app shell: a sidebar (site, main menu, public site link, account) and the main area.
 * On phones the sidebar is a drawer opened from the top bar's menu button.
 */
export function AppLayout() {
  const { data: me } = useSuspenseQuery(meQuery);
  const [opened, { toggle, close }] = useDisclosure(false);
  const { pathname } = useLocation();
  const site = me.sites.find((s) => s.id === me.currentSiteId);
  const desktop = useMediaQuery(DESKTOP_QUERY, true, { getInitialValueInEffect: false });
  const publicUrl = usePublicUrl();

  // Close the drawer after moving to another screen.
  useEffect(() => {
    if (pathname) close();
  }, [pathname, close]);

  return (
    <>
      <a href="#main" className="pw-skip-link">
        本文へ移動
      </a>
      <AppShell
        header={{ height: { base: 56, sm: 0 } }}
        navbar={{ width: 256, breakpoint: "sm", collapsed: { mobile: !opened } }}
      >
        <AppShell.Header hiddenFrom="sm" px="md">
          <Group h="100%" gap="sm" wrap="nowrap" justify="space-between">
            {/* The open drawer shows the site name itself: show it here only while closed. */}
            <Text fw={700} truncate aria-hidden={opened || undefined}>
              {opened ? "" : (site?.name ?? "サイト")}
            </Text>
            <Button
              onClick={toggle}
              size="sm"
              h="2.75rem"
              px="sm"
              aria-expanded={opened}
              aria-controls={NAVBAR_ID}
              leftSection={
                opened ? (
                  <X aria-hidden="true" size={20} />
                ) : (
                  <MenuIcon aria-hidden="true" size={20} />
                )
              }
              className={classes.menuButton}
            >
              {/* One name in both states; aria-expanded tells whether it is open. */}
              メニュー
            </Button>
          </Group>
        </AppShell.Header>

        <AppShell.Navbar
          id={NAVBAR_ID}
          aria-label="メインメニュー"
          className={classes.navbar}
          // The closed drawer is only moved off screen: keep it out of the Tab order.
          inert={!desktop && !opened}
        >
          <AppShell.Section p="sm" pb={0}>
            <SiteSwitcher />
          </AppShell.Section>
          <AppShell.Section grow component={ScrollArea} p="sm">
            <MainMenu />
          </AppShell.Section>
          <AppShell.Section p="sm" className={classes.footer}>
            <NavLink
              href={publicUrl()}
              target="_blank"
              rel="noopener noreferrer"
              label={
                <>
                  公開サイトを見る
                  <VisuallyHidden>（新しいタブで開きます）</VisuallyHidden>
                </>
              }
              leftSection={<ExternalLink aria-hidden="true" size={18} />}
              className={classes.link}
            />
            <UserMenu />
          </AppShell.Section>
        </AppShell.Navbar>

        <AppShell.Main id="main" tabIndex={-1} className={classes.main}>
          <Box className={classes.content}>
            <Outlet />
          </Box>
        </AppShell.Main>
      </AppShell>
    </>
  );
}

function MainMenu() {
  const isAdmin = useIsAdmin();
  const matchRoute = useMatchRoute();
  return (
    <Stack gap="md">
      {navGroups.map((group) => {
        const items = group.filter((item) => isAdmin || !item.adminOnly);
        if (items.length === 0) return null;
        return (
          <Stack key={items[0].to} gap={2} component="ul" className={classes.list}>
            {items.map(({ to, label, icon: Icon, exact }) => (
              <li key={to}>
                <NavLink
                  component={Link}
                  to={to}
                  activeOptions={{ exact: !!exact }}
                  active={!!matchRoute({ to, fuzzy: !exact })}
                  label={label}
                  leftSection={<Icon aria-hidden="true" size={18} />}
                  className={classes.link}
                />
              </li>
            ))}
          </Stack>
        );
      })}
    </Stack>
  );
}

/** Site name at the top of the sidebar; a menu to switch when the user has several sites. */
function SiteSwitcher() {
  const { data: me } = useSuspenseQuery(meQuery);
  const queryClient = useQueryClient();
  const navigate = useNavigate();
  const site = me.sites.find((s) => s.id === me.currentSiteId);
  const name = site?.name ?? "サイト";

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

  const label = (
    <Box miw={0}>
      <Text size="xs" c="dimmed">
        管理中のサイト
      </Text>
      <Text fw={700} truncate>
        {name}
      </Text>
    </Box>
  );

  if (me.sites.length <= 1) return <Box px="xs">{label}</Box>;

  return (
    <Menu position="bottom-start" width="target">
      <Menu.Target>
        <UnstyledButton className={classes.siteButton} disabled={select.isPending}>
          <Group gap="xs" wrap="nowrap" justify="space-between">
            {label}
            <VisuallyHidden>（サイトを切り替える）</VisuallyHidden>
            <ChevronsUpDown aria-hidden="true" size={16} />
          </Group>
        </UnstyledButton>
      </Menu.Target>
      <Menu.Dropdown>
        <Menu.Label>サイトを切り替える</Menu.Label>
        {me.sites.map((s) => {
          const current = s.id === me.currentSiteId;
          return (
            <Menu.Item
              key={s.id}
              leftSection={
                current ? <Check aria-hidden="true" size={16} /> : <Box w={16} aria-hidden />
              }
              onClick={() => !current && select.mutate(s.id)}
            >
              {s.name}
              {current && <VisuallyHidden>（選択中）</VisuallyHidden>}
            </Menu.Item>
          );
        })}
      </Menu.Dropdown>
    </Menu>
  );
}

function UserMenu() {
  const { data: me } = useSuspenseQuery(meQuery);
  const queryClient = useQueryClient();
  const navigate = useNavigate();
  const name = me.user.displayName || me.user.email;

  const logout = useMutation({
    mutationFn: api.logout,
    onSuccess: async () => {
      await navigate({ to: "/login" });
      queryClient.clear();
    },
    onError: (e) => notify(errorMessage(e), "error"),
  });

  return (
    <Menu position="top-start" width="target">
      <Menu.Target>
        {/* Named from its content (not aria-label), so the visible name is part of it. */}
        <Button
          fullWidth
          size="sm"
          h="2.75rem"
          mt="xs"
          justify="space-between"
          leftSection={<UserRound aria-hidden="true" size={18} />}
          rightSection={<ChevronsUpDown aria-hidden="true" size={16} />}
          classNames={{ label: classes.userLabel }}
        >
          <VisuallyHidden>アカウントのメニュー：</VisuallyHidden>
          {name}
        </Button>
      </Menu.Target>
      <Menu.Dropdown>
        <Menu.Label>{me.user.email} でログイン中</Menu.Label>
        <Menu.Item
          leftSection={<LogOut aria-hidden="true" size={16} />}
          onClick={() => logout.mutate()}
          disabled={logout.isPending}
        >
          ログアウト
        </Menu.Item>
      </Menu.Dropdown>
    </Menu>
  );
}
