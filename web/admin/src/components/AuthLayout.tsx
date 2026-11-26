import type { ReactNode } from "react";
import { PageHeading } from "./ui/misc";

/** Simple centered layout for screens shown before login. */
export function AuthLayout({ title, children }: { title: string; children: ReactNode }) {
  return (
    <main id="main" className="mx-auto mt-16 max-w-md px-4">
      <p className="mb-4 text-center font-bold text-slate-700">Pagewright</p>
      <div className="space-y-5 rounded-lg border border-slate-300 bg-white p-6 shadow-sm">
        <PageHeading title={title}>{title}</PageHeading>
        {children}
      </div>
    </main>
  );
}
