// Types mirroring docs/dev/admin-api.md. Keep in sync with the contract.

export type Role = "admin" | "editor";

export type Me = {
  user: { id: string; email: string; displayName: string };
  sites: { id: string; name: string; role: Role }[];
  currentSiteId: string | null;
};

export type ContentType = "post" | "page";
export type ContentStatus = "draft" | "scheduled" | "published";

export type ContentSummary = {
  id: number;
  number: number;
  type: ContentType;
  title: string;
  slug: string | null;
  status: ContentStatus;
  hasUnpublishedChanges: boolean;
  url: string | null;
  publishedAt: string | null;
  updatedAt: string;
};

/** ProseMirror JSON as described in schema/body-v1.md. */
export type BodyNode = {
  type: string;
  attrs?: Record<string, unknown>;
  content?: BodyNode[];
  marks?: { type: string; attrs?: Record<string, unknown> }[];
  text?: string;
};

export type Revision = {
  id: number;
  title: string;
  body: BodyNode;
  bodySchemaVersion: number;
  fields: Record<string, unknown>;
  createdAt: string;
};

export type ContentDetail = ContentSummary & {
  parentId: number | null;
  categoryIds: number[];
  tagIds: number[];
  revision: Revision;
  publishedRevisionId: number | null;
};

export type DraftInput = {
  title: string;
  body: BodyNode;
  fields?: Record<string, unknown>;
};

export type ContentPatch = {
  slug?: string | null;
  parentId?: number | null;
  categoryIds?: number[];
  tagIds?: number[];
};

export type Category = {
  id: number;
  name: string;
  slug: string;
  parentId: number | null;
  position: number;
};

export type CategoryInput = { name: string; slug: string; parentId?: number | null };
export type CategoryPatch = Partial<CategoryInput & { position: number }>;

export type Tag = { id: number; name: string; slug: string };
export type TagInput = { name: string; slug: string };

export type Media = {
  id: number;
  url: string;
  previewUrl: string;
  filename: string;
  contentType: string;
  sizeBytes: number;
  width: number | null;
  height: number | null;
  altText: string;
  createdAt: string;
};

export type ApiErrorBody = { error: { code: string; message: string } };
