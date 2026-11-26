import { queryOptions } from "@tanstack/react-query";
import { api } from "./client";
import type { ContentType } from "./types";

export const meQuery = queryOptions({ queryKey: ["me"], queryFn: api.me, staleTime: 60_000 });

export const contentsQuery = (type: ContentType) =>
  queryOptions({ queryKey: ["contents", type], queryFn: () => api.listContents(type) });

export const contentQuery = (id: number) =>
  queryOptions({ queryKey: ["content", id], queryFn: () => api.getContent(id) });

export const categoriesQuery = queryOptions({
  queryKey: ["categories"],
  queryFn: api.listCategories,
});

export const tagsQuery = queryOptions({ queryKey: ["tags"], queryFn: api.listTags });

export const mediaQuery = queryOptions({ queryKey: ["media"], queryFn: api.listMedia });
