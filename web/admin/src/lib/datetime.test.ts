import { describe, expect, it } from "vitest";
import {
  defaultScheduleInputs,
  formatJapanDateTime,
  fromJapanInputs,
  toJapanInputs,
} from "./datetime";

describe("Japan-time scheduling helpers", () => {
  it("formats in Japan time", () => {
    expect(formatJapanDateTime("2026-12-01T00:00:00Z")).toBe("2026年12月1日 9:00");
    expect(formatJapanDateTime("2026-11-30T15:30:00Z")).toBe("2026年12月1日 0:30");
  });

  it("converts between UTC and Japan-time inputs across the date line", () => {
    expect(toJapanInputs("2026-11-30T15:30:00Z")).toEqual({ date: "2026-12-01", time: "00:30" });
    expect(fromJapanInputs("2026-12-01", "00:30")).toBe("2026-11-30T15:30:00Z");
    expect(fromJapanInputs("2026-12-01", "")).toBeNull();
    expect(fromJapanInputs("", "09:00")).toBeNull();
  });

  it("defaults to tomorrow 9:00 in Japan", () => {
    // 2026-11-27 23:30 in Japan
    expect(defaultScheduleInputs(new Date("2026-11-27T14:30:00Z"))).toEqual({
      date: "2026-11-28",
      time: "09:00",
    });
  });
});
