import { clsx } from "clsx";
import type { ComponentProps } from "react";

type Variant = "primary" | "secondary" | "danger" | "ghost";

const variants: Record<Variant, string> = {
  primary: "bg-blue-700 text-white hover:bg-blue-800 disabled:bg-slate-400",
  secondary:
    "border border-slate-400 bg-white text-slate-900 hover:bg-slate-100 disabled:text-slate-500",
  danger: "bg-red-700 text-white hover:bg-red-800 disabled:bg-slate-400",
  ghost: "text-slate-900 hover:bg-slate-200 disabled:text-slate-500",
};

export const buttonClass = (variant: Variant = "secondary", className?: string) =>
  clsx(
    "inline-flex min-h-10 items-center justify-center gap-1.5 rounded-md px-4 py-2 text-sm font-semibold disabled:cursor-not-allowed",
    variants[variant],
    className,
  );

type Props = ComponentProps<"button"> & { variant?: Variant };

export function Button({ variant, className, type = "button", ...props }: Props) {
  return <button type={type} className={buttonClass(variant, className)} {...props} />;
}
