import { Alert, Group, Loader, Text } from "@mantine/core";
import { CircleAlert, Info, TriangleAlert } from "lucide-react";
import type { ReactNode } from "react";

/** Something went wrong with the whole screen or form (role="alert"). Say what to do next. */
export function ErrorMessage({ children }: { children: ReactNode }) {
  if (!children) return null;
  return (
    <Alert color="red" role="alert" icon={<CircleAlert aria-hidden="true" size={18} />}>
      {children}
    </Alert>
  );
}

/**
 * A lasting note on a screen (not a reaction to an action): "info" for explanations,
 * "warning" for things to be careful about (personal data, changes that will be saved…).
 */
export function Notice({
  kind = "info",
  title,
  children,
}: {
  kind?: "info" | "warning";
  title?: ReactNode;
  children: ReactNode;
}) {
  const warning = kind === "warning";
  return (
    <Alert
      role="note"
      color={warning ? "orange" : "gray"}
      title={title}
      icon={
        warning ? (
          <TriangleAlert aria-hidden="true" size={18} />
        ) : (
          <Info aria-hidden="true" size={18} />
        )
      }
    >
      {children}
    </Alert>
  );
}

/** Loading state of a screen or a section (role="status", so it is announced). */
export function Loading({ label = "読み込んでいます…" }: { label?: string }) {
  return (
    <Group role="status" gap="xs" py="lg">
      <Loader size="xs" color="gray" aria-hidden="true" />
      <Text c="dimmed">{label}</Text>
    </Group>
  );
}
