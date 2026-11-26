import { notifications } from "@mantine/notifications";
import { CircleAlert, CircleCheck } from "lucide-react";

export type NotifyKind = "success" | "error";

/**
 * Short feedback after an action: 「下書きを保存しました。」. Write a full sentence that says
 * what happened (and what happens next, if anything). Errors say what to do.
 *
 * Success is announced politely (role="status"), errors assertively (role="alert").
 * Messages stay until closed or pushed out by newer ones (at most 3 are shown), so nobody
 * has to read them against a timer (WCAG 2.2.1).
 *
 * Errors about a specific input belong next to that input, not here.
 */
export function notify(message: string, kind: NotifyKind = "success") {
  const error = kind === "error";
  notifications.show({
    message,
    color: error ? "red" : "green",
    icon: error ? (
      <CircleAlert aria-hidden="true" size={18} />
    ) : (
      <CircleCheck aria-hidden="true" size={18} />
    ),
    role: error ? "alert" : "status",
    autoClose: false,
    closeButtonProps: { "aria-label": "通知を閉じる" },
  });
}
