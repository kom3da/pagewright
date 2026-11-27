import { Combobox, InputBase, Loader, Text, useCombobox, VisuallyHidden } from "@mantine/core";
import { useDebouncedValue } from "@mantine/hooks";
import { keepPreviousData, useQueries } from "@tanstack/react-query";
import { type KeyboardEvent, useId, useRef, useState } from "react";
import { errorMessage, isApiError } from "../api/client";
import { CONTENT_PICKER_LIMIT, contentChoicesQuery, contentQuery } from "../api/queries";
import type { ContentSummary, ContentType } from "../api/types";

const typeLabel = { page: "固定ページ", post: "記事" } as const;
/** GET /api/contents refuses a longer q. */
const MAX_Q = 100;
const DEBOUNCE_MS = 300;

export const contentTitle = (c: Pick<ContentSummary, "title">) => c.title || "（タイトルなし）";
const optionLabel = (c: ContentSummary) =>
  `${contentTitle(c)}${c.status === "published" ? "" : "（公開していません）"}`;

/** A content looked up by id: "missing" when it no longer exists, undefined while loading. */
export type LookedUp = ContentSummary | "missing" | "failed" | undefined;

/**
 * Contents by id (GET /api/contents/{id}, one request each), for showing what was chosen
 * without loading every content. Ids in `known` are not fetched.
 */
export function useContentsById(ids: number[], known?: ReadonlyMap<number, ContentSummary>) {
  const wanted = [...new Set(ids)].filter((id) => !known?.has(id));
  const results = useQueries({ queries: wanted.map((id) => contentQuery(id)) });
  const out = new Map<number, LookedUp>(known);
  wanted.forEach((id, i) => {
    const { data, error } = results[i];
    out.set(
      id,
      data ??
        (error ? (isApiError(error) && error.status === 404 ? "missing" : "failed") : undefined),
    );
  });
  return out;
}

export const lookedUpLabel = (c: LookedUp) =>
  c === undefined
    ? "読み込み中…"
    : c === "missing"
      ? "（見つからないページ。選び直してください）"
      : c === "failed"
        ? "（タイトルを読み込めませんでした）"
        : optionLabel(c);

type Props = {
  label: string;
  description?: string;
  error?: string;
  required?: boolean;
  maw?: string;
  /** What can be chosen; with both, the choices are grouped by type. */
  types: ContentType[];
  value: number | null;
  /** `item` is null when nothing is chosen. */
  onChange: (id: number | null, item: ContentSummary | null) => void;
  /** First choice meaning "none" (「選ばない」). Without it, a choice cannot be undone. */
  emptyLabel?: string;
};

/**
 * Chooses a page or post by searching its title on the server (GET /api/contents?q=),
 * so that a site with thousands of posts never loads them all. The input shows the
 * current choice; typing searches, the arrow keys move and Enter chooses.
 */
