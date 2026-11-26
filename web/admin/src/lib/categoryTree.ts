import type { Category } from "../api/types";

export type CategoryNode = Category & { children: CategoryNode[]; depth: number };

const byOrder = (a: Category, b: Category) =>
  a.position - b.position || a.name.localeCompare(b.name, "ja");

/** Builds the category tree. Categories whose parent is missing are treated as top level. */
export function buildCategoryTree(categories: Category[]): CategoryNode[] {
  const ids = new Set(categories.map((c) => c.id));
  const build = (parentId: number | null, depth: number, seen: Set<number>): CategoryNode[] =>
    categories
      .filter((c) =>
        parentId === null ? c.parentId === null || !ids.has(c.parentId) : c.parentId === parentId,
      )
      .filter((c) => !seen.has(c.id))
      .sort(byOrder)
      .map((c) => ({
        ...c,
        depth,
        children: build(c.id, depth + 1, new Set([...seen, c.id])),
      }));
  return build(null, 0, new Set());
}

/** Flattens the tree in display order (parents before children). */
export function flattenTree(nodes: CategoryNode[]): CategoryNode[] {
  return nodes.flatMap((n) => [n, ...flattenTree(n.children)]);
}

/** Ids of the category and all its descendants (cannot be chosen as its parent). */
export function descendantIds(categories: Category[], id: number): Set<number> {
  const out = new Set([id]);
  let grew = true;
  while (grew) {
    grew = false;
    for (const c of categories) {
      if (c.parentId !== null && out.has(c.parentId) && !out.has(c.id)) {
        out.add(c.id);
        grew = true;
      }
    }
  }
  return out;
}
