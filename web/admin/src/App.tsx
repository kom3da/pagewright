import { QueryClientProvider } from "@tanstack/react-query";
import { type RouterHistory, RouterProvider } from "@tanstack/react-router";
import { useState } from "react";
import { meQuery } from "./api/queries";
import { Toaster } from "./components/Toaster";
import { createQueryClient } from "./queryClient";
import { createAppRouter } from "./router";

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
    <QueryClientProvider client={queryClient}>
      <Toaster>
        <RouterProvider router={router} />
      </Toaster>
    </QueryClientProvider>
  );
}
