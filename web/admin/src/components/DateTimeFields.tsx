import { Fieldset, SimpleGrid, type SimpleGridProps, Stack, Text } from "@mantine/core";
import { type ReactNode, type Ref, useId } from "react";
import { formatJapanDateTime, fromJapanInputs } from "../lib/datetime";

export type DateTimeValue = { date: string; time: string };

type Props = {
  /** The group's name, e.g. 「公開する日時（日本時間）」. */
  legend: string;
  /** Shown once under the legend and read with both inputs. */
  hint: ReactNode;
  value: DateTimeValue;
  onChange: (value: DateTimeValue) => void;
  error?: string;
  dateRef?: Ref<HTMLInputElement>;
  cols?: SimpleGridProps["cols"];
  /**
   * Reads the chosen value back in words under the inputs, e.g.
   * (at) => `${at} に公開します`. The browser draws the inputs in its own locale (an
   * English browser shows 11/10/2026, 09:00 AM), so this line removes any doubt.
   */
  summary?: (at: string) => string;
};

/**
 * A native date input and time input (Japan time) sharing one hint and one error. Plain
 * `.pw-input`s, because Mantine's inputs cannot share a description; the browser's own
 * pickers also work well with keyboards, screen readers and phones.
 */
export function DateTimeFields({
  legend,
  hint,
  value,
  onChange,
  error,
  dateRef,
  cols = { base: 1, sm: 2 },
  summary,
}: Props) {
  const at = fromJapanInputs(value.date, value.time);
  const id = useId();
  const hintId = `${id}-hint`;
  const errorId = `${id}-error`;
  const control = {
    "aria-describedby": error ? `${hintId} ${errorId}` : hintId,
    "aria-invalid": error ? true : undefined,
    required: true,
    className: "pw-input",
  } as const;

  return (
    <Fieldset legend={legend} variant="unstyled">
      <Stack gap="sm">
        <Text id={hintId} size="sm" c="dimmed">
          {hint}
        </Text>
        <SimpleGrid cols={cols} spacing="sm">
          <div>
            <label htmlFor={`${id}-date`} className="pw-label">
              日付
            </label>
            <input
              {...control}
              id={`${id}-date`}
              ref={dateRef}
              type="date"
              value={value.date}
              onChange={(e) => onChange({ ...value, date: e.target.value })}
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
              value={value.time}
              onChange={(e) => onChange({ ...value, time: e.target.value })}
            />
          </div>
        </SimpleGrid>
        {summary && (
          <Text size="sm" fw={700} aria-live="polite">
            {at ? summary(formatJapanDateTime(at)) : ""}
          </Text>
        )}
        {error && (
          <Text id={errorId} size="sm" fw={700} c="red.7">
            {error}
          </Text>
        )}
      </Stack>
    </Fieldset>
  );
}
