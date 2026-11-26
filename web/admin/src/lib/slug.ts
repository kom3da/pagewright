import { z } from "zod";

// Mirrors the server-side check in docs/dev/admin-api.md:
// lowercase letters, digits and hyphens only, not digits only, 1-100 characters.

/** Returns a message explaining what is wrong, or null if the value is valid. */
export function slugProblem(value: string): string | null {
  if (value.length === 0) return "ページのURLを入力してください。";
  if (/[^\x20-\x7e]/.test(value)) {
    return "日本語や全角文字は使えません。英小文字（a〜z）・数字・ハイフン（-）で入力してください。例：company-profile";
  }
  if (/[A-Z]/.test(value)) return "英大文字は使えません。小文字で入力してください。";
  if (!/^[a-z0-9-]+$/.test(value)) {
    return "使える文字は英小文字・数字・ハイフン（-）だけです。スペースや記号は使えません。";
  }
  if (/^[0-9]+$/.test(value)) {
    return "数字だけのURLは使えません。英字を1文字以上含めてください。";
  }
  if (value.length > 100) return "100文字以内で入力してください。";
  return null;
}

export const slugSchema = z.string().superRefine((value, ctx) => {
  const problem = slugProblem(value);
  if (problem) ctx.addIssue({ code: "custom", message: problem });
});

/** Optional slug: an empty string means "not set". */
export const optionalSlugSchema = z.string().superRefine((value, ctx) => {
  if (value === "") return;
  const problem = slugProblem(value);
  if (problem) ctx.addIssue({ code: "custom", message: problem });
});
