import {
  Box,
  Button,
  Group,
  Input,
  NativeSelect,
  Paper,
  Radio,
  Stack,
  Text,
  TextInput,
  VisuallyHidden,
} from "@mantine/core";
import { ArrowDown, ArrowUp, Plus } from "lucide-react";
import { type FormEvent, useEffect, useRef, useState } from "react";
import type { ContentSummary, MenuItem } from "../api/types";
import classes from "./MenuEditor.module.css";
import { Dialog } from "./ui/Dialog";
import { Form } from "./ui/Form";
import { Notice } from "./ui/feedback";

export type MenuRow = MenuItem & { key: number };

/** Limits checked by PUT /api/site (internal/api/site.go). */
export const MAX_MENU_ITEMS = 20;
const MAX_LABEL = 50;
const menuUrl = /^(\/|https?:\/\/)\S*$/;

let nextKey = 1;
export const toMenuRows = (items: MenuItem[]): MenuRow[] =>
  items.map((it) => ({ ...it, key: nextKey++ }));

type Props = {
  items: MenuRow[];
  onChange: (items: MenuRow[]) => void;
  contents: ContentSummary[];
  error?: string;
  errorId: string;
};

const typeLabel = { page: "固定ページ", post: "記事" } as const;

/** Problem with a menu item that the server would reject, or undefined. */
export function menuItemProblem(it: MenuItem, index: number) {
  const n = `${index + 1}番目の項目`;
  if (it.label !== null && (!it.label.trim() || [...it.label.trim()].length > MAX_LABEL)) {
    return `メニューの${n}の表示名を${MAX_LABEL}文字以内で入力してください。`;
  }
  if (it.url !== null && (!menuUrl.test(it.url) || it.label === null)) {
    return `メニューの${n}のURLは「/」か「https://」で始め、表示名も入れてください。`;
  }
  return undefined;
}

