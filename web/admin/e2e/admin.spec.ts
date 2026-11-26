import AxeBuilder from "@axe-core/playwright";
import { expect, type Page, test } from "@playwright/test";

// The mock API lives in the page (MSW service worker); every page load starts from seed data.

async function expectAccessible(page: Page) {
  const result = await new AxeBuilder({ page })
    .withTags(["wcag2a", "wcag2aa", "wcag21a", "wcag21aa"])
    .analyze();
  expect(
    result.violations.map((v) => `${v.id}: ${v.nodes.map((n) => n.target).join(", ")}`),
  ).toEqual([]);
}

test("login screens are accessible and the link logs in only on button press", async ({ page }) => {
  await page.goto("login");
  await expect(page.getByRole("heading", { level: 1, name: "ログイン" })).toBeVisible();
  await expect(page).toHaveTitle("ログイン | Pagewright 管理画面");
  await expect(page.locator("html")).toHaveAttribute("lang", "ja");
  await expectAccessible(page);

  await page.goto("login/verify?token=valid-token");
  await expectAccessible(page);
  await page.getByRole("button", { name: "ログイン" }).click();
  await expect(page.getByRole("heading", { name: /ようこそ/ })).toBeVisible();
});

test("lists and management screens are accessible", async ({ page }) => {
  for (const [path, heading] of [
    ["", /ようこそ/],
    ["posts", "記事の一覧"],
    ["pages", "固定ページの一覧"],
    ["categories", "カテゴリ"],
    ["tags", "タグ"],
  ] as const) {
    await page.goto(path);
    await expect(page.getByRole("heading", { level: 1, name: heading })).toBeVisible();
    await expectAccessible(page);
  }
});

test("editor is accessible and keyboard operable", async ({ page }) => {
  await page.goto("posts/3");
  const body = page.getByRole("textbox", { name: "本文" });
  await expect(body).toBeVisible();
  await page.getByText("詳細設定").click();
  await expect(page.getByRole("checkbox", { name: "お知らせ" })).toBeChecked();
  await expectAccessible(page);

  // The toolbar is a single Tab stop; arrow keys move between buttons.
  await page.getByLabel("タイトル").focus();
  await page.keyboard.press("Tab");
  await expect(page.getByRole("button", { name: "本文（段落）" })).toBeFocused();
  await page.keyboard.press("ArrowRight");
  await expect(page.getByRole("button", { name: "大見出し" })).toBeFocused();
  await page.keyboard.press("Tab");
  await expect(body).toBeFocused();
});

test("Japanese IME composition is not broken by the editor", async ({ page }) => {
  await page.goto("posts/1");
  const body = page.getByRole("textbox", { name: "本文" });
  await body.click();
  await page.keyboard.press("End");

  // Simulate an IME: composing text, then committing the conversion.
  const cdp = await page.context().newCDPSession(page);
  await cdp.send("Input.imeSetComposition", {
    text: "とうきょう",
    selectionStart: 5,
    selectionEnd: 5,
  });
  await expect(body).toContainText("とうきょう");
  await cdp.send("Input.insertText", { text: "東京" });
  await expect(body).toContainText("本文です。東京");
  await expect(body).not.toContainText("とうきょう");

  // Add ruby to the converted word.
  await page.keyboard.press("Shift+ArrowLeft");
  await page.keyboard.press("Shift+ArrowLeft");
  await page.getByRole("button", { name: "ルビ" }).click();
  const dialog = page.getByRole("dialog", { name: "ルビ（ふりがな）" });
  await dialog.getByLabel(/ふりがな/).fill("とうきょう");
  await dialog.getByRole("button", { name: "ルビをつける" }).click();
  await expect(body.locator("ruby rt")).toHaveText("とうきょう");
  await expect(body.locator("ruby")).toContainText("東京");
});

test("unsaved changes are confirmed before leaving", async ({ page }) => {
  await page.goto("posts/1");
  await page.getByLabel("タイトル").fill("変更したタイトル");
  await page.getByRole("link", { name: "タグ" }).click();
  const dialog = page.getByRole("alertdialog", { name: "保存していない変更があります" });
  await expect(dialog).toBeVisible();
  await expectAccessible(page);
  await dialog.getByRole("button", { name: "このページにとどまる" }).click();
  await page.getByRole("button", { name: "下書き保存" }).click();
  await expect(page.getByText("下書きを保存しました。")).toBeVisible();
  await page.getByRole("link", { name: "タグ" }).click();
  await expect(page.getByRole("heading", { level: 1, name: "タグ" })).toBeVisible();
});
