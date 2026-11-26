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
    ["settings", "サイトの設定"],
    ["inquiries", "お問い合わせ"],
  ] as const) {
    await page.goto(path);
    await expect(page.getByRole("heading", { level: 1, name: heading })).toBeVisible();
    await expectAccessible(page);
  }
});

test("site settings: contrast check, keyboard menu editing and saving", async ({ page }) => {
  await page.goto("settings");
  await expect(page.getByRole("heading", { level: 1, name: "サイトの設定" })).toBeVisible();

  // A light theme color is explained and blocks saving.
  const theme = page.getByRole("group", { name: "テーマカラー" });
  await theme.getByLabel(/色コード/).fill("#7fb3ff");
  await expect(theme.getByText(/明るすぎて/)).toBeVisible();
  await page.getByRole("button", { name: "設定を保存する" }).click();
  await expect(theme.getByLabel(/色コード/)).toBeFocused();
  await theme.getByLabel(/色コード/).fill("#0a4f8f");
  await expect(theme.getByText(/明るすぎて/)).toBeHidden();

  // Add two menu items and reorder them with the keyboard only.
  const menu = page.getByRole("region", { name: "メニュー" });
  for (const title of ["会社概要", "沿革"]) {
    await menu.getByRole("button", { name: "メニューに項目を追加" }).click();
    const dialog = page.getByRole("dialog", { name: "メニューに項目を追加" });
    await dialog.getByRole("combobox", { name: /ページや記事/ }).selectOption({ label: title });
    await dialog.getByRole("button", { name: "追加する" }).click();
    await expect(dialog).toBeHidden();
  }
  await expectAccessible(page);
  await menu.getByRole("button", { name: "「沿革」を上へ移動" }).focus();
  await page.keyboard.press("Enter");
  await expect(menu.getByRole("listitem").first()).toContainText("沿革");
  await expect(menu.getByRole("button", { name: "「沿革」を下へ移動" })).toBeFocused();

  await page.getByRole("button", { name: "設定を保存する" }).click();
  await expect(page.getByText(/サイトの設定を保存しました/)).toBeVisible();
  await page.getByRole("link", { name: "タグ" }).click();
  await expect(page.getByRole("heading", { level: 1, name: "タグ" })).toBeVisible();
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

test("scheduling a post is accessible and keyboard operable", async ({ page }) => {
  await page.goto("posts/1");
  await page.getByRole("button", { name: "日時を指定して公開" }).click();
  const dialog = page.getByRole("dialog", { name: "日時を指定して公開" });
  await expect(dialog.getByLabel("日付")).toBeVisible();
  await expectAccessible(page);

  await dialog.getByLabel("日付").fill("2099-12-01");
  await dialog.getByLabel("時刻").fill("09:00");
  await dialog.getByLabel("時刻").press("Enter");
  await expect(dialog).toBeHidden();
  await expect(page.getByText("予約：2099年12月1日 9:00に公開")).toBeVisible();
  await expectAccessible(page);

  await page.getByRole("button", { name: "予約を取り消す" }).click();
  await expect(page.getByText("予約を取り消しました。")).toBeVisible();
  await expect(page.getByText(/予約：/)).toBeHidden();
});
