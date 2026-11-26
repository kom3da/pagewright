import { zodResolver } from "@hookform/resolvers/zod";
import { useMutation } from "@tanstack/react-query";
import { useForm } from "react-hook-form";
import { z } from "zod";
import { api, errorMessage } from "../api/client";
import { AuthLayout } from "../components/AuthLayout";
import { Button } from "../components/ui/Button";
import { TextField } from "../components/ui/Field";
import { ErrorMessage } from "../components/ui/misc";

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
        <div role="status" className="space-y-3">
          <p>
            <strong>{send.variables.email}</strong>{" "}
            あてに、ログイン用のリンクを送りました。メールを開いてリンクを押してください。
          </p>
          <p className="text-sm text-slate-700">
            リンクの有効期限は15分です。メールが届かないときは、迷惑メールのフォルダも確認してください。
          </p>
        </div>
        <Button onClick={() => send.reset()}>別のメールアドレスで送り直す</Button>
      </AuthLayout>
    );
  }

  return (
    <AuthLayout title="ログイン">
      <p>登録したメールアドレスを入力してください。ログイン用のリンクをメールでお送りします。</p>
      <form noValidate onSubmit={form.handleSubmit((v) => send.mutate(v))} className="space-y-4">
        <TextField
          label="メールアドレス"
          type="email"
          autoComplete="email"
          required
          error={form.formState.errors.email?.message}
          {...form.register("email")}
        />
        {send.isError && <ErrorMessage>{errorMessage(send.error)}</ErrorMessage>}
        <Button type="submit" variant="primary" className="w-full" disabled={send.isPending}>
          {send.isPending ? "送信しています…" : "ログイン用のリンクを送る"}
        </Button>
      </form>
    </AuthLayout>
  );
}
