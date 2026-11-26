import * as AlertDialog from "@radix-ui/react-alert-dialog";
import * as RadixDialog from "@radix-ui/react-dialog";
import { X } from "lucide-react";
import type { ReactNode } from "react";
import { Button, buttonClass } from "./Button";

const overlayClass = "fixed inset-0 z-40 bg-slate-900/50";
const contentClass =
  "fixed top-1/2 left-1/2 z-50 max-h-[90vh] w-[min(92vw,40rem)] -translate-x-1/2 -translate-y-1/2 overflow-y-auto rounded-lg bg-white p-6 shadow-xl";

type DialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  title: string;
  description?: ReactNode;
  children: ReactNode;
  wide?: boolean;
  /** Called instead of returning focus to the previously focused element. */
  onCloseFocus?: () => void;
};

export function Dialog({
  open,
  onOpenChange,
  title,
  description,
  children,
  wide,
  onCloseFocus,
}: DialogProps) {
  return (
    <RadixDialog.Root open={open} onOpenChange={onOpenChange}>
      <RadixDialog.Portal>
        <RadixDialog.Overlay className={overlayClass} />
        <RadixDialog.Content
          className={contentClass}
          onCloseAutoFocus={
            onCloseFocus
              ? (e) => {
                  e.preventDefault();
                  onCloseFocus();
                }
              : undefined
          }
          style={wide ? { width: "min(94vw, 56rem)" } : undefined}
          {...(description ? {} : { "aria-describedby": undefined })}
        >
          <div className="mb-4 flex items-start justify-between gap-4">
            <RadixDialog.Title className="text-xl font-bold">{title}</RadixDialog.Title>
            <RadixDialog.Close className={buttonClass("ghost", "px-2")} aria-label="閉じる">
              <X aria-hidden="true" size={20} />
            </RadixDialog.Close>
          </div>
          {description && (
            <RadixDialog.Description className="mb-4 text-slate-700">
              {description}
            </RadixDialog.Description>
          )}
          {children}
        </RadixDialog.Content>
      </RadixDialog.Portal>
    </RadixDialog.Root>
  );
}

type ConfirmProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  title: string;
  description: ReactNode;
  confirmLabel: string;
  cancelLabel?: string;
  danger?: boolean;
  busy?: boolean;
  onConfirm: () => void;
};

/** Accessible replacement for window.confirm. Focus starts on the cancel button. */
export function ConfirmDialog({
  open,
  onOpenChange,
  title,
  description,
  confirmLabel,
  cancelLabel = "キャンセル",
  danger,
  busy,
  onConfirm,
}: ConfirmProps) {
  return (
    <AlertDialog.Root open={open} onOpenChange={onOpenChange}>
      <AlertDialog.Portal>
        <AlertDialog.Overlay className={overlayClass} />
        <AlertDialog.Content className={contentClass}>
          <AlertDialog.Title className="mb-3 text-xl font-bold">{title}</AlertDialog.Title>
          <AlertDialog.Description className="mb-6 text-slate-700">
            {description}
          </AlertDialog.Description>
          <div className="flex flex-wrap justify-end gap-3">
            <AlertDialog.Cancel className={buttonClass("secondary")}>
              {cancelLabel}
            </AlertDialog.Cancel>
            <Button
              variant={danger ? "danger" : "primary"}
              disabled={busy}
              onClick={(e) => {
                e.preventDefault();
                onConfirm();
              }}
            >
              {confirmLabel}
            </Button>
          </div>
        </AlertDialog.Content>
      </AlertDialog.Portal>
    </AlertDialog.Root>
  );
}
