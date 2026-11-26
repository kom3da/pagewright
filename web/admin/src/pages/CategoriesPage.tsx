import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { api, errorMessage } from "../api/client";
import { categoriesQuery } from "../api/queries";
import type { Category } from "../api/types";
import { NameSlugForm, type NameSlugValues } from "../components/NameSlugForm";
import { useNotify } from "../components/Toaster";
import { Button } from "../components/ui/Button";
import { ConfirmDialog, Dialog } from "../components/ui/Dialog";
import { ErrorMessage, Loading, PageHeading } from "../components/ui/misc";
import { buildCategoryTree, descendantIds, flattenTree } from "../lib/categoryTree";

const toInput = (v: NameSlugValues) => ({
  name: v.name,
  slug: v.slug,
  parentId: v.parentId === "" ? null : Number(v.parentId),
});

export function CategoriesPage() {
  const { data, isPending, error } = useQuery(categoriesQuery);
  const queryClient = useQueryClient();
  const notify = useNotify();
  const [editing, setEditing] = useState<Category | null>(null);
  const [deleting, setDeleting] = useState<Category | null>(null);

  const refresh = () => queryClient.invalidateQueries({ queryKey: categoriesQuery.queryKey });

  const remove = useMutation({
    mutationFn: (c: Category) => api.deleteCategory(c.id),
    onSuccess: (_, c) => {
      setDeleting(null);
      void refresh();
      notify(`カテゴリ「${c.name}」を削除しました。`);
    },
    onError: (e) => {
      setDeleting(null);
      notify(errorMessage(e), "error");
    },
  });

  const categories = data ?? [];
  const deletingChildren = categories.filter((c) => deleting && c.parentId === deleting.id);
  const rows = flattenTree(buildCategoryTree(categories));
  const parentChoices = (exclude?: number) => {
    const excluded = exclude === undefined ? new Set<number>() : descendantIds(categories, exclude);
    return rows
      .filter((c) => !excluded.has(c.id))
      .map((c) => ({ id: c.id, label: `${"　".repeat(c.depth)}${c.name}` }));
  };

  return (
    <div className="space-y-8">
      <PageHeading title="カテゴリ">カテゴリ</PageHeading>
      <p>記事を分類するためのカテゴリです。親カテゴリを選ぶと階層にできます。</p>

      <section
        aria-labelledby="add-category"
        className="space-y-3 rounded-lg border border-slate-300 bg-white p-4"
      >
        <h2 id="add-category" className="text-lg font-bold">
          カテゴリを追加
        </h2>
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
      </section>

      <section aria-labelledby="category-list" className="space-y-3">
        <h2 id="category-list" className="text-lg font-bold">
          カテゴリの一覧
        </h2>
        {isPending ? (
          <Loading />
        ) : error ? (
          <ErrorMessage>{errorMessage(error)}</ErrorMessage>
        ) : rows.length === 0 ? (
          <p>まだカテゴリがありません。</p>
        ) : (
          <div className="overflow-x-auto rounded-lg border border-slate-300 bg-white">
            <table className="w-full text-left">
              <thead className="border-b border-slate-300 bg-slate-100 text-sm">
                <tr>
                  <th scope="col" className="px-4 py-2">
                    名前
                  </th>
                  <th scope="col" className="px-4 py-2">
                    URL（英数字）
                  </th>
                  <th scope="col" className="px-4 py-2">
                    <span className="sr-only">操作</span>
                  </th>
                </tr>
              </thead>
              <tbody>
                {rows.map((c) => (
                  <tr key={c.id} className="border-b border-slate-200 last:border-0">
                    <td className="px-4 py-2" style={{ paddingLeft: `${1 + c.depth * 1.5}rem` }}>
                      {c.depth > 0 && (
                        <span aria-hidden="true" className="mr-1 text-slate-600">
                          └
                        </span>
                      )}
                      {c.name}
                    </td>
                    <td className="px-4 py-2 font-mono text-sm">{c.slug}</td>
                    <td className="px-4 py-2">
                      <div className="flex justify-end gap-2">
                        <Button onClick={() => setEditing(c)} aria-label={`「${c.name}」を編集`}>
                          編集
                        </Button>
                        <Button
                          variant="ghost"
                          className="text-red-700"
                          onClick={() => setDeleting(c)}
                          aria-label={`「${c.name}」を削除`}
                        >
                          削除
                        </Button>
                      </div>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </section>

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

      <ConfirmDialog
        open={deleting !== null}
        onOpenChange={(o) => !o && setDeleting(null)}
        title="カテゴリを削除しますか？"
        description={
          <>
            「{deleting?.name}」を削除します。記事は削除されず、このカテゴリから外れます。
            {deletingChildren.length > 0 &&
              `子カテゴリ（${deletingChildren.map((c) => `「${c.name}」`).join("")}）は削除されず、一つ上の階層に移ります。`}
          </>
        }
        confirmLabel="削除する"
        danger
        busy={remove.isPending}
        onConfirm={() => deleting && remove.mutate(deleting)}
      />
    </div>
  );
}
