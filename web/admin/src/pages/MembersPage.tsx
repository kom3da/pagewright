import { zodResolver } from "@hookform/resolvers/zod";
import { Badge, Button, Group, Radio, Stack, Text, TextInput } from "@mantine/core";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useNavigate } from "@tanstack/react-router";
import { UserPlus } from "lucide-react";
import { useId, useState } from "react";
import { Controller, useForm } from "react-hook-form";
import { z } from "zod";
import { api, errorField, errorMessage, isApiError } from "../api/client";
import { invitationsQuery, membersQuery, meQuery } from "../api/queries";
import type { Invitation, Member, Role } from "../api/types";
import { AdminOnly } from "../components/AdminOnly";
import { confirm } from "../components/ui/confirm";
import { Dialog } from "../components/ui/Dialog";
import { Form } from "../components/ui/Form";
import { ErrorMessage, Loading } from "../components/ui/feedback";
import { notify } from "../components/ui/notify";
import { PageHeader } from "../components/ui/PageHeader";
import { ResponsiveList } from "../components/ui/ResponsiveList";
import { Section } from "../components/ui/Section";
import { StatusBadge } from "../components/ui/StatusBadge";
import { formatDate, formatDateTime } from "../lib/datetime";

const TITLE = "メンバー";
const MEMBERS_HEADING_ID = "members-heading";
const INVITATIONS_HEADING_ID = "invitations-heading";

export const roleLabels: Record<Role, string> = { admin: "管理者", editor: "編集者" };

const roleDescriptions: Record<Role, string> = {
  editor: "記事・固定ページ・カテゴリ・タグ・画像を扱えます。",
  admin: "編集者のできることに加えて、サイトの設定、お問い合わせ、メンバーの招待と変更ができます。",
};

const memberName = (m: Member) => m.displayName || m.email;

/** Moves focus to a list's heading after the focused row's button went away. */
function focusHeading(id: string) {
  const heading = document.getElementById(id);
  heading?.setAttribute("tabindex", "-1");
  heading?.focus();
}

export function MembersPage() {
  return (
    <AdminOnly title={TITLE}>
      <Members />
    </AdminOnly>
  );
}

