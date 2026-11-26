import { describe, expect, it } from "vitest";
import {
  contrast,
  DEFAULT_FOOTER_COLOR,
  DEFAULT_PRIMARY_COLOR,
  formatRatio,
  isHexColor,
} from "./color";

describe("color", () => {
  it("computes WCAG contrast with white", () => {
    expect(contrast("#000000")).toBeCloseTo(21, 2);
    expect(contrast("#ffffff")).toBeCloseTo(1, 2);
    expect(contrast(DEFAULT_PRIMARY_COLOR)).toBeGreaterThanOrEqual(4.5);
    expect(contrast(DEFAULT_FOOTER_COLOR)).toBeGreaterThanOrEqual(4.5);
    expect(contrast("#7fb3ff")).toBeLessThan(4.5);
  });

  it("accepts only 6-digit hex colors", () => {
    expect(isHexColor("#1A73e8")).toBe(true);
    for (const v of ["1a73e8", "#fff", "#1a73e8 ", "red", "#gggggg"])
      expect(isHexColor(v)).toBe(false);
  });

  it("rounds the ratio down for display", () => {
    expect(formatRatio(4.4999)).toBe("4.4:1");
    expect(formatRatio(21)).toBe("21.0:1");
  });
});
