import { createMemoryHistory } from "@tanstack/react-router";
import { render } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import axe from "axe-core";
import { HttpResponse, http } from "msw";
import { expect } from "vitest";
import { App } from "../App";
import { server } from "./server";

/** Renders the whole app at an admin path such as "/posts". */
export function renderApp(path: string) {
  const history = createMemoryHistory({ initialEntries: [`/admin${path}`] });
  const user = userEvent.setup();
  return { user, history, ...render(<App history={history} />) };
}

/** Records the requests the app sends, for asserting on method, headers and body. */
export function recordRequests() {
  const requests: { method: string; path: string; headers: Headers; body: unknown }[] = [];
  server.events.on("request:start", async ({ request }) => {
    const url = new URL(request.url);
    // Reading multipart bodies here races with the handler, so only JSON is recorded.
    const isJson = request.headers.get("Content-Type")?.includes("json");
    const text = isJson ? await request.clone().text() : "";
    let body: unknown = text;
    try {
      body = text ? JSON.parse(text) : undefined;
    } catch {}
    requests.push({
      method: request.method,
      path: url.pathname + url.search,
      headers: request.headers,
      body,
    });
  });
  return requests;
}

export const failWith = (
  method: "get" | "post" | "put" | "patch" | "delete",
  path: string,
  status: number,
  code: string,
  message: string,
  field?: string,
) =>
  server.use(
    http[method](path, () => HttpResponse.json({ error: { code, message, field } }, { status })),
  );

/** Runs axe on the document. Color contrast cannot be computed in jsdom. */
export async function expectNoAxeViolations(container: Element = document.body) {
  const result = await axe.run(container, { rules: { "color-contrast": { enabled: false } } });
  const summary = result.violations.map(
    (v) => `${v.id}: ${v.nodes.map((n) => n.target.join(" ")).join(", ")}`,
  );
  expect(summary).toEqual([]);
}
