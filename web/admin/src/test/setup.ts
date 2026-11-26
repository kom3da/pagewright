import "@testing-library/jest-dom/vitest";
import { File as NodeFile } from "node:buffer";
import { cleanup } from "@testing-library/react";
import { getResponse, type RequestHandler } from "msw";
import { afterAll, afterEach, beforeAll } from "vitest";
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

beforeAll(() => {
  server.listen({ onUnhandledFrame: "error" });
  installFetchShim();
});
afterEach(() => {
  cleanup();
  server.resetHandlers();
  server.events.removeAllListeners();
  resetDb();
});
afterAll(() => server.close());
