import type { ContentSummary } from "../api/types";

export type PageRow = ContentSummary & { depth: number; ancestors: ContentSummary[] };

/**
 * Orders pages parents-first, keeping the given order among siblings. Pages whose
 * parent is missing are treated as top level.
 */
export function pageRows(pages: ContentSummary[]): PageRow[] {
  const ids = new Set(pages.map((p) => p.id));
  const out: PageRow[] = [];
  const visit = (p: ContentSummary, ancestors: ContentSummary[]) => {
    if (ancestors.some((a) => a.id === p.id)) return; // guards against cycles
    out.push({ ...p, depth: ancestors.length, ancestors });
    for (const c of pages) if (c.parentId === p.id) visit(c, [...ancestors, p]);
  };
  for (const p of pages) if (p.parentId === null || !ids.has(p.parentId)) visit(p, []);
  return out;
}

/** Ids of the page and all its descendants (cannot be chosen as its parent). */
export function descendantPageIds(pages: ContentSummary[], id: number): Set<number> {
  const out = new Set([id]);
  let grew = true;
  while (grew) {
    grew = false;
    for (const p of pages) {
      if (p.parentId !== null && out.has(p.parentId) && !out.has(p.id)) {
        out.add(p.id);
        grew = true;
      }
    }
  }
  return out;
}
