import { Input } from "@mantine/core";
import { type ReactNode, useId } from "react";

type FieldProps = {
  label: ReactNode;
  /** Help under the label: format, example, what it is used for. */
  description?: ReactNode;
  error?: ReactNode;
  required?: boolean;
  children: (ids: {
    id: string;
    "aria-describedby"?: string;
    "aria-invalid"?: boolean;
    required?: boolean;
  }) => ReactNode;
};

/**
 * Label + description + error around a control that is not a Mantine input (a file input,
 * the color picker, a custom widget). Mantine inputs (TextInput, Textarea, NativeSelect…)
 * already do this with their own `label` / `description` / `error` / `required` props:
 * use those first.
 */
export function Field({ label, description, error, required, children }: FieldProps) {
  const id = useId();
  const describedBy =
    [error && `${id}-error`, description && `${id}-description`].filter(Boolean).join(" ") ||
    undefined;
  return (
    <Input.Wrapper
      id={id}
      label={label}
      description={description}
      error={error}
      required={required}
    >
      {children({
        id,
        "aria-describedby": describedBy,
        "aria-invalid": error ? true : undefined,
        required: required || undefined,
      })}
    </Input.Wrapper>
  );
}
