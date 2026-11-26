import { type FormEvent, useEffect, useId, useRef, useState } from "react";
import { defaultScheduleInputs, fromJapanInputs, toJapanInputs } from "../lib/datetime";
import { Button } from "./ui/Button";
import { Dialog } from "./ui/Dialog";
import { inputClass } from "./ui/Field";
import { ErrorMessage } from "./ui/misc";

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
  const control = {
    "aria-describedby": describedBy,
    "aria-invalid": fieldError ? true : undefined,
    "aria-required": true,
    required: true,
    className: inputClass,
  } as const;

  return (
    <form noValidate onSubmit={submit} className="space-y-4">
      <fieldset className="space-y-3">
        <legend className="font-semibold">公開する日時（日本時間）</legend>
        <p id={hintId} className="text-sm text-slate-700">
          その日時になると、いまの内容が自動で公開されます。今から1分以上あとの日時にしてください。
        </p>
        <div className="grid gap-3 sm:grid-cols-2">
          <div className="space-y-1">
            <label htmlFor={`${id}-date`} className="block font-semibold">
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
          <div className="space-y-1">
            <label htmlFor={`${id}-time`} className="block font-semibold">
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
        </div>
        {fieldError && (
          <p id={errorId} className="text-sm font-semibold text-red-700">
            {fieldError}
          </p>
        )}
      </fieldset>
      {error && !error.field && <ErrorMessage>{error.message}</ErrorMessage>}
      <div className="flex flex-wrap justify-end gap-3">
        <Button onClick={onCancel}>キャンセル</Button>
        <Button type="submit" variant="primary" disabled={busy}>
          {busy ? "予約しています…" : "この日時で予約する"}
        </Button>
      </div>
    </form>
  );
}
