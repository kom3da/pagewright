import { Button, Stack, Text } from "@mantine/core";
import { useMutation, useQueryClient, useSuspenseQuery } from "@tanstack/react-query";
import { useNavigate } from "@tanstack/react-router";
import { api, errorMessage } from "../api/client";
import { meQuery } from "../api/queries";
import { AuthLayout } from "../components/AuthLayout";
import { ErrorMessage } from "../components/ui/feedback";

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
        <Text>
          管理できるサイトがありません。サイトの管理者に、このメールアドレス（{me.user.email}
          ）を招待してもらってください。届いた招待のメールのリンクを開いて「参加する」を押すと、そのサイトを管理できるようになります。
        </Text>
      ) : (
        <>
          <Text>管理するサイトを選んでください。あとから画面の左上で切り替えられます。</Text>
          <Stack component="ul" gap="xs" style={{ listStyle: "none", margin: 0, padding: 0 }}>
            {me.sites.map((site) => (
              <li key={site.id}>
                <Button
                  fullWidth
                  justify="space-between"
                  aria-current={site.id === me.currentSiteId ? "true" : undefined}
                  disabled={select.isPending}
                  onClick={() => select.mutate(site.id)}
                  rightSection={
                    <Text component="span" size="sm" c="dimmed">
                      {roleLabels[site.role]}
                    </Text>
                  }
                >
                  {site.name}
                </Button>
              </li>
            ))}
          </Stack>
        </>
      )}
      {select.isError && <ErrorMessage>{errorMessage(select.error)}</ErrorMessage>}
      {logout.isError && <ErrorMessage>{errorMessage(logout.error)}</ErrorMessage>}
      <Button
        variant="subtle"
        color="gray"
        onClick={() => logout.mutate()}
        disabled={logout.isPending}
      >
        ログアウト
      </Button>
    </AuthLayout>
  );
}
