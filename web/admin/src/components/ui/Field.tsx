import { clsx } from "clsx";
import { type ComponentProps, type ReactNode, useId } from "react";

export const inputClass =
  "block w-full rounded-md border border-slate-500 bg-white px-3 py-2 text-base text-slate-900 aria-[invalid=true]:border-red-700";

type FieldProps = {
  label: ReactNode;
  hint?: ReactNode;
  error?: string;
  required?: boolean;
  className?: string;
  children: (ids: {
    id: string;
    "aria-describedby"?: string;
    "aria-invalid"?: boolean;
    "aria-required"?: boolean;
  }) => ReactNode;
};

/** Label + control + hint + error, with the error and hint tied to the control. */
export function Field({ label, hint, error, required, className, children }: FieldProps) {
  const id = useId();
  const hintId = `${id}-hint`;
  const errorId = `${id}-error`;
  const describedBy = [hint && hintId, error && errorId].filter(Boolean).join(" ") || undefined;
  return (
    <div className={clsx("space-y-1", className)}>
      <label htmlFor={id} className="block font-semibold">
        {label}
        {required && <span className="ml-1 text-sm font-normal text-red-700">（必須）</span>}
      </label>
      {hint && (
        <p id={hintId} className="text-sm text-slate-700">
          {hint}
        </p>
      )}
      {children({
        id,
        "aria-describedby": describedBy,
        "aria-invalid": error ? true : undefined,
        "aria-required": required || undefined,
      })}
      {error && (
        <p id={errorId} className="text-sm font-semibold text-red-700">
          {error}
        </p>
      )}
    </div>
  );
}

type TextFieldProps = ComponentProps<"input"> & {
  label: ReactNode;
  hint?: ReactNode;
  error?: string;
};

export function TextField({ label, hint, error, required, className, ...props }: TextFieldProps) {
  return (
    <Field label={label} hint={hint} error={error} required={required} className={className}>
      {(ids) => <input className={inputClass} {...ids} {...props} />}
    </Field>
  );
}
