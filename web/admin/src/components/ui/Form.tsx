import { getSpacing, type MantineSpacing } from "@mantine/core";
import type { ComponentProps, Ref } from "react";

type FormProps = Omit<ComponentProps<"form">, "ref"> & {
  ref?: Ref<HTMLFormElement>;
  /** Space between fields (theme spacing). */
  gap?: MantineSpacing;
};

/**
 * A <form> laid out like Mantine's Stack. Browser validation is off (`noValidate`): the
 * app validates and shows Japanese messages at each field instead.
 */
export function Form({ gap = "md", style, ...props }: FormProps) {
  return (
    <form
      noValidate
      style={{ display: "flex", flexDirection: "column", gap: getSpacing(gap), ...style }}
      {...props}
    />
  );
}