export function MenuEditor({ items, onChange, contents, error, errorId }: Props) {
  const [editing, setEditing] = useState<MenuRow | "new" | null>(null);
  const [announce, setAnnounce] = useState("");
  const [focusTarget, setFocusTarget] = useState<string | null>(null);
  const buttons = useRef(new Map<string, HTMLButtonElement>());
  const addRef = useRef<HTMLButtonElement>(null);

  useEffect(() => {
    if (!focusTarget) return;
    (focusTarget === "add" ? addRef.current : buttons.current.get(focusTarget))?.focus();
    setFocusTarget(null);
  }, [focusTarget]);

  const byId = new Map(contents.map((c) => [c.id, c]));
  const title = (it: MenuItem) => {
    if (it.label) return it.label;
    const c = it.contentId !== null ? byId.get(it.contentId) : undefined;
    return c ? c.title || "（タイトルなし）" : "（見つからないページ）";
  };

  const move = (index: number, by: -1 | 1) => {
    const to = index + by;
    const next = [...items];
    const [row] = next.splice(index, 1);
    next.splice(to, 0, row);
    onChange(next);
    setAnnounce(`「${title(row)}」を${to + 1}番目に移動しました。`);
    // Keep focus on the moved item; at either end the button pressed becomes disabled.
    const atEnd = by === -1 ? to === 0 : to === next.length - 1;
    const dir = by === -1 ? "up" : "down";
    const other = by === -1 ? "down" : "up";
    setFocusTarget(`${row.key}-${atEnd ? other : dir}`);
  };

  const remove = (index: number) => {
    const row = items[index];
    const next = items.filter((_, i) => i !== index);
    onChange(next);
    setAnnounce(`「${title(row)}」をメニューから外しました。`);
    const neighbor = next[index] ?? next[index - 1];
    setFocusTarget(neighbor ? `${neighbor.key}-delete` : "add");
  };

  const save = (it: MenuItem) => {
    if (editing === "new") {
      const row = { ...it, key: nextKey++ };
      onChange([...items, row]);
      setAnnounce(`「${title(row)}」をメニューの最後に追加しました。`);
    } else if (editing) {
      onChange(items.map((r) => (r.key === editing.key ? { ...it, key: r.key } : r)));
    }
    setEditing(null);
  };

  const setButton = (id: string) => (el: HTMLButtonElement | null) => {
    if (el) buttons.current.set(id, el);
    else buttons.current.delete(id);
  };

  return (
    <Stack gap="sm">
      {items.length === 0 && (
        <Notice>
          いまはメニューに何も登録していません。この場合、親のない固定ページと「お知らせ」（記事の一覧）が自動で並びます。
        </Notice>
      )}
      {error && (
        <Input.Error id={errorId} tabIndex={-1} data-error-focus="" size="sm">
          {error}
        </Input.Error>
      )}
      {items.length > 0 && (
        <Paper
          withBorder
          component="ol"
          className={classes.list}
          aria-label="メニューの項目（公開サイトでの並び順）"
          aria-describedby={error ? errorId : undefined}
        >
          {items.map((it, i) => {
            const name = title(it);
            const c = it.contentId !== null ? byId.get(it.contentId) : undefined;
            return (
              <Group component="li" key={it.key} gap="sm" className={classes.row}>
                <Text
                  component="span"
                  aria-hidden="true"
                  fw={700}
                  c="dimmed"
                  className={classes.number}
                >
                  {i + 1}.
                </Text>
                <Box className={classes.text}>
                  <Text fw={700}>{name}</Text>
                  <Text size="sm" c="dimmed">
                    {it.url !== null
                      ? `URL：${it.url}`
                      : c
                        ? `${typeLabel[c.type]}「${c.title || "（タイトルなし）"}」${
                            c.status === "published"
                              ? ""
                              : "（公開していないため、いまはサイトに出ません）"
                          }`
                        : "リンク先のページが見つかりません。削除されたか、編集で選び直してください。"}
                  </Text>
                </Box>
                <Group gap={4} className={classes.actions}>
                  <Button
                    ref={setButton(`${it.key}-up`)}
                    size="sm"
                    variant="subtle"
                    color="gray"
                    disabled={i === 0}
                    onClick={() => move(i, -1)}
                    aria-label={`「${name}」を上へ移動`}
                    leftSection={<ArrowUp aria-hidden="true" size={16} />}
                  >
                    上へ
                  </Button>
                  <Button
                    ref={setButton(`${it.key}-down`)}
                    size="sm"
                    variant="subtle"
                    color="gray"
                    disabled={i === items.length - 1}
                    onClick={() => move(i, 1)}
                    aria-label={`「${name}」を下へ移動`}
                    leftSection={<ArrowDown aria-hidden="true" size={16} />}
                  >
                    下へ
                  </Button>
                  <Button size="sm" onClick={() => setEditing(it)} aria-label={`「${name}」を編集`}>
                    編集
                  </Button>
                  <Button
                    ref={setButton(`${it.key}-delete`)}
                    size="sm"
                    variant="subtle"
                    color="red"
                    onClick={() => remove(i)}
                    aria-label={`「${name}」をメニューから外す`}
                  >
                    外す
                  </Button>
                </Group>
              </Group>
            );
          })}
        </Paper>
      )}
      <Group gap="sm">
        <Button
          ref={addRef}
          onClick={() => setEditing("new")}
          disabled={items.length >= MAX_MENU_ITEMS}
          aria-describedby={items.length >= MAX_MENU_ITEMS ? "menu-limit" : undefined}
          leftSection={<Plus aria-hidden="true" size={16} />}
        >
          メニューに項目を追加
        </Button>
        {items.length >= MAX_MENU_ITEMS && (
          <Text id="menu-limit" size="sm" c="dimmed">
            メニューは{MAX_MENU_ITEMS}項目までです。
          </Text>
        )}
      </Group>
      <VisuallyHidden role="status">{announce}</VisuallyHidden>
      <Dialog
        open={editing !== null}
        onOpenChange={(o) => !o && setEditing(null)}
        title={editing === "new" ? "メニューに項目を追加" : "メニューの項目を編集"}
      >
        {editing !== null && (
          <MenuItemForm
            initial={editing === "new" ? null : editing}
            contents={contents}
            onSubmit={save}
            onCancel={() => setEditing(null)}
          />
        )}
      </Dialog>
    </Stack>
  );
}

