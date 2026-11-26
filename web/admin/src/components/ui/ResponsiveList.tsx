import { Box, Group, Paper, Stack, Table, Text, VisuallyHidden } from "@mantine/core";
import { useMediaQuery } from "@mantine/hooks";
import type { CSSProperties, Key, ReactNode } from "react";
import { DESKTOP_QUERY } from "../../theme";
import classes from "./ResponsiveList.module.css";

export type ListColumn<T> = {
  key: string;
  /** Column header on desktop, label in front of the value on phones. */
  header: string;
  render: (item: T) => ReactNode;
  /** Keep the cell on one line (dates, short codes). */
  nowrap?: boolean;
  width?: CSSProperties["width"];
};

type ResponsiveListProps<T> = {
  items: T[];
  getKey: (item: T) => Key;
  /**
   * The first column is the item's title: the first cell on desktop, the card heading on
   * phones. Put the link to the item's screen there.
   */
  columns: ListColumn<T>[];
  /** Row buttons (編集・削除…). The last column on desktop, the card footer on phones. */
  actions?: (item: T) => ReactNode;
  /** What the list is, for screen readers: 「記事の一覧（新しい順）」. */
  caption: string;
};

/**
 * The "DataList" pattern: a list of records shown as a table on desktop and as stacked
 * cards on phones, from one column definition. Use it for every list of things (記事,
 * カテゴリ, お問い合わせ…). Empty and loading states are the caller's job (EmptyState,
 * Loading), so this is only rendered with at least one item.
 */
export function ResponsiveList<T>({
  items,
  getKey,
  columns,
  actions,
  caption,
}: ResponsiveListProps<T>) {
  const desktop = useMediaQuery(DESKTOP_QUERY, true, { getInitialValueInEffect: false });
  const [first, ...rest] = columns;

  if (!desktop) {
    return (
      <Stack component="ul" aria-label={caption} gap="sm" className={classes.cards}>
        {items.map((item) => (
          <Paper component="li" key={getKey(item)} withBorder p="md">
            <Box fw={700}>{first.render(item)}</Box>
            {rest.length > 0 && (
              <Box component="dl" className={classes.fields}>
                {rest.map((col) => (
                  <Group key={col.key} gap="xs" wrap="nowrap" align="baseline">
                    <Text component="dt" size="sm" c="dimmed" className={classes.label}>
                      {col.header}
                    </Text>
                    <Box component="dd" className={classes.value}>
                      {col.render(item)}
                    </Box>
                  </Group>
                ))}
              </Box>
            )}
            {actions && (
              <Group gap="xs" mt="sm" justify="flex-end">
                {actions(item)}
              </Group>
            )}
          </Paper>
        ))}
      </Stack>
    );
  }

  return (
    <Paper withBorder className={classes.tableWrap}>
      <Table.ScrollContainer minWidth={560} type="native">
        <Table>
          <VisuallyHidden component="caption">{caption}</VisuallyHidden>
          <Table.Thead>
            <Table.Tr>
              {columns.map((col) => (
                <Table.Th key={col.key} scope="col" w={col.width}>
                  {col.header}
                </Table.Th>
              ))}
              {actions && (
                <Table.Th scope="col">
                  <VisuallyHidden>操作</VisuallyHidden>
                </Table.Th>
              )}
            </Table.Tr>
          </Table.Thead>
          <Table.Tbody>
            {items.map((item) => (
              <Table.Tr key={getKey(item)}>
                {columns.map((col) => (
                  <Table.Td key={col.key} className={col.nowrap ? classes.nowrap : undefined}>
                    {col.render(item)}
                  </Table.Td>
                ))}
                {actions && (
                  <Table.Td>
                    <Group gap="xs" justify="flex-end" wrap="nowrap">
                      {actions(item)}
                    </Group>
                  </Table.Td>
                )}
              </Table.Tr>
            ))}
          </Table.Tbody>
        </Table>
      </Table.ScrollContainer>
    </Paper>
  );
}
