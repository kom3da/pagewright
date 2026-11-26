import { Modal, Text } from "@mantine/core";
import { type KeyboardEvent, type ReactNode, useEffect, useRef } from "react";

type DialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  title: string;
  /** One or two sentences under the title explaining what the dialog is for. */
  description?: ReactNode;
  children: ReactNode;
  /** Wider dialog for lists, image pickers and side-by-side comparisons. */
  wide?: boolean;
  /** Called instead of returning focus to the previously focused element. */
  onCloseFocus?: () => void;
};

/**
 * A modal dialog for a task (a form, a picker). For yes/no questions use `confirm()`.
 * Closes on Escape, the × button and a click outside; focus is trapped inside and returns
 * to the button that opened it.
 */
export function Dialog({
  open,
  onOpenChange,
  title,
  description,
  children,
  wide,
  onCloseFocus,
}: DialogProps) {
  // Escape is handled here instead of by Mantine, whose window listener would also close
  // this dialog when Escape is pressed in a confirm() opened on top of it.
  const onKeyDown = (e: KeyboardEvent) => {
    if (e.key !== "Escape" || e.nativeEvent.isComposing) return;
    // Ignore Escape from a confirmation nested inside this dialog: it closes only itself.
    const own = e.currentTarget.querySelector("section");
    const from = (e.target as Element).closest("[role=dialog], [role=alertdialog]");
    if (from !== own) return;
    e.stopPropagation();
    onOpenChange(false);
  };

  const wasOpen = useRef(open);
  useEffect(() => {
    if (wasOpen.current && !open && onCloseFocus) setTimeout(onCloseFocus);
    wasOpen.current = open;
  }, [open, onCloseFocus]);

  return (
    <Modal
      opened={open}
      onClose={() => onOpenChange(false)}
      title={title}
      size={wide ? "56rem" : "lg"}
      closeOnEscape={false}
      returnFocus={!onCloseFocus}
      onKeyDown={onKeyDown}
    >
      {description && (
        <Text c="dimmed" mb="md">
          {description}
        </Text>
      )}
      {children}
    </Modal>
  );
}
