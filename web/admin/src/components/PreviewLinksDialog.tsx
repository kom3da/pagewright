import { useQuery, useQueryClient } from "@tanstack/react-query";
import { type FormEvent, useId, useRef, useState } from "react";
import { api, errorMessage, isApiError } from "../api/client";
import { previewLinksQuery } from "../api/queries";
import type { PreviewLink } from "../api/types";
import { formatJapanDateTime } from "../lib/datetime";
import { Button } from "./ui/Button";
import { ConfirmDialog, Dialog } from "./ui/Dialog";
import { Field, inputClass } from "./ui/Field";
import { ErrorMessage, Loading } from "./ui/misc";

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
      title="プレビュー用のリンク"
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

  const [revoking, setRevoking] = useState<PreviewLink | null>(null);
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

  const revoke = async () => {
    const link = revoking;
    if (!link || revokeBusy) return;
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
      setRevoking(null);
    }
    if (created?.id === link.id) setCreated(null);
    await refreshList();
    // The button that opened the confirmation is gone; continue from the list heading.
    setTimeout(() => listHeadingRef.current?.focus());
  };

  const daysHintId = `${id}-days-hint`;

  return (
    <div className="space-y-8">
      <form noValidate onSubmit={create} className="space-y-4">
        {dirty && (
          <p className="rounded-md border border-amber-700 bg-amber-50 px-3 py-2 text-amber-900">
            保存していない変更があります。リンクで見られるのは保存した内容だけなので、リンクを作る前に下書きとして保存します（まだ公開はされません）。
          </p>
        )}
        <fieldset className="space-y-2" aria-describedby={daysHintId}>
          <legend className="font-semibold">有効期間</legend>
          <p id={daysHintId} className="text-sm text-slate-700">
            期限が過ぎると、リンクは自動で使えなくなります。
          </p>
          <div className="flex flex-wrap gap-x-6 gap-y-2">
            {PREVIEW_DAYS.map((d) => (
              <label key={d} className="inline-flex min-h-10 items-center gap-2">
                <input
                  type="radio"
                  name={`${id}-days`}
                  value={d}
                  checked={days === d}
                  onChange={() => setDays(d)}
                  className="size-5"
                />
                {d}日
              </label>
            ))}
          </div>
        </fieldset>
        {createError && <ErrorMessage>{createError}</ErrorMessage>}
        <Button type="submit" variant="primary" disabled={creating}>
          {creating ? "リンクを作っています…" : dirty ? "保存してリンクを作る" : "リンクを作る"}
        </Button>
      </form>

      {created?.url && (
        <section
          aria-labelledby={`${id}-created`}
          className="space-y-3 rounded-md border border-green-700 bg-green-50 p-4"
        >
          <h3 id={`${id}-created`} className="font-semibold text-green-900">
            リンクを作りました（{formatJapanDateTime(created.expiresAt)}まで有効）
          </h3>
          <div className="flex flex-wrap items-end gap-3">
            <Field
              label="プレビュー用のリンク（URL）"
              hint="このURLは今だけ表示されます。あとから見ることはできないので、いまコピーして、見てほしい人に送ってください。"
              className="min-w-0 flex-1 basis-80"
            >
              {(ids) => (
                <input
                  {...ids}
                  ref={urlRef}
                  readOnly
                  value={created.url}
                  onFocus={(e) => e.currentTarget.select()}
                  className={inputClass}
                />
              )}
            </Field>
            <Button onClick={copy}>コピー</Button>
          </div>
          <p
            role="status"
            className={copyResult?.ok === false ? "font-semibold text-red-700" : "text-green-900"}
          >
            {copyResult?.text ?? ""}
          </p>
        </section>
      )}

      <section aria-labelledby={`${id}-list`} className="space-y-3">
        <h3
          id={`${id}-list`}
          ref={listHeadingRef}
          tabIndex={-1}
          className="text-lg font-bold focus:outline-none"
        >
          有効なリンク
        </h3>
        <p className="text-sm text-slate-700">
          作ったリンクのURLは、あとから表示できません。もう一度送りたいときは、新しいリンクを作ってください。使わなくなったリンクは取り消せます。日時は日本時間です。
        </p>
        <p role="status" className="text-sm text-green-900">
          {listMessage}
        </p>
        {listError && <ErrorMessage>{listError}</ErrorMessage>}
        {links.isPending ? (
          <Loading />
        ) : links.error ? (
          <ErrorMessage>{errorMessage(links.error)}</ErrorMessage>
        ) : links.data.length === 0 ? (
          <p>有効なリンクはありません。</p>
        ) : (
          <ul className="divide-y divide-slate-300 rounded-md border border-slate-300">
            {links.data.map((l) => (
              <li key={l.id} className="flex flex-wrap items-center justify-between gap-3 p-3">
                <div>
                  <p className="font-semibold">
                    {formatJapanDateTime(l.expiresAt)}まで有効
                    {created?.id === l.id && (
                      <span className="ml-2 text-sm font-normal text-green-900">
                        （いま作ったリンク）
                      </span>
                    )}
                  </p>
                  <p className="text-sm text-slate-700">
                    作った日時：{formatJapanDateTime(l.createdAt)}
                  </p>
                </div>
                <Button
                  onClick={() => setRevoking(l)}
                  aria-label={`${formatJapanDateTime(l.expiresAt)}まで有効なリンクを取り消す`}
                >
                  取り消す
                </Button>
              </li>
            ))}
          </ul>
        )}
      </section>

      <div className="flex justify-end">
        <Button onClick={onClose}>閉じて編集に戻る</Button>
      </div>

      <ConfirmDialog
        open={revoking !== null}
        onOpenChange={(o) => !o && setRevoking(null)}
        title="このリンクを取り消しますか？"
        description={`取り消すと、このリンクを送った相手も、この${label}のプレビューを見られなくなります。元に戻せません。`}
        confirmLabel="取り消す"
        danger
        busy={revokeBusy}
        onConfirm={revoke}
      />
    </div>
  );
}
