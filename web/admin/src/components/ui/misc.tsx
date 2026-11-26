import { clsx } from "clsx";
import { type ReactNode, useEffect, useRef } from "react";
import type { ContentStatus } from "../../api/types";
import { appName } from "../../config";

export function ErrorMessage({ children }: { children: ReactNode }) {
  if (!children) return null;
  return (
    <div role="alert" className="rounded-md border border-red-700 bg-red-50 p-3 text-red-800">
      {children}
    </div>
  );
}

export function Loading({ label = "読み込んでいます…" }: { label?: string }) {
  return (
    <p role="status" className="py-6 text-slate-700">
      {label}
    </p>
  );
}

const statusLabels: Record<ContentStatus, string> = {
  draft: "下書き",
  published: "公開中",
  scheduled: "予約",
};

const statusStyles: Record<ContentStatus, string> = {
  draft: "border-slate-500 bg-slate-100 text-slate-800",
  published: "border-green-700 bg-green-50 text-green-800",
  scheduled: "border-amber-700 bg-amber-50 text-amber-900",
};

export function StatusBadge({ status }: { status: ContentStatus }) {
  return (
    <span
      className={clsx(
        "inline-block rounded-full border px-2.5 py-0.5 text-sm font-semibold whitespace-nowrap",
        statusStyles[status],
      )}
    >
      {statusLabels[status]}
    </span>
  );
}

export function UnpublishedBadge() {
  return (
    <span className="inline-block rounded-full border border-blue-700 bg-blue-50 px-2.5 py-0.5 text-sm font-semibold whitespace-nowrap text-blue-800">
      未公開の変更あり
    </span>
  );
}

export function ScheduledUpdateBadge() {
  return (
    <span className="inline-block rounded-full border border-amber-700 bg-amber-50 px-2.5 py-0.5 text-sm font-semibold whitespace-nowrap text-amber-900">
      更新を予約
    </span>
  );
}

// Focus moves to the page heading after client-side navigation so screen reader users
// know the page changed. The first page load keeps the browser default.
let firstHeading = true;

/** The page's h1. Also sets document.title. */
export function PageHeading({ children, title }: { children: ReactNode; title: string }) {
  const ref = useRef<HTMLHeadingElement>(null);
  useEffect(() => {
    document.title = `${title} | ${appName}`;
    if (firstHeading) {
      firstHeading = false;
      return;
    }
    ref.current?.focus();
  }, [title]);
  return (
    <h1 ref={ref} tabIndex={-1} className="text-2xl font-bold focus:outline-none">
      {children}
    </h1>
  );
}

export const formatDateTime = (iso: string | null) => {
  if (!iso) return "";
  return new Intl.DateTimeFormat("ja-JP", {
    dateStyle: "medium",
    timeStyle: "short",
    timeZone: "Asia/Tokyo",
  }).format(new Date(iso));
};
