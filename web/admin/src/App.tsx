import { MantineProvider } from "@mantine/core";
import { DatesProvider } from "@mantine/dates";
import { useMediaQuery } from "@mantine/hooks";
import { ModalsProvider } from "@mantine/modals";
import { Notifications } from "@mantine/notifications";
import { QueryClientProvider } from "@tanstack/react-query";
import { type RouterHistory, RouterProvider } from "@tanstack/react-router";
import { useState } from "react";
import "dayjs/locale/ja";
import { meQuery } from "./api/queries";
import { createQueryClient } from "./queryClient";
import { createAppRouter } from "./router";
import { DESKTOP_QUERY, theme } from "./theme";

export function createApp(history?: RouterHistory) {
  let router: ReturnType<typeof createAppRouter>;
  const queryClient = createQueryClient((e) => {
    if (e.status === 401) {
      void router.navigate({ to: "/login" }).then(() => queryClient.clear());
    } else if (e.status === 403 && e.code === "no_site") {
      void queryClient.invalidateQueries({ queryKey: meQuery.queryKey });
      void router.navigate({ to: "/select-site" });
    }
  });
  router = createAppRouter(queryClient, history);
  return { queryClient, router };
}

export function App({ history }: { history?: RouterHistory }) {
  const [{ queryClient, router }] = useState(() => createApp(history));
  return (
    // Light only for now; `env="test"` turns off transitions and portals in Vitest.
    <MantineProvider
      theme={theme}
      forceColorScheme="light"
      env={import.meta.env.MODE === "test" ? "test" : "default"}
    >
      <DatesProvider settings={{ locale: "ja", firstDayOfWeek: 0, labelSeparator: "〜" }}>
        <QueryClientProvider client={queryClient}>
          <ModalsProvider>
            <AppNotifications />
            <RouterProvider router={router} />
          </ModalsProvider>
        </QueryClientProvider>
      </DatesProvider>
    </MantineProvider>
  );
}

/**
 * On phones the editor and the site settings have a save bar fixed to the bottom of the
 * screen, so notifications show at the top there instead of covering its buttons.
 */
function AppNotifications() {
  const desktop = useMediaQuery(DESKTOP_QUERY, true, { getInitialValueInEffect: false });
  return <Notifications position={desktop ? "bottom-right" : "top-center"} limit={3} />;
}
