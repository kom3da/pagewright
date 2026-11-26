import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { api, errorMessage } from "../api/client";
import { tagsQuery } from "../api/queries";
import type { Tag } from "../api/types";
import { NameSlugForm } from "../components/NameSlugForm";
import { useNotify } from "../components/Toaster";
import { Button } from "../components/ui/Button";
import { ConfirmDialog, Dialog } from "../components/ui/Dialog";
import { ErrorMessage, Loading, PageHeading } from "../components/ui/misc";

export function TagsPage() {
  const { data, isPending, error } = useQuery(tagsQuery);
  const queryClient = useQueryClient();
  const notify = useNotify();
  const [editing, setEditing] = useState<Tag | null>(null);
  const [deleting, setDeleting] = useState<Tag | null>(null);

  const refresh = () => queryClient.invalidateQueries({ queryKey: tagsQuery.queryKey });

  const remove = useMutation({
    mutationFn: (t: Tag) => api.deleteTag(t.id),
    onSuccess: (_, t) => {
      setDeleting(null);
      void refresh();
      notify(`タグ「${t.name}」を削除しました。`);
    },
    onError: (e) => {
      setDeleting(null);
      notify(errorMessage(e), "error");
    },
  });

  const tags = [...(data ?? [])].sort((a, b) => a.name.localeCompare(b.name, "ja"));

  return (
    <div className="space-y-8">
      <PageHeading title="タグ">タグ</PageHeading>
      <p>記事につけるキーワードです。記事の編集画面の「詳細設定」からも追加できます。</p>

      <section
        aria-labelledby="add-tag"
        className="space-y-3 rounded-lg border border-slate-300 bg-white p-4"
      >
        <h2 id="add-tag" className="text-lg font-bold">
          タグを追加
        </h2>
        <NameSlugForm
          noun="タグ"
          slugExample="event"
          submitLabel="追加する"
          onSubmit={async (v) => {
            const t = await api.createTag({ name: v.name, slug: v.slug });
            await refresh();
            notify(`タグ「${t.name}」を追加しました。`);
          }}
        />
      </section>

      <section aria-labelledby="tag-list" className="space-y-3">
        <h2 id="tag-list" className="text-lg font-bold">
          タグの一覧
        </h2>
        {isPending ? (
          <Loading />
        ) : error ? (
          <ErrorMessage>{errorMessage(error)}</ErrorMessage>
        ) : tags.length === 0 ? (
          <p>まだタグがありません。</p>
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
                {tags.map((t) => (
                  <tr key={t.id} className="border-b border-slate-200 last:border-0">
                    <td className="px-4 py-2">{t.name}</td>
                    <td className="px-4 py-2 font-mono text-sm">{t.slug}</td>
                    <td className="px-4 py-2">
                      <div className="flex justify-end gap-2">
                        <Button onClick={() => setEditing(t)} aria-label={`「${t.name}」を編集`}>
                          編集
                        </Button>
                        <Button
                          variant="ghost"
                          className="text-red-700"
                          onClick={() => setDeleting(t)}
                          aria-label={`「${t.name}」を削除`}
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
        title="タグを編集"
        description="URL（英数字）を変えると、古いURLは新しいURLへ自動で転送されます。"
      >
        {editing && (
          <NameSlugForm
            noun="タグ"
            slugExample="event"
            defaultValues={{ name: editing.name, slug: editing.slug }}
            submitLabel="保存する"
            extraButtons={<Button onClick={() => setEditing(null)}>キャンセル</Button>}
            onSubmit={async (v) => {
              await api.patchTag(editing.id, { name: v.name, slug: v.slug });
              await refresh();
              setEditing(null);
              notify("タグを保存しました。");
            }}
          />
        )}
      </Dialog>

      <ConfirmDialog
        open={deleting !== null}
        onOpenChange={(o) => !o && setDeleting(null)}
        title="タグを削除しますか？"
        description={`「${deleting?.name ?? ""}」を削除します。記事は削除されず、このタグが外れます。`}
        confirmLabel="削除する"
        danger
        busy={remove.isPending}
        onConfirm={() => deleting && remove.mutate(deleting)}
      />
    </div>
  );
}
