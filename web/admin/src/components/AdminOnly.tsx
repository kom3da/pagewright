import type { ReactNode } from "react";
import { useIsAdmin } from "../lib/useIsAdmin";
import { PageHeading } from "./ui/misc";

/** Shows children to admins; editors get a notice instead (the API would answer 403). */
export function AdminOnly({ title, children }: { title: string; children: ReactNode }) {
  if (useIsAdmin()) return children;
  return (
    <div className="space-y-4">
      <PageHeading title={title}>{title}</PageHeading>
      <p className="rounded-md border border-slate-400 bg-white p-4">
        この画面は、サイトの管理者だけが使えます。変更が必要なときは、管理者に連絡してください。
      </p>
    </div>
  );
}