export function ContentPicker({
  label,
  description,
  error,
  required,
  maw,
  types,
  value,
  onChange,
  emptyLabel,
}: Props) {
  const id = useId();
  const combobox = useCombobox({ onDropdownClose: () => combobox.resetSelectedOption() });
  const opened = combobox.dropdownOpened;
  // null while not typing: the input then shows the current choice.
  const [search, setSearch] = useState<string | null>(null);
  const [picked, setPicked] = useState<ContentSummary | null>(null);
  const justFocused = useRef(false);
  const [q] = useDebouncedValue((search ?? "").trim(), DEBOUNCE_MS);

  const choices = useQueries({
    queries: types.map((t) => ({
      ...contentChoicesQuery(t, q),
      enabled: opened,
      placeholderData: keepPreviousData,
    })),
  });
  const known = picked && picked.id === value ? new Map([[picked.id, picked]]) : undefined;
  const current = useContentsById(value === null ? [] : [value], known).get(value ?? 0);
  const shown = value === null ? (emptyLabel ?? "") : lookedUpLabel(current);

  const groups = types.map((type, i) => ({ type, items: choices[i].data ?? [] }));
  const items = groups.flatMap((g) => g.items);
  const failed = choices.find((c) => c.error)?.error;
  const settled = choices.every((c) => c.data && !c.isFetching && !c.isPlaceholderData);
  const full = groups.some((g) => g.items.length >= CONTENT_PICKER_LIMIT);
  const what = types.map((t) => typeLabel[t]).join("や");
  // The shown q may lag behind the typed text by the debounce; say which one was searched.
  const found = q
    ? items.length > 0
      ? `「${q}」に合う${what}：${items.length}件`
      : `「${q}」に合う${what}は見つかりません`
    : items.length > 0
      ? `新しい${what}${items.length}件を表示しています${full ? "。タイトルの一部を入力すると探せます" : ""}`
      : `${what}はまだありません`;
  const announce = !opened || failed ? "" : settled ? found : "";

  const choose = (v: string) => {
    const chosen = v === "" ? null : (items.find((c) => String(c.id) === v) ?? null);
    setPicked(chosen);
    onChange(chosen?.id ?? null, chosen);
    setSearch(null);
    combobox.closeDropdown();
  };

  const onKeyDown = (e: KeyboardEvent<HTMLInputElement>) => {
    if (e.nativeEvent.isComposing) return;
    // Escape closes the list only, not a dialog around the picker.
    if (e.key === "Escape" && opened) {
      e.stopPropagation();
      setSearch(null);
    }
    // Enter chooses (handled by the combobox) or opens the list; it never submits the form.
    if (e.key === "Enter") {
      e.preventDefault();
      if (!opened) combobox.openDropdown();
    }
  };

  const option = (c: ContentSummary) => (
    <Combobox.Option
      key={c.id}
      value={String(c.id)}
      selected={c.id === value}
      aria-selected={c.id === value}
    >
      {optionLabel(c)}
    </Combobox.Option>
  );

  return (
    <>
      <Combobox store={combobox} onOptionSubmit={choose}>
        <Combobox.Target targetType="input" withExpandedAttribute autoComplete="off">
          <InputBase
            id={id}
            label={label}
            description={description}
            error={error}
            required={required}
            maw={maw}
            value={search ?? shown}
            maxLength={MAX_Q}
            spellCheck={false}
            onChange={(e) => {
              setSearch(e.currentTarget.value);
              combobox.openDropdown();
              combobox.resetSelectedOption();
            }}
            // Typing replaces the shown choice instead of adding to it.
            onFocus={(e) => {
              e.currentTarget.select();
              justFocused.current = true;
            }}
            // The mouseup of the click that focused would put the caret in the selection.
            onMouseUp={(e) => {
              if (justFocused.current) e.preventDefault();
              justFocused.current = false;
            }}
            onClick={() => combobox.openDropdown()}
            onBlur={() => {
              setSearch(null);
              combobox.closeDropdown();
            }}
            onKeyDown={onKeyDown}
            rightSection={
              opened && !settled && !failed ? (
                <Loader size={16} aria-hidden="true" />
              ) : (
                <Combobox.Chevron />
              )
            }
            rightSectionPointerEvents="none"
          />
        </Combobox.Target>
        <Combobox.Dropdown>
          <Combobox.Options labelledBy={`${id}-label`} mah="20rem" style={{ overflowY: "auto" }}>
            {emptyLabel && !q && (
              <Combobox.Option value="" selected={value === null} aria-selected={value === null}>
                {emptyLabel}
              </Combobox.Option>
            )}
            {types.length > 1
              ? groups.map(
                  (g) =>
                    g.items.length > 0 && (
                      <Combobox.Group key={g.type} label={typeLabel[g.type]}>
                        {g.items.map(option)}
                      </Combobox.Group>
                    ),
                )
              : items.map(option)}
          </Combobox.Options>
          {failed ? (
            <Combobox.Empty>{errorMessage(failed)}</Combobox.Empty>
          ) : items.length === 0 ? (
            <Combobox.Empty>
              {!settled ? "読み込み中…" : q ? "見つかりません" : `${what}はまだありません`}
            </Combobox.Empty>
          ) : (
            full && (
              <Combobox.Footer>
                <Text size="sm" c="dimmed">
                  {q
                    ? `合うものが多いため、${CONTENT_PICKER_LIMIT}件まで表示しています。もっと詳しく入力してください。`
                    : `新しいものから${CONTENT_PICKER_LIMIT}件を表示しています。見つからないときは、タイトルの一部を入力してください。`}
                </Text>
              </Combobox.Footer>
            )
          )}
        </Combobox.Dropdown>
      </Combobox>
      <VisuallyHidden role="status">{announce}</VisuallyHidden>
    </>
  );
}
