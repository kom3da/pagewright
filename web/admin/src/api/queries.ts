import { infiniteQueryOptions, queryOptions } from "@tanstack/react-query";
import { api, SUBMISSIONS_PAGE_SIZE } from "./client";
import type { ContentType } from "./types";

export const meQuery = queryOptions({ queryKey: ["me"], queryFn: api.me, staleTime: 60_000 });

export const contentsQuery = (type: ContentType) =>
  queryOptions({ queryKey: ["contents", type], queryFn: () => api.listContents(type) });

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

// Contact messages are personal data: never persisted, and dropped from memory as soon
// as no screen shows them.
export const submissionsQuery = infiniteQueryOptions({
  queryKey: ["submissions"],
  queryFn: ({ pageParam }) => api.listSubmissions(pageParam),
  initialPageParam: undefined as number | undefined,
  // A full page means there may be more; the next page starts below its last id.
  getNextPageParam: (last) =>
    last.length >= SUBMISSIONS_PAGE_SIZE ? last[last.length - 1].id : undefined,
  gcTime: 0,
});
