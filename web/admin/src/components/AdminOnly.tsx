import type { ReactNode } from "react";
import { useIsAdmin } from "../lib/useIsAdmin";
import { Notice } from "./ui/feedback";
import { PageHeader } from "./ui/PageHeader";

/** Shows children to admins; editors get a notice instead (the API would answer 403). */
export function AdminOnly({ title, children }: { title: string; children: ReactNode }) {
  if (useIsAdmin()) return children;
  return (
    <>
      <PageHeader title={title} />
      <Notice>
        この画面は、サイトの管理者だけが使えます。変更が必要なときは、管理者に連絡してください。
      </Notice>
    </>
  );
}
