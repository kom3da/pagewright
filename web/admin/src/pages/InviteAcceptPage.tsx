import { Button, Stack, Text, TextInput } from "@mantine/core";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useNavigate } from "@tanstack/react-router";
import { useState } from "react";
import { api, errorField, errorMessage, isApiError } from "../api/client";
import { meQuery } from "../api/queries";
import { AuthLayout } from "../components/AuthLayout";
import { AnchorLink } from "../components/ui/AnchorLink";
import { Form } from "../components/ui/Form";
import { ErrorMessage, Loading } from "../components/ui/feedback";
import { notify } from "../components/ui/notify";
import { formatJapanDateTime } from "../lib/datetime";
import { roleLabels } from "./MembersPage";

const TITLE = "サイトへの招待";

/**
 * Who is logged in, or null when nobody is. Its own query (not meQuery), so that the 401 of
 * a logged-out visitor is an answer here, not an error that sends them to the login screen.
 */
const currentUserQuery = {
  queryKey: ["me", "optional"],
  queryFn: () =>
    api.me().catch((e) => {
      if (isApiError(e, 401)) return null;
      throw e;
    }),
};

/**
 * Landing page of the emailed invitation link. Like the login link, the token is used only
 * when the button is pressed, so mail scanners that open links do not use it up.
 */
export function InviteAcceptPage({ token }: { token: string }) {
  const queryClient = useQueryClient();
  const navigate = useNavigate();
  const [name, setName] = useState("");
  const invitation = useQuery({
    queryKey: ["invitation", token],
    queryFn: () => api.lookupInvitation(token),
    enabled: token !== "",
  });
  const current = useQuery(currentUserQuery);

  const accept = useMutation({
    mutationFn: () => api.acceptInvitation(token, name),
    onSuccess: (me) => {
      queryClient.clear();
      queryClient.setQueryData(meQuery.queryKey, me);
      void navigate({ to: "/" });
      const site = me.sites.find((s) => s.id === me.currentSiteId);
      notify(`「${site?.name ?? invitation.data?.siteName}」に参加しました。`);
    },
  });

  const logout = useMutation({
    mutationFn: api.logout,
    onSuccess: () => {
      accept.reset();
      queryClient.setQueryData(currentUserQuery.queryKey, null);
    },
  });

  if (!token) {
    return (
      <AuthLayout title={TITLE}>
        <ErrorMessage>
          招待のリンクが正しくありません。メールのリンクをもう一度開いてください。
        </ErrorMessage>
      </AuthLayout>
    );
  }
  if (invitation.isPending || current.isPending) {
    return (
      <AuthLayout title={TITLE}>
        <Loading />
      </AuthLayout>
    );
  }
  if (invitation.error || current.error) {
    return (
      <AuthLayout title={TITLE}>
        <ErrorMessage>{errorMessage(invitation.error ?? current.error)}</ErrorMessage>
        <AnchorLink to="/login" activeOptions={{ exact: true }}>
          ログイン画面へ
        </AnchorLink>
      </AuthLayout>
    );
  }

  const inv = invitation.data;
  const user = current.data;
  if (inv.expired) {
    return (
      <AuthLayout title={TITLE}>
        <ErrorMessage>
          「{inv.siteName}」への招待は、有効期限（{formatJapanDateTime(inv.expiresAt)}
          ）が切れています。招待した人に、もう一度送ってもらってください。
        </ErrorMessage>
      </AuthLayout>
    );
  }

  const wrongAccount =
    (user && user.user.email !== inv.email) || isApiError(accept.error, 409, "wrong_account");
  if (wrongAccount) {
    return (
      <AuthLayout title={TITLE}>
        <Text>
          この招待は <strong>{inv.email}</strong> あてです。いまは別のメールアドレス（
          {user?.user.email}）でログインしています。
        </Text>
        <Text>
          招待されたメールアドレスで参加するには、ログアウトしてから、この画面で「参加する」を押してください。
        </Text>
        {logout.isError && <ErrorMessage>{errorMessage(logout.error)}</ErrorMessage>}
        <Button variant="filled" onClick={() => logout.mutate()} loading={logout.isPending}>
          ログアウトする
        </Button>
      </AuthLayout>
    );
  }

  const askName = !user && !inv.hasAccount;
  const nameError = errorField(accept.error) === "displayName" ? errorMessage(accept.error) : null;
  return (
    <AuthLayout title={TITLE}>
      <Stack gap="xs">
        <Text>
          サイト「<strong>{inv.siteName}</strong>」の管理画面に、{roleLabels[inv.role]}
          として招待されています。
        </Text>
        <Text size="sm" c="dimmed">
          招待されたメールアドレス：{inv.email}
          <br />
          この招待のリンクは {formatJapanDateTime(inv.expiresAt)} まで使えます。
        </Text>
      </Stack>
      <Form
        onSubmit={(e) => {
          e.preventDefault();
          accept.mutate();
        }}
        gap="md"
      >
        {askName && (
          <TextInput
            label="あなたの名前"
            description="管理画面で、ほかのメンバーに見える名前です。空のままでも参加でき、あとからアカウントの画面で変えられます。例：山田 太郎"
            autoComplete="name"
            maxLength={50}
            value={name}
            onChange={(e) => setName(e.currentTarget.value)}
            error={nameError}
          />
        )}
        {accept.isError && !nameError && <ErrorMessage>{errorMessage(accept.error)}</ErrorMessage>}
        <Button
          type="submit"
          variant="filled"
          fullWidth
          loading={accept.isPending}
          disabled={accept.isSuccess}
        >
          参加する
        </Button>
      </Form>
      <Text size="sm" c="dimmed">
        心当たりがない場合は、この画面を閉じてください。「参加する」を押さなければ、何も起きません。
      </Text>
    </AuthLayout>
  );
}
