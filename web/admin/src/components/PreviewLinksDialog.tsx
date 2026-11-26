import { Button, Group, Paper, Radio, Stack, Text, TextInput, Title } from "@mantine/core";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { type FormEvent, useId, useRef, useState } from "react";
import { api, errorMessage, isApiError } from "../api/client";
import { previewLinksQuery } from "../api/queries";
import type { PreviewLink } from "../api/types";
import { formatJapanDateTime } from "../lib/datetime";
import { confirm } from "./ui/confirm";
import { Dialog } from "./ui/Dialog";
import { Form } from "./ui/Form";
import { ErrorMessage, Loading, Notice } from "./ui/feedback";

export const PREVIEW_DAYS = [1, 3, 7, 30] as const;
const DEFAULT_DAYS = 3;

type Props = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  contentId: number;
  /** "記事" or "固定ページ". */
  label: string;
  /** There are unsaved changes: they are saved before the link is created. */
  dirty: boolean;
  /** Saves if needed, then creates the link. Resolves to null when saving failed. */
  onCreate: (days: number) => Promise<PreviewLink | null>;
  onCloseFocus?: () => void;
};

/** Creates, lists and revokes login-free preview links of one content. */
export function PreviewLinksDialog({ open, onOpenChange, label, onCloseFocus, ...rest }: Props) {
  return (
    <Dialog
      open={open}
      onOpenChange={onOpenChange}
      title="確認用のリンク"
      description={`ログインしなくても、公開前の${label}を見てもらえるリンクです。リンクを知っている人なら誰でも、有効期限まで、この${label}の最新の保存内容を見られます。公開サイトには出ません。`}
      onCloseFocus={onCloseFocus}
      wide
    >
      {open && <PreviewLinks label={label} onClose={() => onOpenChange(false)} {...rest} />}
    </Dialog>
  );
}

