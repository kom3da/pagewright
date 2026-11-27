import { describe, expect, it } from "vitest";
import { linkProblem } from "./LinkDialog";

describe("linkProblem", () => {
  it.each([
    "https://example.jp/",
    "/company/",
    "#access",
    "tel:03-1234-5678",
    "mailto:info@example.jp",
  ])("accepts %s", (href) => {
    expect(linkProblem(href)).toBeNull();
  });

  it("names every kind of link it accepts", () => {
    const message = linkProblem("example.jp");
    expect(message).toContain("https://");
    expect(message).toContain("電話（tel:）");
    expect(message).toContain("メール（mailto:）");
  });
});
