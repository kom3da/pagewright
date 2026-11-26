import { Box, Paper, Stack, Text } from "@mantine/core";
import type { ReactNode } from "react";
import { PageTitle } from "./ui/PageHeader";

/** Simple centered layout for screens shown before login (and the site selection). */
export function AuthLayout({ title, children }: { title: string; children: ReactNode }) {
  return (
    <Box component="main" id="main" bg="gray.0" mih="100vh" px="md" py={64}>
      <Box maw={440} mx="auto">
        <Text ta="center" fw={700} c="dimmed" mb="md">
          Pagewright
        </Text>
        <Paper withBorder shadow="xs" p="xl">
          <Stack gap="lg">
            <PageTitle documentTitle={title}>{title}</PageTitle>
            {children}
          </Stack>
        </Paper>
      </Box>
    </Box>
  );
}
