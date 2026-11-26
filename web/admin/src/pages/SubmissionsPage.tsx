import {
  useInfiniteQuery,
  useMutation,
  useQueryClient,
  useSuspenseQuery,
} from "@tanstack/react-query";
import { Mail } from "lucide-react";
import { useState } from "react";
import { api, errorMessage } from "../api/client";
import { meQuery, submissionsQuery } from "../api/queries";
import type { Submission } from "../api/types";
import { AdminOnly } from "../components/AdminOnly";
import { useNotify } from "../components/Toaster";
import { Button, buttonClass } from "../components/ui/Button";
import { ConfirmDialog, Dialog } from "../components/ui/Dialog";
import { ErrorMessage, formatDateTime, Loading, PageHeading } from "../components/ui/misc";

const TITLE = "お問い合わせ";

const firstLine = (s: string) => {
  const line = s.split(/\r?\n/).find((l) => l.trim()) ?? "";
  return line.length > 40 ? `${line.slice(0, 40)}…` : line;
};

export function SubmissionsPage() {
  return (
    <AdminOnly title={TITLE}>
      <Submissions />
    </AdminOnly>
  );
}

function Submissions() {
  // Personal data: kept in memory only while shown (see submissionsQuery), never in storage.
  const query = useInfiniteQuery(submissionsQuery);
  const { isPending, error } = query;
  const data = query.data?.pages.flat() ?? [];
  const queryClient = useQueryClient();
  const notify = useNotify();
  const [open, setOpen] = useState<Submission | null>(null);
  const [deleting, setDeleting] = useState<Submission | null>(null);

  const remove = useMutation({
    mutationFn: (s: Submission) => api.deleteSubmission(s.id),
    onSuccess: (_, s) => {
      setDeleting(null);
      setOpen(null);
      // Not refetched: that would reload every page already shown.
      queryClient.setQueryData(
        submissionsQuery.queryKey,
        (d) => d && { ...d, pages: d.pages.map((p) => p.filter((x) => x.id !== s.id)) },
      );
      notify(`${s.name} さんからのお問い合わせを削除しました。`);
    },
    onError: (e) => {
      setDeleting(null);
      notify(errorMessage(e), "error");
    },
  });

  return (
    <div className="space-y-6">
      <PageHeading title={TITLE}>{TITLE}</PageHeading>
      <p>
        公開サイトのお問い合わせフォームから届いた内容です（新しい順）。届くたびに、サイトの設定で決めた送り先にもメールで知らせます。
      </p>
      <p className="rounded-md border border-amber-700 bg-amber-50 p-3 text-amber-900">
        お名前やメールアドレスなどの個人情報が含まれます。取り扱いに注意し、対応が済んで不要になったものは削除してください。
      </p>
      {isPending ? (
        <Loading />
      ) : error ? (
        <ErrorMessage>{errorMessage(error)}</ErrorMessage>
      ) : data.length === 0 ? (
        <p>
          まだお問い合わせはありません。受け付けるには、固定ページの本文にツールバーの「お問い合わせフォームを入れる」でフォームを入れて公開してください。
        </p>
      ) : (
        <div className="overflow-x-auto rounded-lg border border-slate-300 bg-white">
          <table className="w-full text-left">
            <caption className="sr-only">お問い合わせの一覧（{data.length}件を表示）</caption>
            <thead className="border-b border-slate-300 bg-slate-100 text-sm">
              <tr>
                <th scope="col" className="px-4 py-2 whitespace-nowrap">
                  受け付けた日時
                </th>
                <th scope="col" className="px-4 py-2">
                  お名前
                </th>
                <th scope="col" className="px-4 py-2">
                  メールアドレス
                </th>
                <th scope="col" className="px-4 py-2">
                  内容
                </th>
                <th scope="col" className="px-4 py-2">
                  <span className="sr-only">操作</span>
                </th>
              </tr>
            </thead>
            <tbody>
              {data.map((s) => (
                <tr key={s.id} className="border-b border-slate-200 last:border-0">
                  <td className="px-4 py-2 whitespace-nowrap">{formatDateTime(s.createdAt)}</td>
                  <td className="px-4 py-2">{s.name}</td>
                  <td className="px-4 py-2 break-all">{s.email}</td>
                  <td className="px-4 py-2">{firstLine(s.message)}</td>
                  <td className="px-4 py-2">
                    <Button
                      onClick={() => setOpen(s)}
                      aria-label={`${s.name} さんからのお問い合わせを開く`}
                    >
                      開く
                    </Button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
      {query.hasNextPage && (
        <div className="space-y-2">
          <Button onClick={() => query.fetchNextPage()} disabled={query.isFetchingNextPage}>
            {query.isFetchingNextPage ? "読み込んでいます…" : "もっと見る"}
          </Button>
          <p className="text-sm text-slate-700">
            {data.length}件を表示しています。もっと前のお問い合わせを読み込みます。
          </p>
        </div>
      )}
      {query.isFetchNextPageError && <ErrorMessage>{errorMessage(query.error)}</ErrorMessage>}

      <Dialog
        open={open !== null}
        onOpenChange={(o) => !o && setOpen(null)}
        title="お問い合わせの内容"
        wide
      >
        {open && <SubmissionDetail s={open} onDelete={() => setDeleting(open)} />}
      </Dialog>

      <ConfirmDialog
        open={deleting !== null}
        onOpenChange={(o) => !o && setDeleting(null)}
        title="このお問い合わせを削除しますか？"
        description={`${deleting?.name ?? ""} さん（${formatDateTime(deleting?.createdAt ?? null)}）からのお問い合わせを削除します。元に戻せません。`}
        confirmLabel="削除する"
        danger
        busy={remove.isPending}
        onConfirm={() => deleting && remove.mutate(deleting)}
      />
    </div>
  );
}

function SubmissionDetail({ s, onDelete }: { s: Submission; onDelete: () => void }) {
  const { data: me } = useSuspenseQuery(meQuery);
  const siteName = me.sites.find((x) => x.id === me.currentSiteId)?.name ?? "";
  const subject = `${siteName ? `【${siteName}】` : ""}お問い合わせありがとうございます`;
  const rows: [string, string][] = [
    ["受け付けた日時", formatDateTime(s.createdAt)],
    ["お名前", s.name],
    ["ふりがな", s.kana || "（入力なし）"],
    ["メールアドレス", s.email],
    ["電話番号", s.phone || "（入力なし）"],
    ["送信したページ", s.pagePath],
  ];
  return (
    <div className="space-y-4">
      <dl className="grid gap-x-4 gap-y-2 sm:grid-cols-[10rem_1fr]">
        {rows.map(([k, v]) => (
          <div key={k} className="contents">
            <dt className="font-semibold">{k}</dt>
            <dd className="break-all">{v}</dd>
          </div>
        ))}
      </dl>
      <div>
        <h3 className="font-semibold">お問い合わせ内容</h3>
        <p className="mt-1 rounded-md border border-slate-300 bg-slate-50 p-3 whitespace-pre-wrap">
          {s.message}
        </p>
      </div>
      <div className="flex flex-wrap gap-3">
        <a
          // Encoded so that characters such as "?" in the address cannot add mailto fields.
          href={`mailto:${encodeURIComponent(s.email).replace("%40", "@")}?subject=${encodeURIComponent(subject)}`}
          className={buttonClass("primary")}
        >
          <Mail aria-hidden="true" size={16} />
          メールで返信する
        </a>
        <Button variant="ghost" className="ml-auto text-red-700" onClick={onDelete}>
          このお問い合わせを削除
        </Button>
      </div>
      <p className="text-sm text-slate-700">
        「メールで返信する」を押すと、お使いのメールソフトが開きます。
      </p>
    </div>
  );
}