function MenuItemForm({
  initial,
  contents,
  onSubmit,
  onCancel,
}: {
  initial: MenuItem | null;
  contents: ContentSummary[];
  onSubmit: (it: MenuItem) => void;
  onCancel: () => void;
}) {
  const [kind, setKind] = useState<"content" | "url">(initial?.url != null ? "url" : "content");
  const [contentId, setContentId] = useState(
    initial?.contentId != null ? String(initial.contentId) : "",
  );
  const [url, setUrl] = useState(initial?.url ?? "");
  const [label, setLabel] = useState(initial?.label ?? "");
  const [errors, setErrors] = useState<{ contentId?: string; url?: string; label?: string }>({});
  const formRef = useRef<HTMLFormElement>(null);

  const submit = (e: FormEvent) => {
    e.preventDefault();
    e.stopPropagation();
    const l = label.trim();
    const next: typeof errors = {};
    if (kind === "content" && !contentId)
      next.contentId = "リンク先のページか記事を選んでください。";
    if (kind === "url" && !menuUrl.test(url.trim())) {
      next.url =
        "URLは「/」か「https://」で始めてください（例：https://example.jp/ や /company/）。空白は使えません。";
    }
    if (kind === "url" && !l)
      next.label = "URLのときは、メニューに表示する名前を入力してください。";
    if ([...l].length > MAX_LABEL) next.label = `表示名は${MAX_LABEL}文字以内で入力してください。`;
    setErrors(next);
    if (Object.keys(next).length > 0) {
      setTimeout(() => formRef.current?.querySelector<HTMLElement>("[aria-invalid=true]")?.focus());
      return;
    }
    onSubmit(
      kind === "content"
        ? { contentId: Number(contentId), url: null, label: l || null }
        : { contentId: null, url: url.trim(), label: l },
    );
  };

  const pages = contents.filter((c) => c.type === "page");
  const posts = contents.filter((c) => c.type === "post");
  const option = (c: ContentSummary) => (
    <option key={c.id} value={String(c.id)}>
      {c.title || "（タイトルなし）"}
      {c.status === "published" ? "" : "（公開していません）"}
    </option>
  );

  return (
    <Form ref={formRef} onSubmit={submit} gap="md">
      <Radio.Group
        label="リンク先"
        name="menu-kind"
        value={kind}
        onChange={(v) => {
          setKind(v as "content" | "url");
          setErrors({});
        }}
      >
        <Stack gap="xs" mt="xs">
          <Radio value="content" label="このサイトのページや記事" />
          <Radio value="url" label="URLを入力する（ほかのサイトなど）" />
        </Stack>
      </Radio.Group>
      {kind === "content" ? (
        <NativeSelect
          label="ページや記事"
          error={errors.contentId}
          required
          value={contentId}
          onChange={(e) => setContentId(e.currentTarget.value)}
        >
          <option value="">選んでください</option>
          {pages.length > 0 && <optgroup label="固定ページ">{pages.map(option)}</optgroup>}
          {posts.length > 0 && <optgroup label="記事">{posts.map(option)}</optgroup>}
        </NativeSelect>
      ) : (
        <TextInput
          label="URL"
          description="ほかのサイトは https:// から、このサイトの中は / から入力します。"
          required
          type="url"
          inputMode="url"
          autoComplete="off"
          spellCheck={false}
          value={url}
          onChange={(e) => setUrl(e.target.value)}
          error={errors.url}
        />
      )}
      <TextInput
        label="メニューに表示する名前"
        description={
          kind === "content"
            ? `空欄なら、ページや記事のタイトルを表示します。${MAX_LABEL}文字まで。`
            : `${MAX_LABEL}文字まで。`
        }
        required={kind === "url"}
        value={label}
        onChange={(e) => setLabel(e.target.value)}
        error={errors.label}
      />
      <Group justify="flex-end" gap="sm">
        <Button onClick={onCancel}>キャンセル</Button>
        <Button type="submit" variant="filled">
          {initial ? "変更する" : "追加する"}
        </Button>
      </Group>
    </Form>
  );
}
