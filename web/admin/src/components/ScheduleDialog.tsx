import { Button, Group } from "@mantine/core";
import { type FormEvent, useEffect, useRef, useState } from "react";
import { defaultScheduleInputs, fromJapanInputs, toJapanInputs } from "../lib/datetime";
import { DateTimeFields } from "./DateTimeFields";
import { Dialog } from "./ui/Dialog";
import { Form } from "./ui/Form";
import { ErrorMessage } from "./ui/feedback";

export type ScheduleError = { message: string; field: boolean };

type Props = {
  /** The name of the button that opened it (「日時を決めて公開する」「変更の公開を予約する」). */
  title: string;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  /** The current schedule, to start from. */
  scheduledAt: string | null;
  busy: boolean;
  /** Error from the server: `field` errors are shown at the inputs. */
  error: ScheduleError | null;
  onSubmit: (publishAt: string) => void;
  /** Where focus goes after closing, when the opening control no longer exists (a menu item). */
  onCloseFocus?: () => void;
};

/** Date + time (Japan time) for scheduled publishing. */
export function ScheduleDialog({
  title,
  open,
  onOpenChange,
  scheduledAt,
  busy,
  error,
  onSubmit,
  onCloseFocus,
}: Props) {
  return (
    <Dialog open={open} onOpenChange={onOpenChange} title={title} onCloseFocus={onCloseFocus}>
      {open && (
        <ScheduleForm
          scheduledAt={scheduledAt}
          busy={busy}
          error={error}
          onSubmit={onSubmit}
          onCancel={() => onOpenChange(false)}
        />
      )}
    </Dialog>
  );
}

function ScheduleForm({
  scheduledAt,
  busy,
  error,
  onSubmit,
  onCancel,
}: Omit<Props, "title" | "open" | "onOpenChange"> & { onCancel: () => void }) {
  const [value, setValue] = useState(() =>
    scheduledAt ? toJapanInputs(scheduledAt) : defaultScheduleInputs(),
  );
  const [localError, setLocalError] = useState<string>();
  const dateRef = useRef<HTMLInputElement>(null);

  // A server error is shown until the user changes the date or time.
  const [showServerError, setShowServerError] = useState(true);
  const fieldError = localError ?? (showServerError && error?.field ? error.message : undefined);
  useEffect(() => {
    setShowServerError(true);
    if (error?.field) dateRef.current?.focus();
  }, [error]);

  const submit = (e: FormEvent) => {
    e.preventDefault();
    e.stopPropagation();
    const publishAt = fromJapanInputs(value.date, value.time);
    if (!publishAt) {
      setLocalError("日付と時刻を入力してください。");
      dateRef.current?.focus();
      return;
    }
    if (new Date(publishAt).getTime() < Date.now() + 60_000) {
      setLocalError("公開する日時は、今から1分以上あとにしてください。");
      dateRef.current?.focus();
      return;
    }
    setLocalError(undefined);
    onSubmit(publishAt);
  };

  return (
    <Form onSubmit={submit} gap="md">
      <DateTimeFields
        legend="公開する日時（日本時間）"
        hint="その日時になると、いまの内容が自動で公開されます。今から1分以上あとの日時にしてください。"
        value={value}
        onChange={(v) => {
          setValue(v);
          setLocalError(undefined);
          setShowServerError(false);
        }}
        error={fieldError}
        dateRef={dateRef}
        summary={(at) => `${at} に公開します`}
      />
      {error && !error.field && <ErrorMessage>{error.message}</ErrorMessage>}
      <Group justify="flex-end" gap="sm">
        <Button onClick={onCancel}>キャンセル</Button>
        <Button type="submit" variant="filled" loading={busy}>
          この日時で予約する
        </Button>
      </Group>
    </Form>
  );
}