function Members() {
  const queryClient = useQueryClient();
  const navigate = useNavigate();
  const members = useQuery(membersQuery);
  const invitations = useQuery(invitationsQuery);
  const [inviting, setInviting] = useState(false);

  const refreshMembers = () => queryClient.invalidateQueries({ queryKey: membersQuery.queryKey });
  const refreshInvitations = () =>
    queryClient.invalidateQueries({ queryKey: invitationsQuery.queryKey });

  const setRole = useMutation({
    mutationFn: ({ member, role }: { member: Member; role: Role }) =>
      api.setMemberRole(member.userId, role),
    onSuccess: (updated, { member }) => {
      if (member.you) {
        // No longer an admin: this screen and its menu item go away.
        notify("あなたを編集者にしました。");
        void queryClient.invalidateQueries({ queryKey: meQuery.queryKey });
        return;
      }
      void refreshMembers();
      notify(`「${memberName(member)}」を${roleLabels[updated.role]}にしました。`);
    },
    onError: (e) => notify(errorMessage(e), "error"),
  });

  const remove = useMutation({
    mutationFn: (m: Member) => api.removeMember(m.userId),
    onSuccess: async (_, m) => {
      if (m.you) {
        queryClient.clear();
        await navigate({ to: "/select-site" });
        notify("このサイトから抜けました。");
        return;
      }
      focusHeading(MEMBERS_HEADING_ID);
      await refreshMembers();
      notify(`「${memberName(m)}」をこのサイトから外しました。`);
    },
    onError: (e) => notify(errorMessage(e), "error"),
  });

  const resend = useMutation({
    mutationFn: (i: Invitation) => api.resendInvitation(i.id),
    onSuccess: (i) => {
      void refreshInvitations();
      notify(
        `${i.email} に招待のメールをもう一度送りました。前に送ったリンクは使えなくなりました。`,
      );
    },
    onError: (e) => notify(errorMessage(e), "error"),
  });

  const cancel = useMutation({
    mutationFn: (i: Invitation) => api.cancelInvitation(i.id),
    onSuccess: async (_, i) => {
      focusHeading(INVITATIONS_HEADING_ID);
      await refreshInvitations();
      notify(`${i.email} への招待を取り消しました。`);
    },
    onError: (e) => notify(errorMessage(e), "error"),
  });

  const askRole = async (m: Member, role: Role) => {
    if (m.you) {
      const ok = await confirm({
        title: "あなたを編集者にしますか？",
        message:
          "あなたは、サイトの設定やお問い合わせ、メンバーの招待と変更ができなくなります。管理者に戻るには、ほかの管理者に頼む必要があります。",
        confirmLabel: "編集者になる",
        danger: true,
      });
      if (!ok) return;
    }
    setRole.mutate({ member: m, role });
  };

  const askRemove = async (m: Member) => {
    const ok = await confirm(
      m.you
        ? {
            title: "このサイトから抜けますか？",
            message:
              "あなたは、このサイトの管理画面を使えなくなります。あなたが書いた記事や入れた画像は残ります。戻るには、ほかの管理者にもう一度招待してもらう必要があります。",
            confirmLabel: "抜ける",
            danger: true,
          }
        : {
            title: `「${memberName(m)}」をこのサイトから外しますか？`,
            message:
              "この人は、このサイトの管理画面をすぐに使えなくなります。この人が書いた記事・固定ページや、入れた画像はそのまま残ります。もう一度招待すれば、また参加できます。",
            confirmLabel: "外す",
            danger: true,
          },
    );
    if (ok) remove.mutate(m);
  };

  const askCancel = async (i: Invitation) => {
    const ok = await confirm({
      title: "招待を取り消しますか？",
      message: `${i.email} に送った招待のリンクは使えなくなります。あとからもう一度招待できます。`,
      confirmLabel: "取り消す",
      danger: true,
    });
    if (ok) cancel.mutate(i);
  };

  const memberList = members.data ?? [];
  const invitationList = invitations.data ?? [];

  return (
    <>
      <PageHeader
        title={TITLE}
        description="このサイトの管理画面を使える人です。メールアドレスで新しい人を招待したり、役割を変えたりできます。"
        actions={
          <Button
            variant="filled"
            leftSection={<UserPlus aria-hidden="true" size={18} />}
            onClick={() => setInviting(true)}
          >
            メンバーを招待
          </Button>
        }
      />
      <Stack gap="xl">
        <Section
          id={MEMBERS_HEADING_ID}
          title="メンバーの一覧"
          variant="plain"
          description={`管理者：${roleDescriptions.admin} 編集者：${roleDescriptions.editor}`}
        >
          {members.isPending ? (
            <Loading />
          ) : members.error ? (
            <ErrorMessage>{errorMessage(members.error)}</ErrorMessage>
          ) : (
            <ResponsiveList
              items={memberList}
              getKey={(m) => m.userId}
              caption={`メンバーの一覧（${memberList.length}人）`}
              columns={[
                {
                  key: "name",
                  header: "名前",
                  render: (m) => (
                    <>
                      {m.displayName || (
                        <Text span c="dimmed">
                          名前なし
                        </Text>
                      )}
                      {m.you && <Badge ml="xs">あなた</Badge>}
                    </>
                  ),
                },
                { key: "email", header: "メール", render: (m) => m.email },
                { key: "role", header: "役割", nowrap: true, render: (m) => roleLabels[m.role] },
                {
                  key: "addedAt",
                  header: "追加した日",
                  nowrap: true,
                  render: (m) => formatDate(m.addedAt),
                },
              ]}
              actions={(m) => {
                const next: Role = m.role === "admin" ? "editor" : "admin";
                return (
                  <>
                    <Button
                      size="sm"
                      onClick={() => askRole(m, next)}
                      loading={setRole.isPending && setRole.variables?.member.userId === m.userId}
                      aria-label={`「${memberName(m)}」を${roleLabels[next]}にする`}
                    >
                      {roleLabels[next]}にする
                    </Button>
                    <Button
                      size="sm"
                      variant="subtle"
                      color="red"
                      onClick={() => askRemove(m)}
                      loading={remove.isPending && remove.variables?.userId === m.userId}
                      aria-label={
                        m.you ? "このサイトから抜ける" : `「${memberName(m)}」をサイトから外す`
                      }
                    >
                      {m.you ? "抜ける" : "外す"}
                    </Button>
                  </>
                );
              }}
            />
          )}
        </Section>

        <Section
          id={INVITATIONS_HEADING_ID}
          title="返事待ちの招待"
          variant="plain"
          description="招待のメールのリンクは7日間使えます。期限が切れたときや、メールが届かないと言われたときは「もう一度送る」で新しいリンクを送れます。"
        >
          {invitations.isPending ? (
            <Loading />
          ) : invitations.error ? (
            <ErrorMessage>{errorMessage(invitations.error)}</ErrorMessage>
          ) : invitationList.length === 0 ? (
            <Text c="dimmed">返事待ちの招待はありません。</Text>
          ) : (
            <ResponsiveList
              items={invitationList}
              getKey={(i) => i.id}
              caption={`返事待ちの招待（${invitationList.length}件）`}
              columns={[
                { key: "email", header: "メール", render: (i) => i.email },
                { key: "role", header: "役割", nowrap: true, render: (i) => roleLabels[i.role] },
                {
                  key: "status",
                  header: "状態",
                  nowrap: true,
                  render: (i) => (
                    <StatusBadge status={i.expired ? "invitationExpired" : "invited"} />
                  ),
                },
                {
                  key: "expiresAt",
                  header: "有効期限",
                  nowrap: true,
                  render: (i) => formatDateTime(i.expiresAt),
                },
              ]}
              actions={(i) => (
                <>
                  <Button
                    size="sm"
                    onClick={() => resend.mutate(i)}
                    loading={resend.isPending && resend.variables?.id === i.id}
                    aria-label={`${i.email} に招待のメールをもう一度送る`}
                  >
                    もう一度送る
                  </Button>
                  <Button
                    size="sm"
                    variant="subtle"
                    color="red"
                    onClick={() => askCancel(i)}
                    aria-label={`${i.email} への招待を取り消す`}
                  >
                    取り消す
                  </Button>
                </>
              )}
            />
          )}
        </Section>
      </Stack>

      <Dialog
        open={inviting}
        onOpenChange={setInviting}
        title="メンバーを招待"
        description="招待する人のメールアドレスに、参加するためのリンクを送ります。パスワードは要りません。"
      >
        <InviteForm
          pending={invitationList}
          onCancel={() => setInviting(false)}
          onInvited={(i, replaced) => {
            setInviting(false);
            void refreshInvitations();
            notify(
              `${i.email} に招待のメールを送りました。リンクは7日間使えます。${replaced ? "前に送った招待のリンクは使えなくなりました。" : ""}`,
            );
          }}
        />
      </Dialog>
    </>
  );
}

