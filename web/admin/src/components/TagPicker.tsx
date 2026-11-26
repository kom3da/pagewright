import {
  Button,
  Checkbox,
  Fieldset,
  Group,
  Paper,
  SimpleGrid,
  Stack,
  Text,
  TextInput,
} from "@mantine/core";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { type KeyboardEvent, useRef, useState } from "react";
import { api, errorField, errorMessage } from "../api/client";
import { tagsQuery } from "../api/queries";
import { slugProblem } from "../lib/slug";
import { ErrorMessage, Loading } from "./ui/feedback";

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
    <Fieldset legend="タグ" variant="unstyled">
      <Stack gap="sm">
        {isPending ? (
          <Loading />
        ) : error ? (
          <ErrorMessage>{errorMessage(error)}</ErrorMessage>
        ) : (
          <>
            {data.length > 10 && (
              <TextInput
                label="タグを絞り込む"
                value={filter}
                onChange={(e) => setFilter(e.target.value)}
                maw="24rem"
              />
            )}
            {data.length === 0 ? (
              <Text size="sm" c="dimmed">
                タグはまだありません。
              </Text>
            ) : (
              <Group component="ul" gap="lg" m={0} p={0} style={{ listStyle: "none" }}>
                {shown.map((t) => (
                  <li key={t.id}>
                    <Checkbox
                      label={t.name}
                      checked={value.includes(t.id)}
                      onChange={(e) => toggle(t.id, e.currentTarget.checked)}
                    />
                  </li>
                ))}
              </Group>
            )}
            <NewTag onCreated={(id) => onChange([...value, id])} />
          </>
        )}
      </Stack>
    </Fieldset>
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
    <Paper withBorder p="sm">
      <Stack gap="sm">
        <Text fw={700}>新しいタグを追加</Text>
        <SimpleGrid cols={{ base: 1, sm: 2 }} spacing="sm">
          <TextInput
            label="タグの名前"
            ref={nameRef}
            value={name}
            onChange={(e) => setName(e.target.value)}
            onKeyDown={onEnter}
            error={errors.name}
          />
          <TextInput
            label="タグのURL（英数字）"
            ref={slugRef}
            description="例：event"
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
        </SimpleGrid>
        {errors.form && <ErrorMessage>{errors.form}</ErrorMessage>}
        <Group>
          <Button onClick={submit} loading={create.isPending}>
            タグを追加して選ぶ
          </Button>
        </Group>
      </Stack>
    </Paper>
  );
}
