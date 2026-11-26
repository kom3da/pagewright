import { Box, Breadcrumbs, Group, Text, Title } from "@mantine/core";
import type { LinkProps } from "@tanstack/react-router";
import { type ReactNode, useEffect, useRef } from "react";
import { appName } from "../../config";
import { AnchorLink } from "./AnchorLink";
import classes from "./PageHeader.module.css";

// Focus moves to the page heading after client-side navigation so screen reader users
// know the page changed. The first page load keeps the browser default.
let firstHeading = true;

/**
 * The page's h1. Also sets document.title ("<title> | Pagewright 管理画面") and takes
 * focus after navigation (without drawing a focus ring). Every screen has exactly one.
 */
export function PageTitle({
  children,
  documentTitle,
}: {
  children: ReactNode;
  /** Plain-text title for the browser tab. */
  documentTitle: string;
}) {
  const ref = useRef<HTMLHeadingElement>(null);
  useEffect(() => {
    document.title = `${documentTitle} | ${appName}`;
    if (firstHeading) {
      firstHeading = false;
      return;
    }
    ref.current?.focus();
  }, [documentTitle]);
  return (
    <Title ref={ref} order={1} tabIndex={-1}>
      {children}
    </Title>
  );
}

export type Crumb = { label: string; to?: LinkProps["to"]; params?: LinkProps["params"] };

type PageHeaderProps = {
  /** The h1. A string is also used as the browser tab title. */
  title: ReactNode;
  /** Browser tab title when `title` is not a plain string. */
  documentTitle?: string;
  /** One or two sentences under the title: what this screen is for. */
  description?: ReactNode;
  /** Buttons on the right. At most one `variant="filled"` (the primary action). */
  actions?: ReactNode;
  /** Parents of this screen, e.g. [{ label: "記事", to: "/posts" }]. Not the page itself. */
  breadcrumbs?: Crumb[];
  /** Small things under the title, such as status badges and "最終更新". */
  meta?: ReactNode;
};

/**
 * Top of every screen inside the app shell: breadcrumbs, title, description and the
 * screen's primary actions. On phones the actions wrap below the title.
 */
export function PageHeader({
  title,
  documentTitle,
  description,
  actions,
  breadcrumbs,
  meta,
}: PageHeaderProps) {
  const tabTitle = documentTitle ?? (typeof title === "string" ? title : appName);
  return (
    <Box component="header" className={classes.root}>
      {breadcrumbs && breadcrumbs.length > 0 && (
        <Box component="nav" aria-label="現在の位置" mb={4}>
          <Breadcrumbs separator="›" separatorMargin={6} classNames={{ root: classes.crumbs }}>
            {breadcrumbs.map((c) =>
              c.to ? (
                <AnchorLink
                  key={c.label}
                  to={c.to}
                  params={c.params}
                  size="sm"
                  c="dimmed"
                  underline="hover"
                >
                  {c.label}
                </AnchorLink>
              ) : (
                <Text key={c.label} size="sm" c="dimmed">
                  {c.label}
                </Text>
              ),
            )}
          </Breadcrumbs>
        </Box>
      )}
      <Group justify="space-between" align="flex-start" gap="sm" wrap="wrap">
        <Box miw={0} style={{ flex: "1 1 16rem" }}>
          <PageTitle documentTitle={tabTitle}>{title}</PageTitle>
          {meta && (
            <Group gap="xs" mt={6} wrap="wrap">
              {meta}
            </Group>
          )}
          {description && (
            <Text c="dimmed" mt="xs" maw="48rem">
              {description}
            </Text>
          )}
        </Box>
        {actions && (
          <Group gap="sm" wrap="wrap" className={classes.actions}>
            {actions}
          </Group>
        )}
      </Group>
    </Box>
  );
}
