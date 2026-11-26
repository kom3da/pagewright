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
  scheduledAt: string | null;
  parentId: number | null;
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
  categoryIds: number[];
  tagIds: number[];
  revision: Revision;
  publishedRevisionId: number | null;
  scheduledRevisionId: number | null;
};

export type DraftInput = {
  title: string;
  body: BodyNode;
  fields?: Record<string, unknown>;
  /** The revision.id editing started from. A newer latest revision means 409 edit_conflict. */
  baseRevisionId?: number;
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
export type TagPatch = Partial<TagInput>;

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

/** A menu entry: exactly one of contentId and url is set. A null label uses the content's title. */
export type MenuItem = { label: string | null; contentId: number | null; url: string | null };

export type SiteFont = "gothic" | "mincho";

export type SiteSettings = {
  name: string;
  logoMediaId: number | null;
  /** Output only. */
  logo: Media | null;
  /** "#RRGGBB"; empty or missing means the theme's default. */
  primaryColor?: string;
  footerColor?: string;
  font?: SiteFont;
  timeZone?: string;
  /** Empty or missing sends contact messages to every admin. */
  contactEmail?: string;
  /** Empty: top-level pages and お知らせ are listed automatically. */
  menu: MenuItem[];
  /** Name of the theme; missing means the server's default theme. */
  theme?: string;
  /** Page shown at the top of the home page. Missing: none. */
  frontPageId?: number;
  /** Page about handling personal data, linked from contact forms. Missing: none. */
  privacyPageId?: number;
  /** Theme name → declared setting key → value (page settings hold a page id). */
  themeSettings?: Record<string, Record<string, string | number>>;
  /** Output only: the installed themes and the settings each declares. */
  themes: ThemeInfo[];
};

export type ThemeSettingType = "text" | "tel" | "url" | "page";

/** A setting a theme declares in its theme.json (build.SettingDecl). */
export type ThemeSettingDecl = {
  key: string;
  label: string;
  /** Optional help text; may be empty. */
  description: string;
  type: ThemeSettingType;
  /** Limit for text values; 0 means the default (100). */
  maxLength: number;
};

export type ThemeInfo = {
  name: string;
  title: string;
  description: string;
  settings: ThemeSettingDecl[];
};

/** PUT /api/site replaces everything. `logo` and `themes` must not be sent (the server rejects unknown keys). */
export type SiteSettingsInput = Omit<SiteSettings, "logo" | "themes">;

export type Submission = {
  id: number;
  name: string;
  kana: string;
  email: string;
  phone: string;
  message: string;
  pagePath: string;
  createdAt: string;
};

/** A login-free link to the latest revision. `url` is returned only when created. */
export type PreviewLink = { id: number; url?: string; expiresAt: string; createdAt: string };

/** Filters of GET /api/contents. `q` matches the title or the URL (英数字). */
export type ContentFilter = { q?: string; status?: ContentStatus };

/** One export of the public site (GET /api/site/builds). */
export type Build = {
  id: number;
  startedAt: string;
  finishedAt: string;
  /** All files, files that changed, files that were removed. */
  files: number;
  changed: number;
  removed: number;
  /** Notes for the user, such as posts that could not be written. */
  warnings: string[];
  /** Only when the build failed. */
  error: string | null;
};

export type ApiErrorBody = { error: { code: string; message: string; field?: string } };
