import { zodResolver } from "@hookform/resolvers/zod";
import {
  Box,
  Button,
  Code,
  Group,
  Menu,
  NativeSelect,
  Paper,
  Stack,
  Text,
  Textarea,
  TextInput,
  VisuallyHidden,
} from "@mantine/core";
import { useMediaQuery } from "@mantine/hooks";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useBlocker, useNavigate } from "@tanstack/react-router";
import {
  CalendarClock,
  ChevronDown,
  ExternalLink as ExternalLinkIcon,
  EyeOff,
  Link2,
  SlidersHorizontal,
  Trash2,
} from "lucide-react";
import {
  type CSSProperties,
  type KeyboardEvent,
  type Ref,
  useEffect,
  useId,
  useRef,
  useState,
} from "react";
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
import { DateTimeFields } from "../components/DateTimeFields";
import { PreviewLinksDialog } from "../components/PreviewLinksDialog";
import { ScheduleDialog, type ScheduleError } from "../components/ScheduleDialog";
import { TagPicker } from "../components/TagPicker";
import { confirm } from "../components/ui/confirm";
import { Dialog } from "../components/ui/Dialog";
import { ExternalLink } from "../components/ui/ExternalLink";
import { ErrorMessage, Loading } from "../components/ui/feedback";
import { notify } from "../components/ui/notify";
import { PageHeader } from "../components/ui/PageHeader";
import { Section } from "../components/ui/Section";
import { ContentStatusBadges } from "../components/ui/StatusBadge";
import { BodyEditor, useBodyEditor } from "../editor/BodyEditor";
import { editorContent, normalizeBody, plainText } from "../editor/normalize";
import {
  formatDateTime,
  formatJapanDateTime,
  fromJapanInputs,
  toJapanInputs,
} from "../lib/datetime";
import { descendantPageIds, pageRows } from "../lib/pageTree";
import { optionalSlugSchema } from "../lib/slug";
import { usePublicUrl } from "../lib/usePublicUrl";
import { DESKTOP_QUERY } from "../theme";
import classes from "./EditorPage.module.css";

/** From here the settings sit in a column beside the body (Mantine's `lg`). */
const WIDE_QUERY = "(min-width: 75em)";

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

/** The date shown on the site. It can be moved to any past time; the future is scheduling. */
const publishedSchema = z.object({ date: z.string(), time: z.string() }).superRefine((v, ctx) => {
  // Empty for contents that were never published (the field is not shown then).
  if (!v.date && !v.time) return;
  const at = fromJapanInputs(v.date, v.time);
  if (!at) {
    ctx.addIssue({ code: "custom", message: "公開日の日付と時刻を入力してください。" });
  } else if (new Date(at).getTime() > Date.now()) {
    ctx.addIssue({
      code: "custom",
      message:
        "公開日は今より前の日時にしてください。先の日時に変更を公開するときは「変更の公開を予約する」を使ってください。",
    });
  }
});

const settingsSchema = z.object({
  slug: optionalSlugSchema,
  parentId: z.string(),
  categoryIds: z.array(z.number()),
  tagIds: z.array(z.number()),
  published: publishedSchema,
});
type Settings = z.infer<typeof settingsSchema>;
/** Settings fields in screen order: the first invalid one gets focus. */
const settingsFields = ["published", "slug", "parentId"] as const;

const settingsFrom = (c: ContentDetail): Settings => ({
  slug: c.slug ?? "",
  parentId: c.parentId === null ? "" : String(c.parentId),
  categoryIds: c.categoryIds,
  tagIds: c.tagIds,
  published: c.publishedAt ? toJapanInputs(c.publishedAt) : { date: "", time: "" },
});

const toPatch = (s: Settings, publishedChanged: boolean): ContentPatch => {
  const publishedAt = publishedChanged ? fromJapanInputs(s.published.date, s.published.time) : null;
  return {
    slug: s.slug === "" ? null : s.slug,
    parentId: s.parentId === "" ? null : Number(s.parentId),
    categoryIds: s.categoryIds,
    tagIds: s.tagIds,
    ...(publishedAt && { publishedAt }),
  };
};

type Busy =
  | "save"
  | "publish"
  | "unpublish"
  | "schedule"
  | "unschedule"
  | "delete"
  | "preview"
  | "view"
  | null;

