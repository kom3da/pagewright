import { useQuery } from "@tanstack/react-query";
import { Link } from "@tanstack/react-router";
import { ExternalLink } from "lucide-react";
import { errorMessage } from "../api/client";
import { contentsQuery } from "../api/queries";
import type { ContentType } from "../api/types";
import { NewContentButton } from "../components/NewContentButton";
import {
  ErrorMessage,
  formatDateTime,
  Loading,
  PageHeading,
  StatusBadge,
  UnpublishedBadge,
} from "../components/ui/misc";
import { publicUrl } from "../config";

const labels = {
  post: { title: "記事", empty: "まだ記事がありません。「新規作成」から書き始めましょう。" },
  page: {
    title: "固定ページ",
    empty: "まだ固定ページがありません。「新規作成」から作りましょう。",
  },
} as const;

export function ContentListPage({ type }: { type: ContentType }) {
  const { data, isPending, error } = useQuery(contentsQuery(type));
  const label = labels[type];

  return (
    <div className="space-y-6">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <PageHeading title={`${label.title}の一覧`}>{label.title}の一覧</PageHeading>
        <NewContentButton type={type} label="新規作成" />
      </div>
      {isPending ? (
        <Loading />
      ) : error ? (
        <ErrorMessage>{errorMessage(error)}</ErrorMessage>
      ) : data.length === 0 ? (
        <p>{label.empty}</p>
      ) : (
        <div className="overflow-x-auto rounded-lg border border-slate-300 bg-white">
          <table className="w-full text-left">
            <caption className="sr-only">{label.title}の一覧（番号の新しい順）</caption>
            <thead className="border-b border-slate-300 bg-slate-100 text-sm">
              <tr>
                <th scope="col" className="px-4 py-2">
                  タイトル
                </th>
                <th scope="col" className="px-4 py-2">
                  状態
                </th>
                <th scope="col" className="px-4 py-2">
                  公開中のURL
                </th>
                <th scope="col" className="px-4 py-2">
                  最終更新
                </th>
              </tr>
            </thead>
            <tbody>
              {data.map((c) => (
                <tr key={c.id} className="border-b border-slate-200 last:border-0">
                  <td className="px-4 py-3">
                    <Link
                      to={type === "post" ? "/posts/$id" : "/pages/$id"}
                      params={{ id: String(c.id) }}
                      className="font-semibold text-blue-700 underline"
                    >
                      {c.title || "（タイトルなし）"}
                    </Link>
                  </td>
                  <td className="px-4 py-3">
                    <div className="flex flex-wrap gap-1">
                      <StatusBadge status={c.status} />
                      {c.hasUnpublishedChanges && c.status !== "draft" && <UnpublishedBadge />}
                    </div>
                  </td>
                  <td className="px-4 py-3 text-sm">
                    {c.url ? (
                      <a
                        href={publicUrl(c.url)}
                        target="_blank"
                        rel="noopener noreferrer"
                        className="inline-flex items-center gap-1 break-all text-blue-700 underline"
                      >
                        {c.url}
                        <ExternalLink aria-hidden="true" size={14} />
                        <span className="sr-only">（新しいタブで開きます）</span>
                      </a>
                    ) : (
                      <span className="text-slate-700">未公開</span>
                    )}
                  </td>
                  <td className="px-4 py-3 text-sm whitespace-nowrap">
                    {formatDateTime(c.updatedAt)}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  );
}
