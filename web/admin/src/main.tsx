import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import { App } from "./App";
import "@mantine/core/styles.css";
import "@mantine/notifications/styles.css";
import "@mantine/dates/styles.css";
import "./styles/global.css";
import "./editor/prose.css";

async function start() {
  // `pnpm dev:mock` runs the admin without the Go server, using the MSW mock API.
  if (import.meta.env.MODE === "mock") {
    const { startMockWorker } = await import("./mocks/browser");
    await startMockWorker();
  }
  const root = document.getElementById("root");
  if (!root) return;
  createRoot(root).render(
    <StrictMode>
      <App />
    </StrictMode>,
  );
}

void start();
