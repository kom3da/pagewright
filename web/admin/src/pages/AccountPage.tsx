import { Badge, Button, Group, Stack, TextInput } from "@mantine/core";
import { useMutation, useQuery, useQueryClient, useSuspenseQuery } from "@tanstack/react-query";
import { LogOut } from "lucide-react";
import { useState } from "react";
import { api, errorField, errorMessage } from "../api/client";
import { meQuery, sessionsQuery } from "../api/queries";
import type { Session } from "../api/types";
import { confirm } from "../components/ui/confirm";
import { Form } from "../components/ui/Form";
import { ErrorMessage, Loading } from "../components/ui/feedback";
import { notify } from "../components/ui/notify";
import { PageHeader } from "../components/ui/PageHeader";
import { ResponsiveList } from "../components/ui/ResponsiveList";
import { Section } from "../components/ui/Section";
import { formatDateTime } from "../lib/datetime";

const LIST_HEADING_ID = "sessions-heading";
const revokeButtonId = (id: string) => `session-revoke-${id}`;
const deviceName = (s: Session) => s.device || "名前のわからない端末";

/** Where focus goes when a row's button disappears: the next row, the previous one, or the heading. */
function focusAfterRemoval(list: Session[], id: string) {
  const others = list.filter((s) => !s.current);
  const i = others.findIndex((s) => s.id === id);
  const next = others[i + 1] ?? others[i - 1];
  const el = next && document.getElementById(revokeButtonId(next.id));
  if (el) {
    el.focus();
    return;
  }
  const heading = document.getElementById(LIST_HEADING_ID);
  heading?.setAttribute("tabindex", "-1");
  heading?.focus();
}

