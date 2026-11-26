import { Button, Fieldset, Group, SimpleGrid, Stack, Text } from "@mantine/core";
import { type FormEvent, useEffect, useId, useRef, useState } from "react";
import { defaultScheduleInputs, fromJapanInputs, toJapanInputs } from "../lib/datetime";
import { Dialog } from "./ui/Dialog";
import { Form } from "./ui/Form";
import { ErrorMessage } from "./ui/feedback";

export type ScheduleError = { message: string; field: boolean };

type Props = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  /** The current schedule, to start from. */
  scheduledAt: string | null;
  busy: boolean;
  /** Error from the server: `field` errors are shown at the inputs. */
  error: ScheduleError | null;
  onSubmit: (publishAt: string) => void;
};

/** Date + time (Japan time) for scheduled publishing. */
export function ScheduleDialog({ open, onOpenChange, scheduledAt, busy, error, onSubmit }: Props) {
  return (
    <Dialog open={open} onOpenChange={onOpenChange} title="日時を指定して公開">
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
}: Omit<Props, "open" | "onOpenChange"> & { onCancel: () => void }) {
  const id = useId();
  const initial = scheduledAt ? toJapanInputs(scheduledAt) : defaultScheduleInputs();
  const [date, setDate] = useState(initial.date);
  const [time, setTime] = useState(initial.time);
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
    const publishAt = fromJapanInputs(date, time);
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

  const hintId = `${id}-hint`;
  const errorId = `${id}-error`;
  const describedBy = fieldError ? `${hintId} ${errorId}` : hintId;
  // The hint and the error belong to both inputs, so they are shown once for the group.
  const control = {
    "aria-describedby": describedBy,
    "aria-invalid": fieldError ? true : undefined,
    required: true,
    className: "pw-input",
  } as const;

  return (
    <Form onSubmit={submit} gap="md">
      <Fieldset legend="公開する日時（日本時間）" variant="unstyled">
        <Stack gap="sm">
          <Text id={hintId} size="sm" c="dimmed">
            その日時になると、いまの内容が自動で公開されます。今から1分以上あとの日時にしてください。
          </Text>
          <SimpleGrid cols={{ base: 1, sm: 2 }} spacing="sm">
            <div>
              <label htmlFor={`${id}-date`} className="pw-label">
                日付
              </label>
              <input
                {...control}
                id={`${id}-date`}
                ref={dateRef}
                type="date"
                value={date}
                onChange={(e) => {
                  setDate(e.target.value);
                  setLocalError(undefined);
                  setShowServerError(false);
                }}
              />
            </div>
            <div>
              <label htmlFor={`${id}-time`} className="pw-label">
                時刻
              </label>
              <input
                {...control}
                id={`${id}-time`}
                type="time"
                value={time}
                onChange={(e) => {
                  setTime(e.target.value);
                  setLocalError(undefined);
                  setShowServerError(false);
                }}
              />
            </div>
          </SimpleGrid>
          {fieldError && (
            <Text id={errorId} size="sm" fw={700} c="red.7">
              {fieldError}
            </Text>
          )}
        </Stack>
      </Fieldset>
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