const inviteSchema = z.object({
  email: z
    .string()
    .trim()
    .min(1, "メールアドレスを入力してください。")
    .pipe(z.email("メールアドレスの形式が正しくありません。例：taro@example.jp")),
  role: z.enum(["editor", "admin"]),
});

type InviteValues = z.infer<typeof inviteSchema>;

function InviteForm({
  pending,
  onCancel,
  onInvited,
}: {
  pending: Invitation[];
  onCancel: () => void;
  onInvited: (i: Invitation, replaced: boolean) => void;
}) {
  const id = useId();
  const form = useForm<InviteValues>({
    resolver: zodResolver(inviteSchema),
    defaultValues: { email: "", role: "editor" },
  });
  const { errors, isSubmitting } = form.formState;

  const submit = form.handleSubmit(async (v) => {
    try {
      const inv = await api.invite(v.email, v.role);
      onInvited(
        inv,
        pending.some((p) => p.email === inv.email),
      );
    } catch (e) {
      const field = errorField(e);
      if (isApiError(e) && (field === "email" || field === "role")) {
        form.setError(field, { message: e.message }, { shouldFocus: true });
      } else {
        form.setError("root", { message: errorMessage(e) });
      }
    }
  });

  return (
    <Form onSubmit={submit} gap="md">
      <TextInput
        label="メールアドレス"
        description="例：taro@example.jp"
        type="email"
        autoComplete="off"
        required
        maw="28rem"
        error={errors.email?.message}
        {...form.register("email")}
      />
      <Controller
        control={form.control}
        name="role"
        render={({ field }) => (
          <Radio.Group
            label="役割"
            name={`${id}-role`}
            value={field.value}
            onChange={(v) => field.onChange(v as Role)}
            error={errors.role?.message}
          >
            <Stack gap="sm" mt="xs">
              {(["editor", "admin"] as const).map((r) => (
                <Radio
                  key={r}
                  value={r}
                  label={roleLabels[r]}
                  description={roleDescriptions[r]}
                  ref={r === "editor" ? field.ref : undefined}
                />
              ))}
            </Stack>
          </Radio.Group>
        )}
      />
      {errors.root && <ErrorMessage>{errors.root.message}</ErrorMessage>}
      <Group justify="flex-end" gap="sm">
        <Button onClick={onCancel}>キャンセル</Button>
        <Button type="submit" variant="filled" loading={isSubmitting}>
          招待のメールを送る
        </Button>
      </Group>
    </Form>
  );
}
