import { zodResolver } from "@hookform/resolvers/zod";
import {
  Box,
  Button,
  Code,
  Group,
  NativeSelect,
  Paper,
  Stack,
  Text,
  Textarea,
  TextInput,
} from "@mantine/core";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useBlocker, useNavigate } from "@tanstack/react-router";
import { type KeyboardEvent, type Ref, useEffect, useRef, useState } from "react";
import { Controller, useForm } from "react-hook-form";
import { z } from "zod";
import { api, errorField, errorMessage, isApiError } from "../api/client";
import { contentQuery, contentsQuery } from "../api/queries";
import type {
  ContentDetail,
  ContentPatch,
  ContentSummary,
  ContentType,
  PreviewLink,
} from "../api/types";
import { CategoryCheckboxes } from "../components/CategoryCheckboxes";
import { PreviewLinksDialog } from "../components/PreviewLinksDialog";
import { ScheduleDialog, type ScheduleError } from "../components/ScheduleDialog";
import { TagPicker } from "../components/TagPicker";
import { confirm } from "../components/ui/confirm";
import { Dialog } from "../components/ui/Dialog";
import { ExternalLink } from "../components/ui/ExternalLink";
import { ErrorMessage, Loading } from "../components/ui/feedback";
import { notify } from "../components/ui/notify";
import { PageHeader } from "../components/ui/PageHeader";
import { ContentStatusBadges } from "../components/ui/StatusBadge";
import { publicUrl } from "../config";
import { BodyEditor, useBodyEditor } from "../editor/BodyEditor";
import { editorContent, normalizeBody } from "../editor/normalize";
import { formatDateTime, formatJapanDateTime } from "../lib/datetime";
import { descendantPageIds, pageRows } from "../lib/pageTree";
import { optionalSlugSchema } from "../lib/slug";
import classes from "./EditorPage.module.css";

const typeLabels = { post: "記事", page: "固定ページ" } as const;
const listCrumb = (type: ContentType) =>
  type === "post"
    ? { label: "記事の一覧", to: "/posts" as const }
    : { label: "固定ページの一覧", to: "/pages" as const };

