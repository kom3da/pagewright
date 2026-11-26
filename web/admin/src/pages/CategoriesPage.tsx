import { Button, Code, EmptyState, Stack, Text } from "@mantine/core";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { FolderTree } from "lucide-react";
import { useState } from "react";
import { api, errorMessage } from "../api/client";
import { categoriesQuery } from "../api/queries";
import type { Category } from "../api/types";
import { NameSlugForm, type NameSlugValues } from "../components/NameSlugForm";
import { confirm } from "../components/ui/confirm";
import { Dialog } from "../components/ui/Dialog";
import { ErrorMessage, Loading } from "../components/ui/feedback";
import { notify } from "../components/ui/notify";
import { PageHeader } from "../components/ui/PageHeader";
import { ResponsiveList } from "../components/ui/ResponsiveList";
import { Section } from "../components/ui/Section";
import { buildCategoryTree, descendantIds, flattenTree } from "../lib/categoryTree";

const toInput = (v: NameSlugValues) => ({
  name: v.name,
  slug: v.slug,
  parentId: v.parentId === "" ? null : Number(v.parentId),
});

export function CategoriesPage() {
  const { data, isPending, error } = useQuery(categoriesQuery);
  const queryClient = useQueryClient();
  const [editing, setEditing] = useState<Category | null>(null);

  const refresh = () => queryClient.invalidateQueries({ queryKey: categoriesQuery.queryKey });

  const remove = useMutation({
    mutationFn: (c: Category) => api.deleteCategory(c.id),
    onSuccess: (_, c) => {
      void refresh();
      notify(`カテゴリ「${c.name}」を削除しました。`);
    },
    onError: (e) => notify(errorMessage(e), "error"),
  });

  const categories = data ?? [];
  const rows = flattenTree(buildCategoryTree(categories));
  const parentChoices = (exclude?: number) => {
    const excluded = exclude === undefined ? new Set<number>() : descendantIds(categories, exclude);
    return rows
      .filter((c) => !excluded.has(c.id))
      .map((c) => ({ id: c.id, label: `${"　".repeat(c.depth)}${c.name}` }));
  };

  const askDelete = async (c: Category) => {
    const children = categories.filter((x) => x.parentId === c.id);
    const ok = await confirm({
      title: "カテゴリを削除しますか？",
      message: `「${c.name}」を削除します。記事は削除されず、このカテゴリから外れます。${
        children.length > 0
          ? `子カテゴリ（${children.map((x) => `「${x.name}」`).join("")}）は削除されず、一つ上の階層に移ります。`
          : ""
      }`,
      confirmLabel: "削除する",
      danger: true,
    });
    if (ok) remove.mutate(c);
  };

  return (
    <>
      <PageHeader
        title="カテゴリ"
        description="記事を分類するためのカテゴリです。親カテゴリを選ぶと階層にできます。"
      />
      <Stack gap="xl">
        <Section title="カテゴリを追加">
          <NameSlugForm
            noun="カテゴリ"
            slugExample="news"
            parents={parentChoices()}
            submitLabel="追加する"
            onSubmit={async (v) => {
              const c = await api.createCategory(toInput(v));
              await refresh();
              notify(`カテゴリ「${c.name}」を追加しました。`);
            }}
          />
        </Section>

        <Section title="カテゴリの一覧" variant="plain">
          {isPending ? (
            <Loading />
          ) : error ? (
            <ErrorMessage>{errorMessage(error)}</ErrorMessage>
          ) : rows.length === 0 ? (
            <EmptyState icon={<FolderTree aria-hidden="true" />} title="まだカテゴリがありません" />
          ) : (
            <ResponsiveList
              items={rows}
              getKey={(c) => c.id}
              caption="カテゴリの一覧（親カテゴリの下に子カテゴリ）"
              columns={[
                {
                  key: "name",
                  header: "名前",
                  render: (c) => (
                    <div style={{ paddingLeft: `${c.depth * 1.5}rem` }}>
                      {c.depth > 0 && (
                        <Text component="span" aria-hidden="true" c="dimmed" mr={4}>
                          └
                        </Text>
                      )}
                      {c.name}
                    </div>
                  ),
                },
                { key: "slug", header: "URL（英数字）", render: (c) => <Code>{c.slug}</Code> },
              ]}
              actions={(c) => (
                <>
                  <Button
                    size="sm"
                    onClick={() => setEditing(c)}
                    aria-label={`「${c.name}」を編集`}
                  >
                    編集
                  </Button>
                  <Button
                    size="sm"
                    variant="subtle"
                    color="red"
                    onClick={() => askDelete(c)}
                    aria-label={`「${c.name}」を削除`}
                  >
                    削除
                  </Button>
                </>
              )}
            />
          )}
        </Section>
      </Stack>

      <Dialog
        open={editing !== null}
        onOpenChange={(o) => !o && setEditing(null)}
        title="カテゴリを編集"
      >
        {editing && (
          <NameSlugForm
            noun="カテゴリ"
            slugExample="news"
            defaultValues={{
              name: editing.name,
              slug: editing.slug,
              parentId: editing.parentId === null ? "" : String(editing.parentId),
            }}
            parents={parentChoices(editing.id)}
            submitLabel="保存する"
            extraButtons={<Button onClick={() => setEditing(null)}>キャンセル</Button>}
            onSubmit={async (v) => {
              await api.patchCategory(editing.id, toInput(v));
              await refresh();
              setEditing(null);
              notify("カテゴリを保存しました。");
            }}
          />
        )}
      </Dialog>
    </>
  );
}
