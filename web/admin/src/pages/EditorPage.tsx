import { zodResolver } from "@hookform/resolvers/zod";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { Link, useBlocker, useNavigate } from "@tanstack/react-router";
import { ArrowLeft, ExternalLink } from "lucide-react";
import { type KeyboardEvent, useRef, useState } from "react";
import { Controller, useForm } from "react-hook-form";
import { z } from "zod";
import { api, errorMessage, isApiError } from "../api/client";
import { contentQuery, contentsQuery } from "../api/queries";
import type { ContentDetail, ContentPatch, ContentSummary, ContentType } from "../api/types";
import { CategoryCheckboxes } from "../components/CategoryCheckboxes";
import { TagPicker } from "../components/TagPicker";
import { useNotify } from "../components/Toaster";
import { Button } from "../components/ui/Button";
import { ConfirmDialog } from "../components/ui/Dialog";
import { Field, inputClass } from "../components/ui/Field";
import {
  ErrorMessage,
  formatDateTime,
  Loading,
  PageHeading,
  StatusBadge,
  UnpublishedBadge,
} from "../components/ui/misc";
import { publicUrl } from "../config";
import { BodyEditor, useBodyEditor } from "../editor/BodyEditor";
import { normalizeBody } from "../editor/normalize";
import { optionalSlugSchema } from "../lib/slug";

const typeLabels = { post: "記事", page: "固定ページ" } as const;

export function EditorPage({ type, id }: { type: ContentType; id: number }) {
  const { data, isPending, error } = useQuery(contentQuery(id));
  if (isPending) return <Loading />;
  if (error) {
    return (
      <div className="space-y-4">
        <PageHeading title={`${typeLabels[type]}の編集`}>{typeLabels[type]}の編集</PageHeading>
        <ErrorMessage>{errorMessage(error)}</ErrorMessage>
      </div>
    );
  }
  return <EditorForm content={data} />;
}

const settingsSchema = z.object({
  slug: optionalSlugSchema,
  parentId: z.string(),
  categoryIds: z.array(z.number()),
  tagIds: z.array(z.number()),
});
type Settings = z.infer<typeof settingsSchema>;

const settingsFrom = (c: ContentDetail): Settings => ({
  slug: c.slug ?? "",
  parentId: c.parentId === null ? "" : String(c.parentId),
  categoryIds: c.categoryIds,
  tagIds: c.tagIds,
});

const toPatch = (s: Settings): ContentPatch => ({
  slug: s.slug === "" ? null : s.slug,
  parentId: s.parentId === "" ? null : Number(s.parentId),
  categoryIds: s.categoryIds,
  tagIds: s.tagIds,
});

type Busy = "save" | "publish" | "unpublish" | "delete" | null;