export function EditorPage({ type, id }: { type: ContentType; id: number }) {
  const { data, isPending, error } = useQuery(contentQuery(id));
  if (isPending) return <Loading />;
  if (error) {
    return (
      <>
        <PageHeader title={`${typeLabels[type]}の編集`} breadcrumbs={[listCrumb(type)]} />
        <ErrorMessage>{errorMessage(error)}</ErrorMessage>
      </>
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

type Busy =
  | "save"
  | "publish"
  | "unpublish"
  | "schedule"
  | "unschedule"
  | "delete"
  | "preview"
  | null;

function EditorForm({ content: initial }: { content: ContentDetail }) {
  const { id, type } = initial;
  const label = typeLabels[type];
  const queryClient = useQueryClient();
  const navigate = useNavigate();
  // Latest server state (status, URL…). Editable values live in local state below.
  const content = useQuery({ ...contentQuery(id), initialData: initial }).data;

  const [title, setTitle] = useState(initial.revision.title);
  const [savedTitle, setSavedTitle] = useState(initial.revision.title);
  const [titleError, setTitleError] = useState<string>();
  const titleRef = useRef<HTMLInputElement>(null);

  // The revision this editor's content is based on. Sent with every draft save so the
  // server can tell when someone else saved in between (409 edit_conflict).
  const baseRevisionId = useRef(initial.revision.id);
  const [conflict, setConflict] = useState(false);

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
  const [scheduleOpen, setScheduleOpen] = useState(false);
  const [scheduleError, setScheduleError] = useState<ScheduleError | null>(null);
  const [previewOpen, setPreviewOpen] = useState(false);
  const previewButtonRef = useRef<HTMLButtonElement>(null);
  // Set when saving before creating a preview link failed: focus then goes to the problem.
  const previewSaveFailed = useRef(false);

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

  const showSettingsError = (name: "slug" | "parentId", message?: string) => {
    if (message) form.setError(name, { message });
    setDetailsOpen(true);
    setTimeout(() => form.setFocus(name));
  };

  /** Shows an API error at the input it is about. Returns false when it is not about one. */
  const showFieldError = (e: unknown) => {
    const field = errorField(e);
    if (!isApiError(e) || !field) return false;
    if (field === "title") {
      setTitleError(e.message);
      titleRef.current?.focus();
      return true;
    }
    if (field === "slug" || field === "parentId") {
      showSettingsError(field, e.message);
      return true;
    }
    return false;
  };

  const update = (detail: ContentDetail) => {
    queryClient.setQueryData(contentQuery(id).queryKey, detail);
    void queryClient.invalidateQueries({ queryKey: contentsQuery(type).queryKey });
  };

  /** Saves settings and the draft if they changed. Returns null when input is invalid. */
  const persist = async (): Promise<ContentDetail | null> => {
    if (!(await form.trigger())) {
      showSettingsError("slug");
      return null;
    }
    let detail = content;
    if (settingsDirty) {
      try {
        detail = await api.patchContent(id, toPatch(form.getValues()));
      } catch (e) {
        if (showFieldError(e)) return null;
        throw e;
      }
      form.reset(settingsFrom(detail));
    }
    if (titleDirty || bodyDirty) {
      const sentTitle = title;
      const sentChanges = bodyChanges.current;
      try {
        // `fields` is not edited here; leaving it out keeps the latest revision's fields.
        detail = await api.saveDraft(id, {
          title: sentTitle,
          body: normalizeBody(editor.getJSON()),
          baseRevisionId: baseRevisionId.current,
        });
      } catch (e) {
        if (isApiError(e, 409, "edit_conflict")) {
          setConflict(true);
          return null;
        }
        if (showFieldError(e)) return null;
        throw e;
      }
      baseRevisionId.current = detail.revision.id;
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
      if (!requireTitle("公開するにはタイトルを入力してください。")) return;
      // The server cancels a pending schedule when publishing now.
      const hadSchedule = content.scheduledAt !== null;
      if (!(await persist())) return;
      update(await api.publish(id));
      notify(
        `公開しました。${hadSchedule ? "予約は取り消しました。" : ""}公開サイトに反映されるまで少しお待ちください。`,
      );
    });

  const requireTitle = (message: string) => {
    if (title.trim()) return true;
    setTitleError(message);
    titleRef.current?.focus();
    return false;
  };

  const openSchedule = () => {
    if (!requireTitle("予約するにはタイトルを入力してください。")) return;
    setScheduleError(null);
    setScheduleOpen(true);
  };

  const schedule = (publishAt: string) =>
    run("schedule", async () => {
      setScheduleError(null);
      try {
        // Save first so that what the user sees is what gets scheduled.
        if (!(await persist())) {
          setScheduleOpen(false);
          return;
        }
        update(await api.schedule(id, publishAt));
      } catch (e) {
        setScheduleError({ message: errorMessage(e), field: errorField(e) === "publishAt" });
        return;
      }
      setScheduleOpen(false);
      notify(`${formatJapanDateTime(publishAt)}に公開するよう予約しました。`);
    });

  // The preview shows the latest saved revision, so unsaved changes are saved first:
  // otherwise the person receiving the link would not see what the editor shows.
  const createPreviewLink = async (days: number): Promise<PreviewLink | null> => {
    if (busy) return null;
    setBusy("preview");
    try {
      if (dirty && !(await persist())) {
        previewSaveFailed.current = true;
        setPreviewOpen(false);
        return null;
      }
      return await api.createPreviewLink(id, days);
    } finally {
      setBusy(null);
    }
  };

  const afterPreviewClose = () => {
    if (!previewSaveFailed.current) {
      previewButtonRef.current?.focus();
      return;
    }
    previewSaveFailed.current = false;
    // Settings errors and the conflict dialog take focus themselves.
    if (titleRef.current?.getAttribute("aria-invalid") === "true") titleRef.current.focus();
  };

  const unschedule = () =>
    run("unschedule", async () => {
      update(await api.unschedule(id));
      notify("予約を取り消しました。");
    });

  const unpublish = async () => {
    const ok = await confirm({
      title: "公開をやめますか？",
      message: content.scheduledAt
        ? `この${label}は公開サイトから見えなくなります。${formatJapanDateTime(content.scheduledAt)}の更新の予約も取り消されます。内容は下書きとして残ります。`
        : `この${label}は公開サイトから見えなくなります。内容は下書きとして残ります。`,
      confirmLabel: "公開をやめる",
    });
    if (!ok) return;
    await run("unpublish", async () => {
      update(await api.unpublish(id));
      notify("公開をやめました。");
    });
  };

  const remove = async () => {
    const ok = await confirm({
      title: `この${label}を削除しますか？`,
      message: `「${title || "（タイトルなし）"}」を削除します。元に戻せません。`,
      confirmLabel: "削除する",
      danger: true,
    });
    if (!ok) return;
    await run("delete", async () => {
      await api.deleteContent(id);
      leaving.current = true;
      queryClient.removeQueries({ queryKey: contentQuery(id).queryKey });
      void queryClient.invalidateQueries({ queryKey: contentsQuery(type).queryKey });
      notify(`${label}を削除しました。`);
      await navigate({ to: type === "post" ? "/posts" : "/pages" });
    });
  };

  // Leaving with unsaved changes (in-app navigation): ask first.
  const blocked = blocker.status === "blocked";
  useEffect(() => {
    if (!blocked) return;
    void confirm({
      title: "保存していない変更があります",
      message: "このまま移動すると、保存していない変更は失われます。",
      cancelLabel: "このページにとどまる",
      confirmLabel: "変更を捨てて移動する",
      danger: true,
    }).then((ok) => (ok ? blocker.proceed?.() : blocker.reset?.()));
  }, [blocked, blocker]);

  // Edit conflict: either discard local edits and load the latest revision, or save the
  // local edits on top of it.
  const loadLatest = () =>
    run("save", async () => {
      const latest = await api.getContent(id);
      baseRevisionId.current = latest.revision.id;
      setTitle(latest.revision.title);
      setSavedTitle(latest.revision.title);
      setTitleError(undefined);
      editor.commands.setContent(editorContent(latest.revision.body) ?? "", {
        emitUpdate: false,
      });
      setBodyDirty(false);
      form.reset(settingsFrom(latest));
      update(latest);
      setConflict(false);
      notify("最新の内容を読み込みました。");
    });

  const overwrite = () =>
    run("save", async () => {
      const latest = await api.getContent(id);
      baseRevisionId.current = latest.revision.id;
      setConflict(false);
      if (await persist()) notify("自分の内容で上書きして保存しました。");
    });

  const onKeyDown = (e: KeyboardEvent) => {
    if ((e.ctrlKey || e.metaKey) && e.key === "s") {
      e.preventDefault();
      void save();
    }
  };

  const isPublic = content.status === "published";
  // Saved after scheduling: the schedule still points at an older revision.
  const scheduleOutdated =
    content.scheduledRevisionId !== null && content.scheduledRevisionId !== content.revision.id;
  const slug = form.watch("slug");
  const parentId = form.watch("parentId");
  const pages = useQuery({ ...contentsQuery("page"), enabled: type === "page" }).data ?? [];

  return (
    <Stack gap="lg" onKeyDown={onKeyDown}>
      <PageHeader
        title={`${label}の編集`}
        breadcrumbs={[listCrumb(type)]}
        meta={
          <>
            <ContentStatusBadges {...content} revisionId={content.revision.id} />
            {content.url && (
              <ExternalLink href={publicUrl(content.url)} size="sm">
                公開中のページを見る
              </ExternalLink>
            )}
            <Text size="sm" c="dimmed">
              最終更新：{formatDateTime(content.updatedAt)}
            </Text>
          </>
        }
      />

      <TextInput
        label="タイトル"
        ref={titleRef}
        value={title}
        onChange={(e) => {
          setTitle(e.target.value);
          setTitleError(undefined);
        }}
        error={titleError}
        size="lg"
        classNames={{ input: classes.titleInput }}
      />

      <BodyEditor editor={editor} />

      <details
        open={detailsOpen}
        onToggle={(e) => setDetailsOpen(e.currentTarget.open)}
        className={classes.details}
      >
        <summary className={classes.summary}>詳細設定</summary>
        <Stack gap="lg" p="md" className={classes.detailsBody}>
          <TextInput
            label="ページのURL（英数字）"
            description={
              <>
                英小文字・数字・ハイフン（-）で入力します。例：company-profile
                <br />
                空のままなら番号で決まります。URL：
                <Code>{urlPreview(type, slug || String(content.number), parentId, pages)}</Code>
              </>
            }
            error={form.formState.errors.slug?.message}
            {...form.register("slug")}
            autoCapitalize="none"
            autoComplete="off"
            spellCheck={false}
          />
          {isPublic && (
            <Text size="sm" c="dimmed">
              公開中にURLを変えると、古いURLは新しいURLへ自動で転送されます。
            </Text>
          )}
          {type === "page" && (
            <Controller
              control={form.control}
              name="parentId"
              render={({ field, fieldState }) => (
                <ParentSelect
                  ref={field.ref}
                  pages={pages}
                  self={id}
                  value={field.value}
                  onChange={field.onChange}
                  error={fieldState.error?.message}
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
        </Stack>
      </details>

      <Box className={classes.actionBar}>
        <Stack gap="sm">
          {actionError && <ErrorMessage>{actionError}</ErrorMessage>}
          {content.scheduledAt && (
            <Paper withBorder bg="orange.0" px="sm" py="xs">
              <Group justify="space-between" gap="sm">
                <div>
                  <Text fw={700} c="orange.9">
                    {isPublic ? "更新の予約" : "予約"}：{formatJapanDateTime(content.scheduledAt)}
                    に公開
                  </Text>
                  {scheduleOutdated && (
                    <Text size="sm">
                      予約したあとに保存した変更は、この予約に含まれていません。含めるには予約し直してください。
                    </Text>
                  )}
                </div>
                <Button
                  size="sm"
                  onClick={unschedule}
                  disabled={busy !== null}
                  loading={busy === "unschedule"}
                >
                  予約を取り消す
                </Button>
              </Group>
            </Paper>
          )}
          <Group gap="sm">
            <Button onClick={save} disabled={busy !== null} loading={busy === "save"}>
              下書き保存
            </Button>
            <Button
              variant="filled"
              onClick={publish}
              disabled={busy !== null}
              loading={busy === "publish"}
            >
              {content.status === "published" ? "変更を公開する" : "公開する"}
            </Button>
            <Button onClick={openSchedule} disabled={busy !== null}>
              {content.scheduledAt ? "予約し直す" : "日時を指定して公開"}
            </Button>
            <Button
              ref={previewButtonRef}
              onClick={() => setPreviewOpen(true)}
              disabled={busy !== null}
            >
              プレビュー用のリンク
            </Button>
            {isPublic && (
              <Button onClick={unpublish} disabled={busy !== null}>
                公開をやめる
              </Button>
            )}
            <Text size="sm" c="dimmed" aria-live="polite">
              {dirty ? "保存していない変更があります" : ""}
            </Text>
            <Button
              variant="subtle"
              color="red"
              ml="auto"
              onClick={remove}
              disabled={busy !== null}
            >
              削除
            </Button>
          </Group>
        </Stack>
      </Box>

      <ScheduleDialog
        open={scheduleOpen}
        onOpenChange={setScheduleOpen}
        scheduledAt={content.scheduledAt}
        busy={busy === "schedule"}
        error={scheduleError}
        onSubmit={schedule}
      />
      <PreviewLinksDialog
        open={previewOpen}
        onOpenChange={setPreviewOpen}
        contentId={id}
        label={label}
        dirty={dirty}
        onCreate={createPreviewLink}
        onCloseFocus={afterPreviewClose}
      />
      <ConflictDialog
        open={conflict}
        onClose={() => setConflict(false)}
        label={label}
        myText={conflict ? `${title}\n\n${editor.getText({ blockSeparator: "\n" })}` : ""}
        busy={busy !== null}
        onLoadLatest={loadLatest}
        onOverwrite={overwrite}
      />
    </Stack>
  );
}

function ConflictDialog({
  open,
  onClose,
  label,
  myText,
  busy,
  onLoadLatest,
  onOverwrite,
}: {
  open: boolean;
  onClose: () => void;
  label: string;
  myText: string;
  busy: boolean;
  onLoadLatest: () => void;
  onOverwrite: () => void;
}) {
  return (
    <Dialog
      open={open}
      onOpenChange={(o) => !o && onClose()}
      title="ほかの人が先に保存しました"
      description={`この${label}を編集しているあいだに、別の画面やほかの人が新しい内容を保存しました。どちらの内容を残すか選んでください。`}
      wide
    >
      <Stack gap="lg">
        <Textarea
          label="あなたの編集内容"
          description="まだ保存されていません。念のため、コピーして控えておくこともできます。"
          readOnly
          autosize={false}
          rows={8}
          value={myText}
        />
        <Stack gap={4} align="flex-start">
          <Button variant="filled" onClick={onOverwrite} disabled={busy}>
            自分の内容で上書きする
          </Button>
          <Text size="sm" c="dimmed">
            ほかの人が保存した内容は、あなたの編集内容に置き換わります。
          </Text>
        </Stack>
        <Stack gap={4} align="flex-start">
          <Button variant="outline" color="red" onClick={onLoadLatest} disabled={busy}>
            最新の内容を読み込む
          </Button>
          <Text size="sm" fw={700} c="red.7">
            注意：あなたの編集内容は捨てられ、元に戻せません。
          </Text>
        </Stack>
        <Group justify="flex-end">
          <Button onClick={onClose}>閉じて編集を続ける</Button>
        </Group>
      </Stack>
    </Dialog>
  );
}

function urlPreview(type: ContentType, last: string, parentId: string, pages: ContentSummary[]) {
  if (type === "post") return `/posts/${last}/`;
  const parent = pages.find((p) => String(p.id) === parentId);
  return parent ? `/${parent.slug ?? parent.number}/${last}/` : `/${last}/`;
}

function ParentSelect({
  ref,
  pages,
  self,
  value,
  onChange,
  error,
}: {
  ref: Ref<HTMLSelectElement>;
  pages: ContentSummary[];
  /** The page being edited: it and its descendants cannot be its parent. */
  self: number;
  value: string;
  onChange: (value: string) => void;
  error?: string;
}) {
  const excluded = descendantPageIds(pages, self);
  const choices = pageRows(pages).filter((p) => !excluded.has(p.id));
  return (
    <NativeSelect
      ref={ref}
      label="親ページ"
      description="選ぶと、URLが「/親ページのURL/このページのURL/」になります。"
      error={error}
      value={value}
      onChange={(e) => onChange(e.currentTarget.value)}
    >
      <option value="">なし（いちばん上の階層）</option>
      {choices.map((p) => (
        <option key={p.id} value={String(p.id)}>
          {"　".repeat(p.depth)}
          {p.title || "（タイトルなし）"}
        </option>
      ))}
    </NativeSelect>
  );
}
