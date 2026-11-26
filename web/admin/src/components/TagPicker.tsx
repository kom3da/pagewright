import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { type KeyboardEvent, useRef, useState } from "react";
import { api, errorField, errorMessage } from "../api/client";
import { tagsQuery } from "../api/queries";
import { slugProblem } from "../lib/slug";
import { Button } from "./ui/Button";
import { TextField } from "./ui/Field";
import { ErrorMessage, Loading } from "./ui/misc";

type Props = { value: number[]; onChange: (ids: number[]) => void };

export function TagPicker({ value, onChange }: Props) {
  const { data, isPending, error } = useQuery(tagsQuery);
  const [filter, setFilter] = useState("");
  const toggle = (id: number, checked: boolean) =>
    onChange(checked ? [...value, id] : value.filter((v) => v !== id));

  const shown = (data ?? []).filter(
    (t) => !filter || t.name.includes(filter) || t.slug.includes(filter) || value.includes(t.id),
  );

  return (
    <fieldset className="space-y-3">
      <legend className="font-semibold">タグ</legend>
      {isPending ? (
        <Loading />
      ) : error ? (
        <ErrorMessage>{errorMessage(error)}</ErrorMessage>
      ) : (
        <>
          {data.length > 10 && (
            <TextField
              label="タグを絞り込む"
              value={filter}
              onChange={(e) => setFilter(e.target.value)}
              className="max-w-sm"
            />
          )}
          {data.length === 0 ? (
            <p className="text-sm text-slate-700">タグはまだありません。</p>
          ) : (
            <ul className="flex flex-wrap gap-x-5 gap-y-1">
              {shown.map((t) => (
                <li key={t.id}>
                  <label className="inline-flex items-center gap-2">
                    <input
                      type="checkbox"
                      className="size-5"
                      checked={value.includes(t.id)}
                      onChange={(e) => toggle(t.id, e.target.checked)}
                    />
                    {t.name}
                  </label>
                </li>
              ))}
            </ul>
          )}
          <NewTag onCreated={(id) => onChange([...value, id])} />
        </>
      )}
    </fieldset>
  );
}

/** Inline "add a tag" fields. Not a <form>, because it lives inside the editor screen. */
function NewTag({ onCreated }: { onCreated: (id: number) => void }) {
  const queryClient = useQueryClient();
  const [name, setName] = useState("");
  const [slug, setSlug] = useState("");
  const [errors, setErrors] = useState<{ name?: string; slug?: string; form?: string }>({});
  const nameRef = useRef<HTMLInputElement>(null);
  const slugRef = useRef<HTMLInputElement>(null);

  const create = useMutation({
    mutationFn: api.createTag,
    onSuccess: (tag) => {
      void queryClient.invalidateQueries({ queryKey: tagsQuery.queryKey });
      onCreated(tag.id);
      setName("");
      setSlug("");
      setErrors({});
    },
    onError: (e) => {
      const field = errorField(e);
      if (field === "slug" || field === "name") {
        setErrors({ [field]: e.message });
        (field === "slug" ? slugRef : nameRef).current?.focus();
      } else {
        setErrors({ form: errorMessage(e) });
      }
    },
  });

  const submit = () => {
    const next = {
      name: name.trim() ? undefined : "タグの名前を入力してください。",
      slug: slugProblem(slug) ?? undefined,
    };
    setErrors(next);
    if (!next.name && !next.slug) create.mutate({ name: name.trim(), slug });
  };
  const onEnter = (e: KeyboardEvent) => {
    if (e.key === "Enter" && !e.nativeEvent.isComposing) {
      e.preventDefault();
      submit();
    }
  };

  return (
    <div className="space-y-3 rounded-md border border-slate-300 p-3">
      <p className="font-semibold">新しいタグを追加</p>
      <div className="grid gap-3 sm:grid-cols-2">
        <TextField
          label="タグの名前"
          ref={nameRef}
          value={name}
          onChange={(e) => setName(e.target.value)}
          onKeyDown={onEnter}
          error={errors.name}
        />
        <TextField
          label="タグのURL（英数字）"
          ref={slugRef}
          hint="例：event"
          value={slug}
          onChange={(e) => {
            setSlug(e.target.value);
            setErrors({ ...errors, slug: undefined });
          }}
          onKeyDown={onEnter}
          // Guidance appears while typing (e.g. as soon as Japanese is entered).
          error={errors.slug ?? (slug ? (slugProblem(slug) ?? undefined) : undefined)}
          autoCapitalize="none"
          spellCheck={false}
        />
      </div>
      {errors.form && <ErrorMessage>{errors.form}</ErrorMessage>}
      <Button onClick={submit} disabled={create.isPending}>
        タグを追加して選ぶ
      </Button>
    </div>
  );
}
