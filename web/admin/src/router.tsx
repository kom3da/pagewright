import { Box, Stack, Text } from "@mantine/core";
import type { QueryClient } from "@tanstack/react-query";
import {
  createRootRouteWithContext,
  createRoute,
  createRouter,
  Outlet,
  type RouterHistory,
  redirect,
} from "@tanstack/react-router";
import { lazy, type ReactNode } from "react";
import { errorMessage, isApiError } from "./api/client";
import { meQuery } from "./api/queries";
import type { ContentType } from "./api/types";
import { AppLayout } from "./components/AppLayout";
import { AnchorLink } from "./components/ui/AnchorLink";
import { ErrorMessage, Loading } from "./components/ui/feedback";
import { PageTitle } from "./components/ui/PageHeader";
import { AccountPage } from "./pages/AccountPage";
import { CategoriesPage } from "./pages/CategoriesPage";
import { ContentListPage, validateListSearch } from "./pages/ContentListPage";
import { HomePage } from "./pages/HomePage";
import { InviteAcceptPage } from "./pages/InviteAcceptPage";
import { LoginPage } from "./pages/LoginPage";
import { MembersPage } from "./pages/MembersPage";
import { SelectSitePage } from "./pages/SelectSitePage";
import { SiteSettingsPage } from "./pages/SiteSettingsPage";
import { SubmissionsPage, validateSubmissionsSearch } from "./pages/SubmissionsPage";
import { TagsPage } from "./pages/TagsPage";
import { VerifyPage } from "./pages/VerifyPage";

// The editor (Tiptap) is the heaviest screen; load it only when needed.
const EditorPage = lazy(() =>
  import("./pages/EditorPage").then((m) => ({ default: m.EditorPage })),
);

type RouterContext = { queryClient: QueryClient };

async function loadMe(queryClient: QueryClient) {
  try {
    return await queryClient.ensureQueryData(meQuery);
  } catch (e) {
    if (isApiError(e, 401)) throw redirect({ to: "/login" });
    throw e;
  }
}

/** Errors and "not found" outside the app shell (it may be what failed). */
function StandaloneMessage({ title, children }: { title: string; children: ReactNode }) {
  return (
    <Box component="main" id="main" maw={720} mx="auto" p="xl">
      <Stack gap="md">
        <PageTitle documentTitle={title}>{title}</PageTitle>
        {children}
        <AnchorLink to="/">ホームに戻る</AnchorLink>
      </Stack>
    </Box>
  );
}

function RouteError({ error }: { error: unknown }) {
  return (
    <StandaloneMessage title="エラーが起きました">
      <ErrorMessage>{errorMessage(error)}</ErrorMessage>
    </StandaloneMessage>
  );
}

function NotFound() {
  return (
    <StandaloneMessage title="ページが見つかりません">
      <Text>URLが間違っているか、削除された可能性があります。</Text>
    </StandaloneMessage>
  );
}

const rootRoute = createRootRouteWithContext<RouterContext>()({
  component: Outlet,
  errorComponent: RouteError,
  notFoundComponent: NotFound,
});

const loginRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "login",
  component: LoginPage,
});

const verifyRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "login/verify",
  validateSearch: (search: Record<string, unknown>): { token?: string } =>
    typeof search.token === "string" ? { token: search.token } : {},
  component: function Verify() {
    const { token } = verifyRoute.useSearch();
    return <VerifyPage token={token ?? ""} />;
  },
});

/** The emailed invitation link. Works logged in or out: accepting it also logs in. */
const inviteRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "invite/accept",
  validateSearch: (search: Record<string, unknown>): { token?: string } =>
    typeof search.token === "string" ? { token: search.token } : {},
  component: function Invite() {
    const { token } = inviteRoute.useSearch();
    return <InviteAcceptPage token={token ?? ""} />;
  },
});

const selectSiteRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "select-site",
  beforeLoad: ({ context }) => loadMe(context.queryClient),
  component: SelectSitePage,
});

/** Every screen below requires login and a selected site. */
const appRoute = createRoute({
  getParentRoute: () => rootRoute,
  id: "app",
  beforeLoad: async ({ context }) => {
    const me = await loadMe(context.queryClient);
    if (!me.currentSiteId) throw redirect({ to: "/select-site" });
  },
  component: AppLayout,
});

const homeRoute = createRoute({
  getParentRoute: () => appRoute,
  path: "/",
  component: HomePage,
});

const listRoute = (path: "posts" | "pages", type: ContentType) => {
  const route = createRoute({
    getParentRoute: () => appRoute,
    path,
    validateSearch: validateListSearch,
    component: function List() {
      return <ContentListPage type={type} search={route.useSearch()} />;
    },
  });
  return route;
};

const postsRoute = listRoute("posts", "post");
const pagesRoute = listRoute("pages", "page");

const postEditRoute = createRoute({
  getParentRoute: () => appRoute,
  path: "posts/$id",
  component: function PostEdit() {
    const { id } = postEditRoute.useParams();
    return <EditorPage key={id} type="post" id={Number(id)} />;
  },
});

const pageEditRoute = createRoute({
  getParentRoute: () => appRoute,
  path: "pages/$id",
  component: function PageEdit() {
    const { id } = pageEditRoute.useParams();
    return <EditorPage key={id} type="page" id={Number(id)} />;
  },
});

const categoriesRoute = createRoute({
  getParentRoute: () => appRoute,
  path: "categories",
  component: CategoriesPage,
});

const tagsRoute = createRoute({
  getParentRoute: () => appRoute,
  path: "tags",
  component: TagsPage,
});

const accountRoute = createRoute({
  getParentRoute: () => appRoute,
  path: "account",
  component: AccountPage,
});

const settingsRoute = createRoute({
  getParentRoute: () => appRoute,
  path: "settings",
  component: SiteSettingsPage,
});

const membersRoute = createRoute({
  getParentRoute: () => appRoute,
  path: "members",
  component: MembersPage,
});

const inquiriesRoute = createRoute({
  getParentRoute: () => appRoute,
  path: "inquiries",
  validateSearch: validateSubmissionsSearch,
  component: function Inquiries() {
    return <SubmissionsPage search={inquiriesRoute.useSearch()} />;
  },
});

const routeTree = rootRoute.addChildren([
  loginRoute,
  verifyRoute,
  inviteRoute,
  selectSiteRoute,
  appRoute.addChildren([
    homeRoute,
    postsRoute,
    pagesRoute,
    postEditRoute,
    pageEditRoute,
    categoriesRoute,
    tagsRoute,
    settingsRoute,
    inquiriesRoute,
    membersRoute,
    accountRoute,
  ]),
]);

export function createAppRouter(queryClient: QueryClient, history?: RouterHistory) {
  return createRouter({
    routeTree,
    history,
    basepath: "/admin",
    context: { queryClient },
    defaultPreload: false,
    defaultPendingComponent: () => <Loading />,
  });
}

declare module "@tanstack/react-router" {
  interface Register {
    router: ReturnType<typeof createAppRouter>;
  }
}
