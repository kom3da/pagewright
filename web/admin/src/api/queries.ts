import { infiniteQueryOptions, queryOptions } from "@tanstack/react-query";
import { api, SUBMISSIONS_PAGE_SIZE } from "./client";
import type { ContentFilter, ContentType } from "./types";

export const meQuery = queryOptions({ queryKey: ["me"], queryFn: api.me, staleTime: 60_000 });

// Filtered lists share the ["contents", type] prefix, so invalidating it refreshes them all.
export const contentsQuery = (type: ContentType, filter: ContentFilter = {}) =>
  queryOptions({
    queryKey: filter.q || filter.status ? ["contents", type, filter] : ["contents", type],
    queryFn: () => api.listContents(type, filter),
  });

/** How many contents the paged lists load at a time. */
export const CONTENTS_PAGE_SIZE = 100;

// The list of posts loads CONTENTS_PAGE_SIZE at a time (a site may have thousands). The key
// shares the ["contents", type] prefix, so the same invalidations refresh it.
export const contentsPagesQuery = (type: ContentType, filter: ContentFilter = {}) =>
  infiniteQueryOptions({
    queryKey: ["contents", type, "paged", filter],
    queryFn: async ({ pageParam }) => {
      const items = await api.listContents(type, filter, {
        before: pageParam,
        limit: CONTENTS_PAGE_SIZE,
      });
      return { items, more: items.length >= CONTENTS_PAGE_SIZE, next: items.at(-1)?.number };
    },
    initialPageParam: undefined as number | undefined,
    getNextPageParam: (last) => (last.more ? last.next : undefined),
  });

/** How many choices the content picker shows at a time. */
export const CONTENT_PICKER_LIMIT = 20;

/** The picker's choices: the newest, or those matching `q`. Shares the ["contents", type] prefix. */
export const contentChoicesQuery = (type: ContentType, q: string) =>
  queryOptions({
    queryKey: ["contents", type, "choices", q],
    queryFn: () => api.listContents(type, { q }, { limit: CONTENT_PICKER_LIMIT }),
  });

/** The last updated contents of a type, for the home. Shares the ["contents", type] prefix. */
export const recentContentsQuery = (type: ContentType, limit: number) =>
  queryOptions({
    queryKey: ["contents", type, "recent", limit],
    queryFn: () => api.listRecentContents(type, limit),
  });

/** The home's status counts of both types (the lists stop at 1000 rows). */
export const contentCountsQuery = queryOptions({
  queryKey: ["contents", "counts"],
  queryFn: api.contentCounts,
});

export const contentQuery = (id: number) =>
  queryOptions({ queryKey: ["content", id], queryFn: () => api.getContent(id) });

export const previewLinksQuery = (contentId: number) =>
  queryOptions({
    queryKey: ["previewLinks", contentId],
    queryFn: () => api.listPreviewLinks(contentId),
  });

export const categoriesQuery = queryOptions({
  queryKey: ["categories"],
  queryFn: api.listCategories,
});

export const tagsQuery = queryOptions({ queryKey: ["tags"], queryFn: api.listTags });

export const mediaQuery = queryOptions({ queryKey: ["media"], queryFn: api.listMedia });

export const siteQuery = queryOptions({ queryKey: ["site"], queryFn: api.getSite });

/** The latest export of the public site, for the home screen. */
export const buildsQuery = queryOptions({
  queryKey: ["builds"],
  queryFn: () => api.listBuilds(1),
});

// Contact messages are personal data: never persisted, and dropped from memory as soon
// as no screen shows them.
export const submissionsQuery = infiniteQueryOptions({
  queryKey: ["submissions"],
  // Whether there may be more, and where to continue, are decided from the page as it was
  // fetched: deleting from the cached page must not hide 「もっと見る」.
  queryFn: async ({ pageParam }) => {
    const items = await api.listSubmissions(pageParam);
    return {
      items,
      more: items.length >= SUBMISSIONS_PAGE_SIZE,
      next: items.at(-1)?.id,
    };
  },
  initialPageParam: undefined as number | undefined,
  // A full page means there may be more; the next page starts below its last id.
  getNextPageParam: (last) => (last.more ? last.next : undefined),
  gcTime: 0,
});
