import { Box, Card, Group, Stack, Text, Title } from "@mantine/core";
import { type ReactNode, useId } from "react";

type SectionProps = {
  /** The section's h2 (or h3 with `order={3}`). */
  title: ReactNode;
  description?: ReactNode;
  /** Buttons at the top right of the section (secondary actions only). */
  actions?: ReactNode;
  children: ReactNode;
  order?: 2 | 3;
  /**
   * "card" (default): a bordered white card, for groups of settings or forms.
   * "plain": no box, for a list or table that has its own border.
   */
  variant?: "card" | "plain";
  id?: string;
};

/**
 * A titled group on a screen (<section aria-labelledby>). Use it to split a long screen
 * into parts such as 「基本」「見た目」 or 「タグを追加」「タグの一覧」.
 */
export function Section({
  title,
  description,
  actions,
  children,
  order = 2,
  variant = "card",
  id,
}: SectionProps) {
  const autoId = useId();
  const headingId = id ?? `${autoId}-title`;
  const header = (
    <Group justify="space-between" align="flex-start" gap="sm">
      <Box miw={0} style={{ flex: 1 }}>
        <Title order={order} id={headingId}>
          {title}
        </Title>
        {description && (
          <Text size="sm" c="dimmed" mt={4}>
            {description}
          </Text>
        )}
      </Box>
      {actions && <Group gap="xs">{actions}</Group>}
    </Group>
  );
  if (variant === "plain") {
    return (
      <Stack component="section" aria-labelledby={headingId} gap="sm">
        {header}
        {children}
      </Stack>
    );
  }
  return (
    <Card component="section" aria-labelledby={headingId}>
      <Stack gap="md">
        {header}
        {children}
      </Stack>
    </Card>
  );
}