function EditorForm({ content: initial }: { content: ContentDetail }) {
  const publicUrl = usePublicUrl();
  const { id, type } = initial;
  const label = typeLabels[type];
  const queryClient = useQueryClient();
  const navigate = useNavigate();
  // Latest server state (status, URL…). Editable values live in local state below.
  const content = useQuery({ ...contentQuery(id), initialData: initial }).data;

  const [title, setTitle] = useState(initial.revision.title);
  const [savedTitle, setSavedTitle] = useState(initial.revision.title);
  const [titleError, setTitleError] = useState<string>();
  const titleRef = useRef<HTMLTextAreaElement>(null);

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

  const [busy, setBusy] = useState<Busy>(null);
  const [actionError, setActionError] = useState<string | null>(null);
  const [scheduleOpen, setScheduleOpen] = useState(false);
  const [scheduleError, setScheduleError] = useState<ScheduleError | null>(null);
  const [previewOpen, setPreviewOpen] = useState(false);
  const previewButtonRef = useRef<HTMLButtonElement>(null);
  // Set when saving before creating a preview link failed: focus then goes to the problem.
  const previewSaveFailed = useRef(false);

  // The action bar is sticky at the top on desktop and fixed at the bottom on phones. Its
  // height keeps the body toolbar below it (desktop) and the page end above it (phones).
  const desktop = useMediaQuery(DESKTOP_QUERY, true, { getInitialValueInEffect: false });
  const wide = useMediaQuery(WIDE_QUERY, true, { getInitialValueInEffect: false });
  const barRef = useRef<HTMLDivElement>(null);
  const [barHeight, setBarHeight] = useState(0);
  useEffect(() => {
    const bar = barRef.current;
    if (!bar) return;
    const measure = () => setBarHeight(bar.offsetHeight);
    measure();
    const observer = new ResizeObserver(measure);
    observer.observe(bar);
    return () => observer.disconnect();
  }, []);
  // Items of the 「その他」 menu disappear when chosen: focus returns to the menu button.
  const moreButtonRef = useRef<HTMLButtonElement>(null);
  const focusMoreButton = () => moreButtonRef.current?.focus();
  // The schedule dialog opens from the 公開 card or from 「その他」: focus goes back there.
  const scheduleButtonRef = useRef<HTMLButtonElement>(null);
  const scheduleOpener = useRef<"card" | "menu">("menu");
  const scheduleSaveFailed = useRef(false);
  const afterScheduleClose = () => {
    if (scheduleSaveFailed.current) {
      scheduleSaveFailed.current = false;
      return;
    }
    if (scheduleOpener.current === "card" && scheduleButtonRef.current) {
      scheduleButtonRef.current.focus();
      return;
    }
    focusMoreButton();
  };
  const settingsHeadingId = `${useId()}-settings`;

  const isPublic = content.status === "published";
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

  const showSettingsError = (name: (typeof settingsFields)[number], message?: string) => {
    if (message) form.setError(name, { message });
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
    if (field === "publishedAt") {
      showSettingsError("published", e.message);
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
      const errors = form.formState.errors;
      showSettingsError(settingsFields.find((f) => errors[f]) ?? "slug");
      return null;
    }
    let detail = content;
    if (settingsDirty) {
      try {
        const publishedChanged = form.getFieldState("published").isDirty;
        detail = await api.patchContent(id, toPatch(form.getValues(), publishedChanged));
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
      if (!(await persist())) return;
      if (!isPublic) {
        notify("下書きを保存しました。");
        return;
      }
      // Title, body and settings all wait for 「変更を公開する」 (PATCH /api/contents keeps
      // the settings of a published content pending).
      notify("保存しました。「変更を公開する」を押すまで、公開サイトは変わりません。");
    });

  const publish = () =>
    run("publish", async () => {
      if (!requireTitle("公開するにはタイトルを入力してください。")) return;
      // The server cancels a pending schedule when publishing now.
      const hadSchedule = content.scheduledAt !== null;
      if (!(await persist())) return;
      let detail: ContentDetail;
      try {
        detail = await api.publish(id, baseRevisionId.current);
      } catch (e) {
        if (isApiError(e, 409, "edit_conflict")) {
          setConflict(true);
          return;
        }
        throw e;
      }
      update(detail);
      // The first publish sets the date shown on the site: show it like a fresh load does.
      // (Its fields are not mounted yet, so resetField would not reach them.)
      form.reset(
        { ...form.formState.defaultValues, published: settingsFrom(detail).published },
        { keepDirtyValues: true },
      );
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

  const openSchedule = (from: "card" | "menu") => {
    scheduleOpener.current = from;
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
          // The problem (or the conflict dialog) takes focus, not the menu button.
          scheduleSaveFailed.current = true;
          setScheduleOpen(false);
          return;
        }
        update(await api.schedule(id, publishAt, baseRevisionId.current));
      } catch (e) {
        if (isApiError(e, 409, "edit_conflict")) {
          scheduleSaveFailed.current = true;
          setScheduleOpen(false);
          setConflict(true);
          return;
        }
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

  // 「プレビューを見る」: the server draws the latest saved revision at an address only
  // logged-in members can open, so no shareable link is made. Unsaved changes are saved
  // first.
  const viewPreview = () => {
    if (busy) return;
    // Opened now, while the click still counts as the user's: after the await below a
    // browser may block a new tab.
    const tab = window.open("about:blank", "_blank");
    if (tab) tab.opener = null;
    void run("view", async () => {
      if (dirty && !(await persist())) {
        tab?.close();
        return;
      }
      const url = api.previewUrl(id);
      if (tab) tab.location.href = url;
      else window.open(url, "_blank", "noopener");
    });
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
    setTimeout(focusMoreButton);
    if (!ok) return;
    await run("unpublish", async () => {
      update(await api.unpublish(id));
      notify("公開をやめました。");
    });
    // 「その他」 was disabled while working, which lost the focus: give it back.
    setTimeout(focusMoreButton);
  };

  const remove = async () => {
    const ok = await confirm({
      title: `この${label}を削除しますか？`,
      message: `「${title || "（タイトルなし）"}」を削除します。${isPublic ? "公開サイトからも消えます。" : ""}元に戻せません。`,
      confirmLabel: "削除する",
      danger: true,
    });
    if (!ok) {
      setTimeout(focusMoreButton);
      return;
    }
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

  const scheduleLabel = content.scheduledAt
    ? "予約し直す"
    : isPublic
      ? "変更の公開を予約する"
      : "日時を決めて公開する";
  // Saved after scheduling: the schedule still points at an older revision.
  const scheduleOutdated =
    content.scheduledRevisionId !== null && content.scheduledRevisionId !== content.revision.id;
  const slug = form.watch("slug");
  const parentId = form.watch("parentId");
  // All pages (they are few, unlike posts): the parent choice and URL preview need the tree.
  const pages = useQuery({ ...contentsQuery("page"), enabled: type === "page" }).data ?? [];

  // Below the `lg` breakpoint the settings are under the body: 「その他」 offers a jump there.
  const goToSettings = () => {
    const heading = document.getElementById(settingsHeadingId);
    if (!heading) return;
    heading.setAttribute("tabindex", "-1");
    heading.scrollIntoView({ block: "start" });
    // After the menu has returned focus to its button (Mantine does that 10ms after closing).
    setTimeout(() => heading.focus({ preventScroll: true }), 50);
  };

  const actionBar = (
    <Box
      ref={barRef}
      component="section"
      aria-label="保存と公開"
      className={desktop ? classes.barTop : classes.barBottom}
    >
      <Stack gap="xs">
        {actionError && <ErrorMessage>{actionError}</ErrorMessage>}
        <Group gap="xs" justify="flex-end" wrap="wrap">
          <Text size="sm" c="dimmed" aria-live="polite" className={classes.saveState}>
            {dirty ? "保存していない変更があります" : ""}
          </Text>
          {desktop && (
            <Button
              variant="subtle"
              onClick={viewPreview}
              disabled={busy !== null}
              loading={busy === "view"}
              rightSection={<ExternalLinkIcon aria-hidden="true" size={16} />}
            >
              プレビューを見る
              <VisuallyHidden>（新しいタブで開きます）</VisuallyHidden>
            </Button>
          )}
          <Group gap="xs" wrap="nowrap">
            <Button
              size={desktop ? "md" : "sm"}
              onClick={save}
              disabled={busy !== null}
              loading={busy === "save"}
            >
              {isPublic ? (desktop ? "保存（まだ公開しない）" : "保存") : "下書き保存"}
            </Button>
            <Button
              size={desktop ? "md" : "sm"}
              variant="filled"
              onClick={publish}
              disabled={busy !== null}
              loading={busy === "publish"}
            >
              {isPublic ? "変更を公開する" : "公開する"}
            </Button>
            {/* No focus placeholder: a focusable role="presentation" child breaks the menu
                role's required children (axe). Opening with Enter focuses the first item. */}
            <Menu position={desktop ? "bottom-end" : "top-end"} withInitialFocusPlaceholder={false}>
              <Menu.Target>
                <Button
                  ref={moreButtonRef}
                  size={desktop ? "md" : "sm"}
                  disabled={busy !== null}
                  rightSection={<ChevronDown aria-hidden="true" size={16} />}
                >
                  その他
                </Button>
              </Menu.Target>
              <Menu.Dropdown>
                {!desktop && (
                  <Menu.Item
                    leftSection={<ExternalLinkIcon aria-hidden="true" size={16} />}
                    onClick={viewPreview}
                  >
                    プレビューを見る
                    <VisuallyHidden>（新しいタブで開きます）</VisuallyHidden>
                  </Menu.Item>
                )}
                <Menu.Item
                  leftSection={<CalendarClock aria-hidden="true" size={16} />}
                  onClick={() => openSchedule("menu")}
                >
                  {scheduleLabel}
                </Menu.Item>
                {!wide && (
                  <Menu.Item
                    leftSection={<SlidersHorizontal aria-hidden="true" size={16} />}
                    onClick={goToSettings}
                  >
                    公開日・URL・カテゴリなどの設定へ移動
                  </Menu.Item>
                )}
                {isPublic && (
                  <Menu.Item
                    leftSection={<EyeOff aria-hidden="true" size={16} />}
                    onClick={unpublish}
                  >
                    公開をやめる
                  </Menu.Item>
                )}
                <Menu.Divider />
                <Menu.Item
                  color="red"
                  leftSection={<Trash2 aria-hidden="true" size={16} />}
                  onClick={remove}
                >
                  削除する
                </Menu.Item>
              </Menu.Dropdown>
            </Menu>
          </Group>
        </Group>
      </Stack>
    </Box>
  );

  const barVar = (desktop ? { "--pw-editor-bar": `${barHeight}px` } : {}) as CSSProperties;

  return (
    <Box
      onKeyDown={onKeyDown}
      style={barVar}
      // On phones the bar is fixed to the bottom: keep the end of the page above it.
      pb={desktop ? undefined : barHeight}
    >
      <PageHeader
        title={`${label}の編集`}
        breadcrumbs={[listCrumb(type)]}
        meta={
          <>
            <ContentStatusBadges {...content} revisionId={content.revision.id} />
            {content.url && (
              <ExternalLink href={publicUrl(content.url)} size="sm" className="pw-hit-target">
                公開中のページを見る
              </ExternalLink>
            )}
            <Text size="sm" c="dimmed">
              最終更新：{formatDateTime(content.updatedAt)}
            </Text>
          </>
        }
      />
      {actionBar}

      <Box className={classes.layout}>
        <Stack gap="lg" miw={0}>
          {/* Wraps instead of cutting a long title; a title has no line breaks. */}
          <Textarea
            label="タイトル"
            ref={titleRef}
            value={title}
            onChange={(e) => {
              setTitle(e.target.value.replace(/[\r\n]+/g, " "));
              setTitleError(undefined);
            }}
            onKeyDown={(e) => {
              // Enter would add a line break; while an IME converts, Enter is the IME's.
              if (e.key === "Enter" && !e.nativeEvent.isComposing) e.preventDefault();
            }}
            error={titleError}
            size="lg"
            autosize
            minRows={1}
            classNames={{ input: classes.titleInput }}
          />
          <BodyEditor editor={editor} />
        </Stack>

        <Stack gap="md" miw={0}>
          <Section title="公開" id={settingsHeadingId}>
            {content.scheduledAt && (
              <Paper withBorder bg="orange.0" p="sm" className={classes.schedule}>
                <Stack gap="xs" align="flex-start">
                  <Text fw={700} c="orange.9">
                    {isPublic ? "更新の予約" : "予約"}：{formatJapanDateTime(content.scheduledAt)}
                    に公開
                  </Text>
                  {scheduleOutdated && (
                    <Text size="sm">
                      予約したあとに保存した変更は、この予約に含まれていません。含めるには予約し直してください。
                    </Text>
                  )}
                  <Button
                    size="sm"
                    onClick={unschedule}
                    disabled={busy !== null}
                    loading={busy === "unschedule"}
                  >
                    予約を取り消す
                  </Button>
                </Stack>
              </Paper>
            )}
            {isPublic ? (
              <>
                <Text size="sm">
                  {/* With pending settings, publishedAt is the saved date, not the live one. */}
                  {content.publishedAt &&
                    !content.pendingSettings &&
                    `${formatJapanDateTime(content.publishedAt)} に公開しました。`}
                  直したタイトル・本文と、公開日やURLなどの設定は、「変更を公開する」を押すまで公開サイトに出ません。
                </Text>
                <Controller
                  control={form.control}
                  name="published"
                  render={({ field, fieldState }) => (
                    <DateTimeFields
                      legend="公開日（日本時間）"
                      hint="サイトに表示される公開日です。今より前の日時に変えられます。変えた公開日は、「変更を公開する」を押すと公開サイトに出ます。"
                      value={field.value}
                      onChange={field.onChange}
                      error={fieldState.error?.message}
                      dateRef={field.ref}
                      cols={{ base: 1, xs: 2, lg: 1 }}
                      summary={(at) => `サイトには公開日を ${at} と表示します`}
                    />
                  )}
                />
              </>
            ) : (
              !content.scheduledAt && (
                <Text size="sm">
                  まだ公開していません。「公開する」ですぐに公開できます。先の日時に公開するときは「日時を決めて公開する」で予約します。
                </Text>
              )
            )}
            {/* Scheduling is offered where it matters: a draft, or changes not yet public. */}
            {!content.scheduledAt && (!isPublic || content.hasUnpublishedChanges || dirty) && (
              <Group>
                <Button
                  ref={scheduleButtonRef}
                  onClick={() => openSchedule("card")}
                  disabled={busy !== null}
                  leftSection={<CalendarClock aria-hidden="true" size={16} />}
                >
                  {scheduleLabel}
                </Button>
              </Group>
            )}
          </Section>

          <Section
            title="ほかの人に見せる"
            description={
              isPublic
                ? `まだ公開していない変更も含めた、保存した最新の${label}を、ログインしていない人に見てもらうリンクを作れます。`
                : `公開する前の${label}を、ログインしていない人に見てもらうリンクを作れます。`
            }
          >
            <Group>
              <Button
                ref={previewButtonRef}
                onClick={() => setPreviewOpen(true)}
                disabled={busy !== null}
                leftSection={<Link2 aria-hidden="true" size={16} />}
              >
                確認用のリンクを作る
              </Button>
            </Group>
          </Section>

          <Section title="URL">
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
              classNames={{ description: classes.slugDescription }}
              error={form.formState.errors.slug?.message}
              {...form.register("slug")}
              autoCapitalize="none"
              autoComplete="off"
              spellCheck={false}
            />
            {isPublic && (
              <Text size="sm" c="dimmed">
                公開中にURLを変えると、「変更を公開する」を押したときに新しいURLになり、古いURLは新しいURLへ自動で転送されます。
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
          </Section>

          {type === "post" && (
            <Section title="カテゴリとタグ">
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
            </Section>
          )}
        </Stack>
      </Box>

      <ScheduleDialog
        title={
          content.scheduledAt
            ? "予約し直す"
            : isPublic
              ? "変更の公開を予約する"
              : "日時を決めて公開する"
        }
        open={scheduleOpen}
        onOpenChange={setScheduleOpen}
        scheduledAt={content.scheduledAt}
        busy={busy === "schedule"}
        error={scheduleError}
        onSubmit={schedule}
        onCloseFocus={afterScheduleClose}
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
        myText={conflict ? `${title}\n\n${plainText(editor.state.doc)}` : ""}
        busy={busy !== null}
        onLoadLatest={loadLatest}
        onOverwrite={overwrite}
      />
    </Box>
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