function PreviewLinks({
  contentId,
  label,
  dirty,
  onCreate,
  onClose,
}: Omit<Props, "open" | "onOpenChange" | "onCloseFocus"> & { onClose: () => void }) {
  const id = useId();
  const queryClient = useQueryClient();
  const links = useQuery(previewLinksQuery(contentId));
  const [days, setDays] = useState<number>(DEFAULT_DAYS);
  const [creating, setCreating] = useState(false);
  const [createError, setCreateError] = useState<string | null>(null);
  // The new link's URL is shown only until the dialog closes: it cannot be retrieved later.
  const [created, setCreated] = useState<PreviewLink | null>(null);
  const [copyResult, setCopyResult] = useState<{ ok: boolean; text: string } | null>(null);
  const urlRef = useRef<HTMLInputElement>(null);

  const [revokeBusy, setRevokeBusy] = useState(false);
  const [listMessage, setListMessage] = useState("");
  const [listError, setListError] = useState<string | null>(null);
  const listHeadingRef = useRef<HTMLHeadingElement>(null);

  const refreshList = () =>
    queryClient.invalidateQueries({ queryKey: previewLinksQuery(contentId).queryKey });

  const create = async (e: FormEvent) => {
    e.preventDefault();
    // Keep the editor's Ctrl+S / form handlers from seeing this submit.
    e.stopPropagation();
    if (creating) return;
    setCreating(true);
    setCreateError(null);
    try {
      const link = await onCreate(days);
      if (!link) return;
      setCreated(link);
      setCopyResult(null);
      setListMessage("");
      void refreshList();
      setTimeout(() => urlRef.current?.focus());
    } catch (err) {
      setCreateError(errorMessage(err));
    } finally {
      setCreating(false);
    }
  };

  const copy = async () => {
    const url = created?.url ?? "";
    try {
      if (!navigator.clipboard) throw new Error("no clipboard");
      await navigator.clipboard.writeText(url);
      setCopyResult({ ok: true, text: "コピーしました。メールやチャットに貼り付けて送れます。" });
    } catch {
      urlRef.current?.focus();
      urlRef.current?.select();
      setCopyResult({
        ok: false,
        text: "自動でコピーできませんでした。欄のURLを選んだ状態にしたので、Ctrl＋C（Macは ⌘＋C）でコピーしてください。",
      });
    }
  };

  const revoke = async (link: PreviewLink) => {
    if (revokeBusy) return;
    const ok = await confirm({
      title: "このリンクを取り消しますか？",
      message: `取り消すと、このリンクを送った相手も、この${label}のプレビューを見られなくなります。元に戻せません。`,
      confirmLabel: "取り消す",
      danger: true,
    });
    if (!ok) return;
    setRevokeBusy(true);
    setListError(null);
    try {
      await api.deletePreviewLink(link.id);
      setListMessage("リンクを取り消しました。このリンクではもう見られません。");
    } catch (err) {
      if (!isApiError(err, 404)) {
        setListError(errorMessage(err));
        return;
      }
      setListMessage("このリンクは、すでに取り消されているか、期限が切れていました。");
    } finally {
      setRevokeBusy(false);
    }
    if (created?.id === link.id) setCreated(null);
    await refreshList();
    // The button that opened the confirmation is gone; continue from the list heading.
    setTimeout(() => listHeadingRef.current?.focus());
  };

  return (
    <Stack gap="xl">
      <Form onSubmit={create} gap="md">
        {dirty && (
          <Notice kind="warning">
            保存していない変更があります。リンクで見られるのは保存した内容だけなので、リンクを作る前に下書きとして保存します（まだ公開はされません）。
          </Notice>
        )}
        <Radio.Group
          label="有効期間"
          description="期限が過ぎると、リンクは自動で使えなくなります。"
          name={`${id}-days`}
          value={String(days)}
          onChange={(v) => setDays(Number(v))}
        >
          <Group gap="lg" mt="xs">
            {PREVIEW_DAYS.map((d) => (
              <Radio key={d} value={String(d)} label={`${d}日`} />
            ))}
          </Group>
        </Radio.Group>
        {createError && <ErrorMessage>{createError}</ErrorMessage>}
        <Group>
          <Button type="submit" variant="filled" loading={creating}>
            {dirty ? "保存してリンクを作る" : "リンクを作る"}
          </Button>
        </Group>
      </Form>

      {created?.url && (
        <Paper
          component="section"
          aria-labelledby={`${id}-created`}
          withBorder
          bg="green.0"
          p="md"
          style={{ borderColor: "var(--mantine-color-green-3)" }}
        >
          <Stack gap="sm">
            <Title order={3} id={`${id}-created`} c="green.9">
              リンクを作りました（{formatJapanDateTime(created.expiresAt)}まで有効）
            </Title>
            <Group align="flex-end" gap="sm" wrap="wrap">
              <TextInput
                ref={urlRef}
                label="確認用のリンク（URL）"
                description="このURLは今だけ表示されます。あとから見ることはできないので、いまコピーして、見てほしい人に送ってください。"
                readOnly
                value={created.url}
                onFocus={(e) => e.currentTarget.select()}
                style={{ flex: "1 1 20rem", minWidth: 0 }}
              />
              <Button onClick={copy}>コピー</Button>
            </Group>
            <Text
              role="status"
              fw={copyResult?.ok === false ? 700 : undefined}
              c={copyResult?.ok === false ? "red.7" : undefined}
            >
              {copyResult?.text ?? ""}
            </Text>
          </Stack>
        </Paper>
      )}

      <Stack component="section" aria-labelledby={`${id}-list`} gap="sm">
        <Title order={3} id={`${id}-list`} ref={listHeadingRef} tabIndex={-1}>
          有効なリンク
        </Title>
        <Text size="sm" c="dimmed">
          作ったリンクのURLは、あとから表示できません。もう一度送りたいときは、新しいリンクを作ってください。使わなくなったリンクは取り消せます。日時は日本時間です。
        </Text>
        <Text role="status" size="sm" c="green.9">
          {listMessage}
        </Text>
        {listError && <ErrorMessage>{listError}</ErrorMessage>}
        {links.isPending ? (
          <Loading />
        ) : links.error ? (
          <ErrorMessage>{errorMessage(links.error)}</ErrorMessage>
        ) : links.data.length === 0 ? (
          <Text>有効なリンクはありません。</Text>
        ) : (
          <Paper withBorder component="ul" p={0} m={0} style={{ listStyle: "none" }}>
            {links.data.map((l, i) => (
              <Group
                component="li"
                key={l.id}
                justify="space-between"
                gap="sm"
                p="sm"
                style={i > 0 ? { borderTop: "1px solid var(--mantine-color-gray-2)" } : undefined}
              >
                <div>
                  <Text fw={700}>
                    {formatJapanDateTime(l.expiresAt)}まで有効
                    {created?.id === l.id && (
                      <Text component="span" size="sm" fw={400} c="green.9" ml="xs">
                        （いま作ったリンク）
                      </Text>
                    )}
                  </Text>
                  <Text size="sm" c="dimmed">
                    作った日時：{formatJapanDateTime(l.createdAt)}
                  </Text>
                </div>
                <Button
                  size="sm"
                  variant="subtle"
                  color="red"
                  onClick={() => revoke(l)}
                  aria-label={`${formatJapanDateTime(l.expiresAt)}まで有効なリンクを取り消す`}
                >
                  取り消す
                </Button>
              </Group>
            ))}
          </Paper>
        )}
      </Stack>

      <Group justify="flex-end">
        <Button onClick={onClose}>閉じて編集に戻る</Button>
      </Group>
    </Stack>
  );
}
