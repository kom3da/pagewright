import { Button, Text } from "@mantine/core";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { Link, useNavigate } from "@tanstack/react-router";
import { api, errorMessage, isApiError } from "../api/client";
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

  // A used or expired link cannot log in again: sending a new one is all that is left.
  if (isApiError(verify.error, 400, "invalid_token")) {
    return (
      <AuthLayout title="ログイン">
        <ErrorMessage>{errorMessage(verify.error)}</ErrorMessage>
        <Button component={Link} to="/login" variant="filled" fullWidth>
          ログイン用のリンクを送り直す
        </Button>
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
