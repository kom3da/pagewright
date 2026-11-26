import { zodResolver } from "@hookform/resolvers/zod";
import { Button, Stack, Text, TextInput } from "@mantine/core";
import { useMutation } from "@tanstack/react-query";
import { useForm } from "react-hook-form";
import { z } from "zod";
import { api, errorMessage } from "../api/client";
import { AuthLayout } from "../components/AuthLayout";
import { Form } from "../components/ui/Form";
import { ErrorMessage } from "../components/ui/feedback";

const schema = z.object({
  email: z
    .string()
    .trim()
    .min(1, "メールアドレスを入力してください。")
    .pipe(z.email("メールアドレスの形式が正しくありません。例：taro@example.jp")),
});

type Values = z.infer<typeof schema>;

export function LoginPage() {
  const form = useForm<Values>({ resolver: zodResolver(schema), defaultValues: { email: "" } });
  const send = useMutation({ mutationFn: (v: Values) => api.requestLoginLink(v.email) });

  if (send.isSuccess) {
    return (
      <AuthLayout title="メールを送りました">
        <Stack role="status" gap="sm">
          <Text>
            <strong>{send.variables.email}</strong>{" "}
            あてに、ログイン用のリンクを送りました。メールを開いてリンクを押してください。
          </Text>
          <Text size="sm" c="dimmed">
            リンクの有効期限は15分です。メールが届かないときは、迷惑メールのフォルダも確認してください。
          </Text>
        </Stack>
        <Button onClick={() => send.reset()}>別のメールアドレスで送り直す</Button>
      </AuthLayout>
    );
  }

  return (
    <AuthLayout title="ログイン">
      <Text>
        登録したメールアドレスを入力してください。ログイン用のリンクをメールでお送りします。
      </Text>
      <Form onSubmit={form.handleSubmit((v) => send.mutate(v))} gap="md">
        <TextInput
          label="メールアドレス"
          type="email"
          autoComplete="email"
          required
          error={form.formState.errors.email?.message}
          {...form.register("email")}
        />
        {send.isError && <ErrorMessage>{errorMessage(send.error)}</ErrorMessage>}
        <Button type="submit" variant="filled" fullWidth loading={send.isPending}>
          ログイン用のリンクを送る
        </Button>
      </Form>
    </AuthLayout>
  );
}
