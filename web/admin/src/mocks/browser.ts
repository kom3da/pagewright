import { setupWorker } from "msw/browser";
import { handlers } from "./handlers";

/** Starts the mock API in the browser (`pnpm dev:mock`). The login token is "valid-token". */
export async function startMockWorker() {
  const worker = setupWorker(...handlers);
  await worker.start({
    serviceWorker: { url: `${import.meta.env.BASE_URL}mockServiceWorker.js` },
    onUnhandledFrame: "bypass",
  });
}
