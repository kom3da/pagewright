import { Button, Group, Stack, Text } from "@mantine/core";
import { modals } from "@mantine/modals";
import { type ReactNode, useId, useLayoutEffect, useRef } from "react";

export type ConfirmOptions = {
  /** A question: 「タグを削除しますか？」 */
  title: string;
  /** What will happen, in plain words. Say so when it cannot be undone. */
  message: ReactNode;
  /** The verb of the action: 「削除する」. Never 「OK」 or 「はい」. */
  confirmLabel: string;
  cancelLabel?: string;
  /** Red confirm button, for actions that delete or discard something. */
  danger?: boolean;
};

/**
 * Asks before doing something important (replaces window.confirm). Resolves to true when
 * the user confirms, false when they cancel, press Escape or click outside.
 *
 *   if (!(await confirm({ title: "タグを削除しますか？", message: "…", confirmLabel: "削除する", danger: true }))) return;
 *
 * The dialog is an alertdialog; focus starts on the cancel button so that Enter does not
 * destroy anything by accident, and returns to the button that opened it.
 */
export function confirm({
  title,
  message,
  confirmLabel,
  cancelLabel = "キャンセル",
  danger,
}: ConfirmOptions): Promise<boolean> {
  // Opened from inside another dialog: render inside it, so that its focus trap treats this
  // one as nested (otherwise Tab would jump back into the outer dialog).
  const parent = document.activeElement?.closest<HTMLElement>("[role=dialog], [role=alertdialog]");
  return new Promise((resolve) => {
    let result = false;
    const id = modals.open({
      title,
      size: "md",
      portalProps: parent ? { target: parent } : undefined,
      // The page is already dimmed by the outer dialog.
      withOverlay: !parent,
      shadow: "xl",
      onClose: () => resolve(result),
      children: (
        <ConfirmBody
          message={message}
          confirmLabel={confirmLabel}
          cancelLabel={cancelLabel}
          danger={danger}
          onCancel={() => modals.close(id)}
          onConfirm={() => {
            result = true;
            modals.close(id);
          }}
        />
      ),
    });
  });
}

function ConfirmBody({
  message,
  confirmLabel,
  cancelLabel,
  danger,
  onCancel,
  onConfirm,
}: {
  message: ReactNode;
  confirmLabel: string;
  cancelLabel: string;
  danger?: boolean;
  onCancel: () => void;
  onConfirm: () => void;
}) {
  const messageId = useId();
  const ref = useRef<HTMLDivElement>(null);
  // Mantine's modal is role="dialog" described by its whole body (buttons included); a
  // confirmation is an alertdialog described by its message only. Mantine sets
  // aria-describedby once more after mounting the body, hence the second pass.
  useLayoutEffect(() => {
    const dialog = ref.current?.closest("[role=dialog]");
    if (!dialog) return;
    const apply = () => {
      dialog.setAttribute("role", "alertdialog");
      dialog.setAttribute("aria-describedby", messageId);
    };
    apply();
    const timer = setTimeout(apply);
    return () => clearTimeout(timer);
  }, [messageId]);
  return (
    <Stack ref={ref} gap="lg">
      <Text id={messageId}>{message}</Text>
      <Group justify="flex-end" gap="sm">
        <Button onClick={onCancel} data-autofocus>
          {cancelLabel}
        </Button>
        <Button variant="filled" color={danger ? "red" : undefined} onClick={onConfirm}>
          {confirmLabel}
        </Button>
      </Group>
    </Stack>
  );
}
