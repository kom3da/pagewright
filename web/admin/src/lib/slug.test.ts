import { describe, expect, it } from "vitest";
import { slugProblem } from "./slug";

describe("slugProblem", () => {
  it.each(["company", "company-profile", "2026-news", "a", "a".repeat(100)])("accepts %s", (v) => {
    expect(slugProblem(v)).toBeNull();
  });

  it.each([
    ["会社概要", "日本語"],
    ["ｃｏｍｐａｎｙ", "全角"],
    ["Company", "大文字"],
    ["company profile", "スペース"],
    ["company_profile", "ハイフン"],
    ["123", "数字だけ"],
    ["a".repeat(101), "100文字"],
    ["", "入力してください"],
  ])("rejects %s with guidance", (v, hint) => {
    expect(slugProblem(v)).toContain(hint);
  });
});
