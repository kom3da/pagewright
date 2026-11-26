import { Button, Code, EmptyState, Stack } from "@mantine/core";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Tags } from "lucide-react";
import { useState } from "react";
import { api, errorMessage } from "../api/client";
import { tagsQuery } from "../api/queries";
import type { Tag } from "../api/types";
import { NameSlugForm } from "../components/NameSlugForm";
import { confirm } from "../components/ui/confirm";
import { Dialog } from "../components/ui/Dialog";
import { ErrorMessage, Loading } from "../components/ui/feedback";
import { notify } from "../components/ui/notify";
import { PageHeader } from "../components/ui/PageHeader";
import { ResponsiveList } from "../components/ui/ResponsiveList";
import { Section } from "../components/ui/Section";

export function TagsPage() {
  const { data, isPending, error } = useQuery(tagsQuery);
  const queryClient = useQueryClient();
  const [editing, setEditing] = useState<Tag | null>(null);

  const refresh = () => queryClient.invalidateQueries({ queryKey: tagsQuery.queryKey });

  const remove = useMutation({
    mutationFn: (t: Tag) => api.deleteTag(t.id),
    onSuccess: (_, t) => {
      void refresh();
      notify(`タグ「${t.name}」を削除しました。`);
    },
    onError: (e) => notify(errorMessage(e), "error"),
  });

  const askDelete = async (t: Tag) => {
    const ok = await confirm({
      title: "タグを削除しますか？",
      message: `「${t.name}」を削除します。記事は削除されず、このタグが外れます。`,
      confirmLabel: "削除する",
      danger: true,
    });
    if (ok) remove.mutate(t);
  };

  const tags = [...(data ?? [])].sort((a, b) => a.name.localeCompare(b.name, "ja"));

  return (
    <>
      <PageHeader
        title="タグ"
        description="記事につけるキーワードです。記事の編集画面の「詳細設定」からも追加できます。"
      />
      <Stack gap="xl">
        <Section title="タグを追加">
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
        </Section>

        <Section title="タグの一覧" variant="plain">
          {isPending ? (
            <Loading />
          ) : error ? (
            <ErrorMessage>{errorMessage(error)}</ErrorMessage>
          ) : tags.length === 0 ? (
            <EmptyState icon={<Tags aria-hidden="true" />} title="まだタグがありません" />
          ) : (
            <ResponsiveList
              items={tags}
              getKey={(t) => t.id}
              caption="タグの一覧（名前の順）"
              columns={[
                { key: "name", header: "名前", render: (t) => t.name },
                { key: "slug", header: "URL（英数字）", render: (t) => <Code>{t.slug}</Code> },
              ]}
              actions={(t) => (
                <>
                  <Button
                    size="sm"
                    onClick={() => setEditing(t)}
                    aria-label={`「${t.name}」を編集`}
                  >
                    編集
                  </Button>
                  <Button
                    size="sm"
                    variant="subtle"
                    color="red"
                    onClick={() => askDelete(t)}
                    aria-label={`「${t.name}」を削除`}
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
    </>
  );
}
