import "@testing-library/jest-dom/vitest";
import { File as NodeFile } from "node:buffer";
import { modals } from "@mantine/modals";
import { notifications } from "@mantine/notifications";
import { cleanup } from "@testing-library/react";
import { getResponse, type RequestHandler } from "msw";
import { afterAll, afterEach, beforeAll } from "vitest";
import { forgetShownSite } from "../api/client";
import { resetDb } from "../mocks/handlers";
import { server } from "./server";

// jsdom replaces FormData/File with its own classes, which Node's fetch cannot send.
// Node's FormData class is reachable through Node's Response.
const NodeFormData = (await new Response(new URLSearchParams()).formData())
  .constructor as typeof FormData;

async function toNodeFormData(form: FormData) {
  const out = new NodeFormData();
  for (const [key, value] of form.entries()) {
    if (typeof value === "string") {
      out.append(key, value);
    } else {
      const file = new NodeFile([await value.arrayBuffer()], value.name, { type: value.type });
      out.append(key, file as unknown as Blob, value.name);
    }
  }
  return out;
}

/** Wraps MSW's patched fetch with what jsdom + Node's fetch lack compared to a browser. */
function installFetchShim() {
  const mswFetch = globalThis.fetch;
  globalThis.fetch = async (input, init) => {
    // Node's fetch does not resolve relative URLs.
    const url = typeof input === "string" ? new URL(input, window.location.origin) : input;
    if (init?.body instanceof FormData) {
      // Node's fetch cannot send jsdom's FormData, so multipart requests are resolved
      // against the mock handlers directly.
      const request = new Request(url, { ...init, body: await toNodeFormData(init.body) });
      const response = await getResponse(server.listHandlers() as RequestHandler[], request);
      if (!response) throw new Error(`No mock handler for ${request.method} ${request.url}`);
      return response;
    }
    return mswFetch(url, init);
  };
}

// jsdom does not implement layout. The router scrolls on navigation and ProseMirror
// measures selections, so give them harmless stand-ins.
window.scrollTo = () => {};
const emptyRect = () => new DOMRect(0, 0, 0, 0);
const emptyRects = () => ({ length: 0, item: () => null, [Symbol.iterator]: [][Symbol.iterator] });
Range.prototype.getBoundingClientRect = emptyRect;
Range.prototype.getClientRects = emptyRects as unknown as () => DOMRectList;
Element.prototype.getClientRects ??= emptyRects as unknown as () => DOMRectList;
document.elementFromPoint = () => null;
Element.prototype.scrollIntoView = () => {};

// Mantine needs matchMedia and ResizeObserver, which jsdom lacks. Media queries are
// evaluated against window.innerWidth (1024 in jsdom), so tests see the desktop layout
// (tables, sidebar). Set window.innerWidth = 390 before rendering to test the phone layout.
const matches = (query: string) =>
  query.split(",").some(
    (part) =>
      [...part.matchAll(/\((min|max)-width:\s*([\d.]+)(px|em|rem)\)/g)].every(
        ([, kind, n, unit]) => {
          const px = Number(n) * (unit === "px" ? 1 : 16);
          return kind === "min" ? window.innerWidth >= px : window.innerWidth <= px;
        },
      ) && /width/.test(part),
  );
window.matchMedia = (query: string) =>
  ({
    matches: matches(query),
    media: query,
    onchange: null,
    addListener: () => {},
    removeListener: () => {},
    addEventListener: () => {},
    removeEventListener: () => {},
    dispatchEvent: () => false,
  }) as MediaQueryList;
window.ResizeObserver ??= class {
  observe() {}
  unobserve() {}
  disconnect() {}
};
// Textarea's autosize listens for web fonts loading.
if (!("fonts" in document)) {
  Object.defineProperty(document, "fonts", {
    value: { addEventListener() {}, removeEventListener() {}, ready: Promise.resolve() },
  });
}
const { getComputedStyle } = window;
window.getComputedStyle = (elt) => getComputedStyle(elt);

beforeAll(() => {
  server.listen({ onUnhandledFrame: "error" });
  installFetchShim();
});
afterEach(() => {
  cleanup();
  // Mantine keeps notifications and modals in module-level stores.
  notifications.clean();
  notifications.cleanQueue();
  modals.closeAll();
  window.innerWidth = 1024;
  server.resetHandlers();
  server.events.removeAllListeners();
  resetDb();
  forgetShownSite();
});
afterAll(() => server.close());
