import type { QueryClient } from "@tanstack/react-query";
import {
  createRootRouteWithContext,
  createRoute,
  createRouter,
  Link,
  Outlet,
  type RouterHistory,
  redirect,
} from "@tanstack/react-router";
import { lazy } from "react";
import { errorMessage, isApiError } from "./api/client";
import { meQuery } from "./api/queries";
import type { ContentType } from "./api/types";
import { AppLayout } from "./components/AppLayout";
import { ErrorMessage, Loading, PageHeading } from "./components/ui/misc";
import { CategoriesPage } from "./pages/CategoriesPage";
import { ContentListPage } from "./pages/ContentListPage";
import { HomePage } from "./pages/HomePage";
import { LoginPage } from "./pages/LoginPage";
import { SelectSitePage } from "./pages/SelectSitePage";
import { SiteSettingsPage } from "./pages/SiteSettingsPage";
import { SubmissionsPage } from "./pages/SubmissionsPage";
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

function RouteError({ error }: { error: unknown }) {
  return (
    <main id="main" className="mx-auto max-w-3xl space-y-4 p-6">
      <PageHeading title="エラー">エラーが起きました</PageHeading>
      <ErrorMessage>{errorMessage(error)}</ErrorMessage>
      <Link to="/" className="text-blue-700 underline">
        ホームに戻る
      </Link>
    </main>
  );
}

function NotFound() {
  return (
    <main id="main" className="mx-auto max-w-3xl space-y-4 p-6">
      <PageHeading title="ページが見つかりません">ページが見つかりません</PageHeading>
      <p>URLが間違っているか、削除された可能性があります。</p>
      <Link to="/" className="text-blue-700 underline">
        ホームに戻る
      </Link>
    </main>
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

const listRoute = (path: "posts" | "pages", type: ContentType) =>
  createRoute({
    getParentRoute: () => appRoute,
    path,
    component: () => <ContentListPage type={type} />,
  });

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

const settingsRoute = createRoute({
  getParentRoute: () => appRoute,
  path: "settings",
  component: SiteSettingsPage,
});

const inquiriesRoute = createRoute({
  getParentRoute: () => appRoute,
  path: "inquiries",
  component: SubmissionsPage,
});

const routeTree = rootRoute.addChildren([
  loginRoute,
  verifyRoute,
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
