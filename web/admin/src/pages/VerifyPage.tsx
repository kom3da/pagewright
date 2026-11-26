import { Button, Text } from "@mantine/core";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useNavigate } from "@tanstack/react-router";
import { api, errorMessage } from "../api/client";
import { meQuery } from "../api/queries";
import { AuthLayout } from "../components/AuthLayout";
import { AnchorLink } from "../components/ui/AnchorLink";
import { ErrorMessage } from "../components/ui/feedback";

/**
 * Landing page of the emailed login link. The token is used only when the button is
 * pressed, so mail scanners that open links do not consume it.
 */
export function VerifyPage({ token }: { token: string }) {
  const queryClient = useQueryClient();
  const navigate = useNavigate();
  const verify = useMutation({
    mutationFn: () => api.verify(token),
    onSuccess: (me) => {
      queryClient.setQueryData(meQuery.queryKey, me);
      void navigate({ to: me.currentSiteId ? "/" : "/select-site" });
    },
  });

  if (!token) {
    return (
      <AuthLayout title="ログイン">
        <ErrorMessage>
          ログイン用のリンクが正しくありません。もう一度リンクを送ってください。
        </ErrorMessage>
        <AnchorLink to="/login" activeOptions={{ exact: true }}>
          ログイン画面へ
        </AnchorLink>
      </AuthLayout>
    );
  }

  return (
    <AuthLayout title="ログイン">
      <Text>下のボタンを押すとログインします。</Text>
      {verify.isError && (
        <>
          <ErrorMessage>{errorMessage(verify.error)}</ErrorMessage>
          <AnchorLink to="/login" activeOptions={{ exact: true }}>
            ログイン用のリンクを送り直す
          </AnchorLink>
        </>
      )}
      <Button
        variant="filled"
        fullWidth
        onClick={() => verify.mutate()}
        loading={verify.isPending}
        disabled={verify.isSuccess}
      >
        ログイン
      </Button>
    </AuthLayout>
  );
}