function EditorForm({ content: initial }: { content: ContentDetail }) {
  const { id, type } = initial;
  const label = typeLabels[type];
  const queryClient = useQueryClient();
  const navigate = useNavigate();
  const notify = useNotify();
  // Latest server state (status, URL…). Editable values live in local state below.
  const content = useQuery({ ...contentQuery(id), initialData: initial }).data;

  const [title, setTitle] = useState(initial.revision.title);
  const [savedTitle, setSavedTitle] = useState(initial.revision.title);
  const [titleError, setTitleError] = useState<string>();
  const titleRef = useRef<HTMLInputElement>(null);

  const bodyChanges = useRef(0);
  const [bodyDirty, setBodyDirty] = useState(false);
  const editor = useBodyEditor(initial.revision.body, () => {
    bodyChanges.current++;
    setBodyDirty(true);
  });

  const form = useForm<Settings>({
    resolver: zodResolver(settingsSchema),
    mode: "onChange",
    defaultValues: settingsFrom(initial),
  });
  const settingsDirty = form.formState.isDirty;
  const [detailsOpen, setDetailsOpen] = useState(false);

  const [busy, setBusy] = useState<Busy>(null);
  const [actionError, setActionError] = useState<string | null>(null);
  const [confirm, setConfirm] = useState<"unpublish" | "delete" | null>(null);

  const titleDirty = title !== savedTitle;
  const dirty = titleDirty || bodyDirty || settingsDirty;

  // Warn before leaving with unsaved changes (in-app navigation and closing the tab).
  const dirtyRef = useRef(dirty);
  dirtyRef.current = dirty;
  const leaving = useRef(false);
  const blocker = useBlocker({
    shouldBlockFn: () => dirtyRef.current && !leaving.current,
    enableBeforeUnload: () => dirtyRef.current,
    withResolver: true,
  });

  const showSlugError = (message?: string) => {
    if (message) form.setError("slug", { message });
    setDetailsOpen(true);
    setTimeout(() => form.setFocus("slug"));
  };

  const update = (detail: ContentDetail) => {
    queryClient.setQueryData(contentQuery(id).queryKey, detail);
    void queryClient.invalidateQueries({ queryKey: contentsQuery(type).queryKey });
  };

  /** Saves settings and the draft if they changed. Returns null when input is invalid. */
  const persist = async (): Promise<ContentDetail | null> => {
    if (!(await form.trigger())) {
      showSlugError();
      return null;
    }
    let detail = content;
    if (settingsDirty) {
      try {
        detail = await api.patchContent(id, toPatch(form.getValues()));
      } catch (e) {
        if (isApiError(e, 409) || isApiError(e, 422, "invalid_slug")) {
          showSlugError(e.message);
          return null;
        }
        throw e;
      }
      form.reset(settingsFrom(detail));
    }
    if (titleDirty || bodyDirty) {
      const sentTitle = title;
      const sentChanges = bodyChanges.current;
      detail = await api.saveDraft(id, {
        title: sentTitle,
        body: normalizeBody(editor.getJSON()),
        fields: content.revision.fields,
      });
      setSavedTitle(sentTitle);
      // Keep the dirty flag if the user kept typing while saving.
      if (bodyChanges.current === sentChanges) setBodyDirty(false);
    }
    if (detail !== content) update(detail);
    return detail;
  };

  const run = async (kind: Exclude<Busy, null>, action: () => Promise<void>) => {
    if (busy) return;
    setBusy(kind);
    setActionError(null);
    try {
      await action();
    } catch (e) {
      setActionError(errorMessage(e));
    } finally {
      setBusy(null);
    }
  };

  const save = () =>
    run("save", async () => {
      if (!dirty) {
        notify("変更はありません。");
        return;
      }
      if (await persist()) notify("下書きを保存しました。");
    });

  const publish = () =>
    run("publish", async () => {
      if (!title.trim()) {
        setTitleError("公開するにはタイトルを入力してください。");
        titleRef.current?.focus();
        return;
      }
      if (!(await persist())) return;
      update(await api.publish(id));
      notify("公開しました。公開サイトに反映されるまで少しお待ちください。");
    });

  const unpublish = () =>
    run("unpublish", async () => {
      setConfirm(null);
      update(await api.unpublish(id));
      notify("公開をやめました。");
    });

  const remove = () =>
    run("delete", async () => {
      setConfirm(null);
      await api.deleteContent(id);
      leaving.current = true;
      queryClient.removeQueries({ queryKey: contentQuery(id).queryKey });
      void queryClient.invalidateQueries({ queryKey: contentsQuery(type).queryKey });
      notify(`${label}を削除しました。`);
      await navigate({ to: type === "post" ? "/posts" : "/pages" });
    });

  const onKeyDown = (e: KeyboardEvent) => {
    if ((e.ctrlKey || e.metaKey) && e.key === "s") {
      e.preventDefault();
      void save();
    }
  };

  const isPublic = content.status !== "draft";
  const slug = form.watch("slug");
  const parentId = form.watch("parentId");
  const pages = useQuery({ ...contentsQuery("page"), enabled: type === "page" }).data ?? [];

  return (
    // biome-ignore lint/a11y/noStaticElementInteractions: Ctrl+S shortcut for the whole editor area.
    <div className="space-y-6" onKeyDown={onKeyDown}>
      <Link
        to={type === "post" ? "/posts" : "/pages"}
        className="inline-flex items-center gap-1 text-blue-700 underline"
      >
        <ArrowLeft aria-hidden="true" size={16} />
        {label}の一覧へ
      </Link>

      <div className="space-y-2">
        <PageHeading title={`${label}の編集`}>{label}の編集</PageHeading>
        <div className="flex flex-wrap items-center gap-2 text-sm">
          <StatusBadge status={content.status} />
          {content.hasUnpublishedChanges && isPublic && <UnpublishedBadge />}
          {content.url && (
            <a
              href={publicUrl(content.url)}
              target="_blank"
              rel="noopener noreferrer"
              className="inline-flex items-center gap-1 text-blue-700 underline"
            >
              公開中のページを見る
              <ExternalLink aria-hidden="true" size={14} />
              <span className="sr-only">（新しいタブで開きます）</span>
            </a>
          )}
          <span className="text-slate-700">最終更新：{formatDateTime(content.updatedAt)}</span>
        </div>
      </div>

      <Field label="タイトル" error={titleError}>
        {(ids) => (
          <input
            {...ids}
            ref={titleRef}
            value={title}
            onChange={(e) => {
              setTitle(e.target.value);
              if (e.target.value.trim()) setTitleError(undefined);
            }}
            className={`${inputClass} text-xl font-bold`}
          />
        )}
      </Field>

      <BodyEditor editor={editor} />

      <details
        open={detailsOpen}
        onToggle={(e) => setDetailsOpen(e.currentTarget.open)}
        className="rounded-md border border-slate-300 bg-white"
      >
        <summary className="cursor-pointer px-4 py-3 font-semibold">詳細設定</summary>
        <div className="space-y-6 border-t border-slate-300 p-4">
          <Field
            label="ページのURL（英数字）"
            hint={
              <>
                英小文字・数字・ハイフン（-）で入力します。例：company-profile
                <br />
                空のままなら番号で決まります。URL：
                <code>{urlPreview(type, slug || String(content.number), parentId, pages)}</code>
              </>
            }
            error={form.formState.errors.slug?.message}
          >
            {(ids) => (
              <input
                {...ids}
                {...form.register("slug")}
                className={inputClass}
                autoCapitalize="none"
                autoComplete="off"
                spellCheck={false}
              />
            )}
          </Field>
          {isPublic && (
            <p className="text-sm text-slate-700">
              公開中にURLを変えると、古いURLは新しいURLへ自動で転送されます。
            </p>
          )}
          {type === "page" && (
            <Controller
              control={form.control}
              name="parentId"
              render={({ field }) => (
                <ParentSelect
                  pages={pages.filter((p) => p.id !== id)}
                  value={field.value}
                  onChange={field.onChange}
                />
              )}
            />
          )}
          {type === "post" && (
            <>
              <Controller
                control={form.control}
                name="categoryIds"
                render={({ field }) => (
                  <CategoryCheckboxes value={field.value} onChange={field.onChange} />
                )}
              />
              <Controller
                control={form.control}
                name="tagIds"
                render={({ field }) => <TagPicker value={field.value} onChange={field.onChange} />}
              />
            </>
          )}
        </div>
      </details>

      <div className="sticky bottom-0 z-10 -mx-4 space-y-3 border-t border-slate-300 bg-slate-50/95 px-4 py-3">
        {actionError && <ErrorMessage>{actionError}</ErrorMessage>}
        <div className="flex flex-wrap items-center gap-3">
          <Button variant="secondary" onClick={save} disabled={busy !== null}>
            {busy === "save" ? "保存しています…" : "下書き保存"}
          </Button>
          <Button variant="primary" onClick={publish} disabled={busy !== null}>
            {busy === "publish"
              ? "公開しています…"
              : content.status === "published"
                ? "変更を公開する"
                : "公開する"}
          </Button>
          {isPublic && (
            <Button onClick={() => setConfirm("unpublish")} disabled={busy !== null}>
              公開をやめる
            </Button>
          )}
          <p aria-live="polite" className="text-sm text-slate-700">
            {dirty ? "保存していない変更があります" : ""}
          </p>
          <Button
            variant="danger"
            className="ml-auto"
            onClick={() => setConfirm("delete")}
            disabled={busy !== null}
          >
            削除
          </Button>
        </div>
      </div>

      <ConfirmDialog
        open={confirm === "unpublish"}
        onOpenChange={(o) => !o && setConfirm(null)}
        title="公開をやめますか？"
        description={`この${label}は公開サイトから見えなくなります。内容は下書きとして残ります。`}
        confirmLabel="公開をやめる"
        onConfirm={unpublish}
      />
      <ConfirmDialog
        open={confirm === "delete"}
        onOpenChange={(o) => !o && setConfirm(null)}
        title={`この${label}を削除しますか？`}
        description={`「${title || "（タイトルなし）"}」を削除します。元に戻せません。`}
        confirmLabel="削除する"
        danger
        onConfirm={remove}
      />
      <ConfirmDialog
        open={blocker.status === "blocked"}
        onOpenChange={(o) => !o && blocker.reset?.()}
        title="保存していない変更があります"
        description="このまま移動すると、保存していない変更は失われます。"
        cancelLabel="このページにとどまる"
        confirmLabel="変更を捨てて移動する"
        danger
        onConfirm={() => blocker.proceed?.()}
      />
    </div>
  );
}

function urlPreview(type: ContentType, last: string, parentId: string, pages: ContentSummary[]) {
  if (type === "post") return `/posts/${last}/`;
  const parent = pages.find((p) => String(p.id) === parentId);
  return parent ? `/${parent.slug ?? parent.number}/${last}/` : `/${last}/`;
}

function ParentSelect({
  pages,
  value,
  onChange,
}: {
  pages: ContentSummary[];
  value: string;
  onChange: (value: string) => void;
}) {
  return (
    <Field label="親ページ" hint="選ぶと、URLが「/親ページのURL/このページのURL/」になります。">
      {(ids) => (
        <select
          {...ids}
          value={value}
          onChange={(e) => onChange(e.target.value)}
          className={inputClass}
        >
          <option value="">なし（いちばん上の階層）</option>
          {pages.map((p) => (
            <option key={p.id} value={String(p.id)}>
              {p.title || "（タイトルなし）"}
            </option>
          ))}
        </select>
      )}
    </Field>
  );
}
