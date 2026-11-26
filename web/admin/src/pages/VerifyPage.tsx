import { useMutation, useQueryClient } from "@tanstack/react-query";
import { Link, useNavigate } from "@tanstack/react-router";
import { api, errorMessage } from "../api/client";
import { meQuery } from "../api/queries";
import { AuthLayout } from "../components/AuthLayout";
import { Button } from "../components/ui/Button";
import { ErrorMessage } from "../components/ui/misc";

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
        <Link to="/login" activeOptions={{ exact: true }} className="text-blue-700 underline">
          ログイン画面へ
        </Link>
      </AuthLayout>
    );
  }

  return (
    <AuthLayout title="ログイン">
      <p>下のボタンを押すとログインします。</p>
      {verify.isError && (
        <>
          <ErrorMessage>{errorMessage(verify.error)}</ErrorMessage>
          <Link
            to="/login"
            activeOptions={{ exact: true }}
            className="block text-blue-700 underline"
          >
            ログイン用のリンクを送り直す
          </Link>
        </>
      )}
      <Button
        variant="primary"
        className="w-full"
        onClick={() => verify.mutate()}
        disabled={verify.isPending || verify.isSuccess}
      >
        {verify.isPending ? "ログインしています…" : "ログイン"}
      </Button>
    </AuthLayout>
  );
}
