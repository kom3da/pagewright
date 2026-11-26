import type { BodyNode } from "../api/types";

export const BODY_SCHEMA_VERSION = 1;

// Allowed nodes/marks and their attributes, from schema/body-v1.md. Tiptap adds a few
// attributes of its own (e.g. orderedList.type, link.rel, cell colwidth); they are dropped.
const nodeAttrs: Record<string, readonly string[]> = {
  doc: [],
  paragraph: [],
  heading: ["level"],
  bulletList: [],
  orderedList: ["start"],
  listItem: [],
  blockquote: [],
  horizontalRule: [],
  image: ["src", "alt", "mediaId", "title"],
  table: [],
  tableRow: [],
  tableHeader: ["colspan", "rowspan"],
  tableCell: ["colspan", "rowspan"],
  embed: ["provider", "url"],
  rawHtml: ["html"],
  contactForm: [],
  hardBreak: [],
  text: [],
};

const markAttrs: Record<string, readonly string[]> = {
  bold: [],
  italic: [],
  code: [],
  link: ["href", "target"],
  ruby: ["rt"],
};

// null is meaningful for image.mediaId (external image); elsewhere it means "not set".
const keepNull = new Set(["image.mediaId"]);

function pickAttrs(kind: string, allowed: readonly string[], attrs?: Record<string, unknown>) {
  if (!attrs) return undefined;
  const out: Record<string, unknown> = {};
  for (const key of allowed) {
    const v = attrs[key];
    if (v === undefined) continue;
    if (v === null && !keepNull.has(`${kind}.${key}`)) continue;
    out[key] = v;
  }
  if (kind === "orderedList" && out.start === 1) delete out.start;
  if (kind === "link" && out.target !== "_blank") delete out.target;
  return Object.keys(out).length > 0 ? out : undefined;
}

/** Converts editor JSON into body JSON v1, dropping anything the schema does not define. */
export function normalizeBody(node: BodyNode): BodyNode {
  const allowed = nodeAttrs[node.type];
  const out: BodyNode = { type: node.type };
  const attrs = pickAttrs(node.type, allowed ?? [], node.attrs);
  if (attrs) out.attrs = attrs;
  if (node.type === "text") out.text = node.text ?? "";
  if (node.marks) {
    const marks = node.marks
      .filter((m) => m.type in markAttrs)
      .map((m) => {
        const a = pickAttrs(m.type, markAttrs[m.type], m.attrs);
        return a ? { type: m.type, attrs: a } : { type: m.type };
      });
    if (marks.length > 0) out.marks = marks;
  }
  if (node.content) {
    out.content = node.content.filter((c) => c.type in nodeAttrs).map(normalizeBody);
  }
  return out;
}

/** Content to load into the editor. A new (empty) revision may have an empty body. */
export function editorContent(body: BodyNode | null | undefined): BodyNode | undefined {
  if (body?.type !== "doc" || !body.content || body.content.length === 0) {
    return undefined;
  }
  return body;
}