export function AccountPage() {
  const { data: me } = useSuspenseQuery(meQuery);
  const { data, isPending, error } = useQuery(sessionsQuery);
  const queryClient = useQueryClient();
  const sessions = data ?? [];
  const others = sessions.filter((s) => !s.current);

  const revoke = useMutation({
    mutationFn: (s: Session) => api.revokeSession(s.id),
    onSuccess: (_, s) => {
      focusAfterRemoval(sessions, s.id);
      queryClient.setQueryData<Session[]>(sessionsQuery.queryKey, (d) =>
        d?.filter((x) => x.id !== s.id),
      );
      notify(`「${deviceName(s)}」をログアウトさせました。`);
    },
    onError: (e) => {
      notify(errorMessage(e), "error");
      void queryClient.invalidateQueries({ queryKey: sessionsQuery.queryKey });
    },
  });

  const logoutOthers = useMutation({
    mutationFn: api.logoutOtherSessions,
    onSuccess: ({ revoked }) => {
      focusAfterRemoval([], "");
      queryClient.setQueryData<Session[]>(sessionsQuery.queryKey, (d) =>
        d?.filter((x) => x.current),
      );
      notify(
        revoked > 0
          ? `ほかの端末（${revoked}台）からログアウトしました。この端末はログインしたままです。`
          : "ほかにログイン中の端末はありませんでした。",
      );
    },
    onError: (e) => notify(errorMessage(e), "error"),
  });

  const askRevoke = async (s: Session) => {
    const ok = await confirm({
      title: "この端末をログアウトさせますか？",
      message: `「${deviceName(s)}」（最後に使った日時：${formatDateTime(s.lastUsedAt)}）はログアウトします。その端末で管理画面を使うには、もう一度ログインが必要です。`,
      confirmLabel: "ログアウトさせる",
      danger: true,
    });
    if (ok) revoke.mutate(s);
  };

  const askLogoutOthers = async () => {
    const ok = await confirm({
      title: "ほかの端末からログアウトしますか？",
      message: `この端末のほかの${others.length}台はログアウトします。それらの端末で管理画面を使うには、もう一度ログインが必要です。この端末はログインしたままです。`,
      confirmLabel: "ログアウトする",
      danger: true,
    });
    if (ok) logoutOthers.mutate();
  };

  return (
    <>
      <PageHeader title="アカウント" description={`${me.user.email} でログインしています。`} />
      <Stack gap="xl">
        <NameSection />
        <Section
          id={LIST_HEADING_ID}
          title="ログイン中の端末"
          variant="plain"
          description="このアカウントでログインしているパソコンやスマートフォンです。ログインは、ログインした日から30日で切れます。使っていない端末や、心当たりのない端末はログアウトさせてください。"
        >
          {isPending ? (
            <Loading />
          ) : error ? (
            <ErrorMessage>{errorMessage(error)}</ErrorMessage>
          ) : (
            <ResponsiveList
              items={sessions}
              getKey={(s) => s.id}
              caption={`ログイン中の端末（${sessions.length}台）`}
              columns={[
                {
                  key: "device",
                  header: "端末",
                  render: (s) => (
                    <>
                      {deviceName(s)}
                      {s.current && <Badge ml="xs">この端末</Badge>}
                    </>
                  ),
                },
                {
                  key: "lastUsedAt",
                  header: "最後に使った日時",
                  nowrap: true,
                  render: (s) => (s.current ? "いま使用中" : formatDateTime(s.lastUsedAt)),
                },
                {
                  key: "createdAt",
                  header: "ログインした日時",
                  nowrap: true,
                  render: (s) => formatDateTime(s.createdAt),
                },
              ]}
              actions={(s) =>
                s.current ? null : (
                  <Button
                    id={revokeButtonId(s.id)}
                    size="sm"
                    variant="subtle"
                    color="red"
                    onClick={() => askRevoke(s)}
                    loading={revoke.isPending && revoke.variables?.id === s.id}
                    aria-label={`「${deviceName(s)}」（最後に使った日時：${formatDateTime(s.lastUsedAt)}）をログアウトさせる`}
                  >
                    ログアウトさせる
                  </Button>
                )
              }
            />
          )}
        </Section>
        {others.length > 0 && (
          <Section
            title="ほかの端末からログアウト"
            description="パソコンやスマートフォンをなくしたときや、ほかの人に渡すとき、共用のパソコンでログインしたままにしたときに使います。この端末のほかは、すべてログアウトします。"
          >
            <Group>
              <Button
                color="red"
                leftSection={<LogOut aria-hidden="true" size={16} />}
                onClick={askLogoutOthers}
                loading={logoutOthers.isPending}
              >
                ほかの端末からログアウト
              </Button>
            </Group>
          </Section>
        )}
      </Stack>
    </>
  );
}

/** The user's own display name, shown to the other members of every site they belong to. */
function NameSection() {
  const { data: me } = useSuspenseQuery(meQuery);
  const queryClient = useQueryClient();
  const [name, setName] = useState(me.user.displayName);
  const save = useMutation({
    mutationFn: () => api.setDisplayName(name.trim()),
    onSuccess: (next) => {
      queryClient.setQueryData(meQuery.queryKey, next);
      setName(next.user.displayName);
      notify(
        next.user.displayName
          ? "名前を保存しました。"
          : "名前を消しました。名前の代わりにメールアドレスが表示されます。",
      );
    },
  });
  const fieldError =
    errorField(save.error) === "displayName" ? errorMessage(save.error) : undefined;

  return (
    <Section
      title="名前"
      description="管理画面で、ほかのメンバーに見える名前です。あなたが参加しているすべてのサイトで同じ名前になります。"
    >
      <Form
        onSubmit={(e) => {
          e.preventDefault();
          save.mutate();
        }}
        gap="md"
      >
        <TextInput
          label="あなたの名前"
          description="例：山田 太郎。空にすると、名前の代わりにメールアドレスが表示されます。"
          autoComplete="name"
          maxLength={50}
          maw="28rem"
          value={name}
          onChange={(e) => setName(e.currentTarget.value)}
          error={fieldError}
        />
        {save.isError && !fieldError && <ErrorMessage>{errorMessage(save.error)}</ErrorMessage>}
        <Group>
          <Button type="submit" variant="filled" loading={save.isPending}>
            名前を保存する
          </Button>
        </Group>
      </Form>
    </Section>
  );
}
