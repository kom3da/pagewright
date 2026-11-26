import { zodResolver } from "@hookform/resolvers/zod";
import { Button, Group, NativeSelect, SimpleGrid, TextInput } from "@mantine/core";
import type { ReactNode } from "react";
import { Controller, useForm } from "react-hook-form";
import { z } from "zod";
import { errorField, isApiError } from "../api/client";
import { slugSchema } from "../lib/slug";
import { Form } from "./ui/Form";
import { ErrorMessage } from "./ui/feedback";

const schema = z.object({
  name: z.string().trim().min(1, "名前を入力してください。"),
  slug: slugSchema,
  parentId: z.string(),
});

export type NameSlugValues = z.infer<typeof schema>;

type Props = {
  /** "カテゴリ" or "タグ" */
  noun: string;
  slugExample: string;
  defaultValues?: Partial<NameSlugValues>;
  /** Choices for the parent (categories only). */
  parents?: { id: number; label: string }[];
  submitLabel: string;
  onSubmit: (values: NameSlugValues) => Promise<unknown>;
  extraButtons?: ReactNode;
};

/** Name + URL (+ parent) form shared by the category and tag screens. */
export function NameSlugForm({
  noun,
  slugExample,
  defaultValues,
  parents,
  submitLabel,
  onSubmit,
  extraButtons,
}: Props) {
  const form = useForm<NameSlugValues>({
    resolver: zodResolver(schema),
    mode: "onChange",
    defaultValues: { name: "", slug: "", parentId: "", ...defaultValues },
  });
  const { errors, isSubmitting } = form.formState;

  const submit = form.handleSubmit(async (values) => {
    try {
      await onSubmit(values);
      form.reset();
    } catch (e) {
      const field = errorField(e);
      if (isApiError(e) && (field === "name" || field === "slug" || field === "parentId")) {
        form.setError(field, { message: e.message }, { shouldFocus: true });
      } else if (isApiError(e)) {
        form.setError("root", { message: e.message });
      } else {
        throw e;
      }
    }
  });

  return (
    <Form onSubmit={submit} gap="md">
      <SimpleGrid cols={{ base: 1, sm: 2 }} spacing="md">
        <TextInput
          label={`${noun}の名前`}
          required
          error={errors.name?.message}
          {...form.register("name")}
        />
        <TextInput
          label={`${noun}のURL（英数字）`}
          description={`英小文字・数字・ハイフン（-）。例：${slugExample}`}
          required
          autoCapitalize="none"
          autoComplete="off"
          spellCheck={false}
          error={errors.slug?.message}
          {...form.register("slug")}
        />
        {parents && (
          <Controller
            control={form.control}
            name="parentId"
            render={({ field, fieldState }) => (
              <NativeSelect
                label="親カテゴリ"
                ref={field.ref}
                value={field.value}
                onChange={(e) => field.onChange(e.currentTarget.value)}
                error={fieldState.error?.message}
              >
                <option value="">なし（いちばん上の階層）</option>
                {parents.map((p) => (
                  <option key={p.id} value={String(p.id)}>
                    {p.label}
                  </option>
                ))}
              </NativeSelect>
            )}
          />
        )}
      </SimpleGrid>
      {errors.root && <ErrorMessage>{errors.root.message}</ErrorMessage>}
      <Group justify="flex-end" gap="sm">
        {extraButtons}
        <Button type="submit" variant="filled" loading={isSubmitting}>
          {submitLabel}
        </Button>
      </Group>
    </Form>
  );
}
