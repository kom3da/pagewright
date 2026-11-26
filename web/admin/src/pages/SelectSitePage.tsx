import { useMutation, useQueryClient, useSuspenseQuery } from "@tanstack/react-query";
import { useNavigate } from "@tanstack/react-router";
import { api, errorMessage } from "../api/client";
import { meQuery } from "../api/queries";
import { AuthLayout } from "../components/AuthLayout";
import { Button } from "../components/ui/Button";
import { ErrorMessage } from "../components/ui/misc";

const roleLabels = { admin: "管理者", editor: "編集者" } as const;

export function SelectSitePage() {
  const { data: me } = useSuspenseQuery(meQuery);
  const queryClient = useQueryClient();
  const navigate = useNavigate();

  const select = useMutation({
    mutationFn: api.selectSite,
    onSuccess: (next) => {
      queryClient.clear();
      queryClient.setQueryData(meQuery.queryKey, next);
      void navigate({ to: "/" });
    },
  });

  const logout = useMutation({
    mutationFn: api.logout,
    onSuccess: async () => {
      await navigate({ to: "/login" });
      queryClient.clear();
    },
  });

  return (
    <AuthLayout title="サイトを選ぶ">
      {me.sites.length === 0 ? (
        <p>
          管理できるサイトがありません。サイトの管理者に、このメールアドレス（{me.user.email}
          ）を招待してもらってください。
        </p>
      ) : (
        <>
          <p>管理するサイトを選んでください。あとから画面上部で切り替えられます。</p>
          <ul className="space-y-2">
            {me.sites.map((site) => (
              <li key={site.id}>
                <Button
                  className="w-full justify-between"
                  aria-current={site.id === me.currentSiteId ? "true" : undefined}
                  disabled={select.isPending}
                  onClick={() => select.mutate(site.id)}
                >
                  <span>{site.name}</span>
                  <span className="text-sm font-normal text-slate-700">
                    {roleLabels[site.role]}
                  </span>
                </Button>
              </li>
            ))}
          </ul>
        </>
      )}
      {select.isError && <ErrorMessage>{errorMessage(select.error)}</ErrorMessage>}
      {logout.isError && <ErrorMessage>{errorMessage(logout.error)}</ErrorMessage>}
      <Button variant="ghost" onClick={() => logout.mutate()} disabled={logout.isPending}>
        ログアウト
      </Button>
    </AuthLayout>
  );
}
