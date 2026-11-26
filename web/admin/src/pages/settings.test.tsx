import { screen, waitFor, within } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import type { Media, SiteSettingsInput } from "../api/types";
import { db } from "../mocks/handlers";
import { expectNoAxeViolations, failWith, recordRequests, renderApp } from "../test/utils";

const editorSite = "22222222-2222-4222-8222-222222222222";

async function openSettings() {
  const r = renderApp("/settings");
  await screen.findByLabelText(/サイトの名前/);
  // The menu section waits for the contents.
  await screen.findByRole("button", { name: "メニューに項目を追加" });
  return r;
}

const save = async (user: Awaited<ReturnType<typeof openSettings>>["user"]) =>
  user.click(screen.getByRole("button", { name: "設定を保存する" }));

const logo: Media = {
  id: 50,
  url: "/media/50/logo.png",
  previewUrl: "/api/media/50/file",
  filename: "logo.png",
  contentType: "image/png",
  sizeBytes: 10,
  width: null,
  height: null,
  altText: "ロゴ",
  createdAt: "2026-11-20T00:00:00Z",
};

describe("site settings", () => {
  it("shows the settings to admins without accessibility violations", async () => {
    await openSettings();
    expect(screen.getByRole("heading", { level: 1, name: "サイトの設定" })).toBeInTheDocument();
    expect(screen.getByLabelText(/サイトの名前/)).toHaveValue("山田工務店");
    expect(screen.getByRole("radio", { name: "ゴシック体（標準）" })).toBeChecked();
    expect(screen.getByLabelText("タイムゾーン")).toHaveValue("Asia/Tokyo");
    expect(screen.getByLabelText(/送り先メールアドレス/)).toHaveAccessibleDescription(
      /空欄にすると、このサイトの管理者全員に届きます/,
    );
    expect(screen.getByText(/親のない固定ページと「お知らせ」/)).toBeInTheDocument();
    const nav = screen.getByRole("navigation", { name: "メインメニュー" });
    expect(within(nav).getByRole("link", { name: "サイトの設定" })).toBeInTheDocument();
    expect(within(nav).getByRole("link", { name: "お問い合わせ" })).toBeInTheDocument();
    await expectNoAxeViolations();
  });

  it("shows the contrast with white and blocks saving a light color", async () => {
    const requests = recordRequests();
    const { user } = await openSettings();
    const theme = screen.getByRole("group", { name: "テーマカラー" });
    const code = within(theme).getByLabelText(/色コード/);
    expect(code).toHaveAccessibleDescription(/コントラスト比：8\.\d:1（基準の 4\.5:1 以上です）/);

    await user.type(code, "#7fb3ff");
    expect(code).toHaveAttribute("aria-invalid", "true");
    expect(code).toHaveAccessibleDescription(
      /明るすぎて.*コントラスト比 2\.\d:1.*4\.5:1 以上が必要/,
    );
    expect(within(theme).getByText(/基準の 4\.5:1 に足りません/)).toBeInTheDocument();

    await save(user);
    await waitFor(() => expect(code).toHaveFocus());
    expect(requests.filter((r) => r.method === "PUT")).toEqual([]);

    await user.click(within(theme).getByRole("button", { name: "標準の色に戻す" }));
    expect(code).toHaveValue("");
    expect(code).not.toHaveAttribute("aria-invalid");
  });

  it("checks the color code format on save", async () => {
    const { user } = await openSettings();
    const footer = within(screen.getByRole("group", { name: "フッターの色" })).getByLabelText(
      /色コード/,
    );
    await user.type(footer, "#12");
    expect(footer).not.toHaveAttribute("aria-invalid");
    await save(user);
    await waitFor(() => expect(footer).toHaveFocus());
    expect(footer).toHaveAccessibleDescription(/# のあとに英数字6桁/);
  });

  it("saves every setting with PUT /api/site", async () => {
    const requests = recordRequests();
    const { user } = await openSettings();
    const name = screen.getByLabelText(/サイトの名前/);
    await user.clear(name);
    await user.type(name, "山田工務店 本店");
    const theme = within(screen.getByRole("group", { name: "テーマカラー" })).getByLabelText(
      /色コード/,
    );
    await user.type(theme, "#0A4F8F");
    await user.click(screen.getByRole("radio", { name: "明朝体" }));
    await user.selectOptions(screen.getByLabelText("タイムゾーン"), "Asia/Seoul");
    await user.type(screen.getByLabelText(/送り先メールアドレス/), "info@example.jp");
    expect(screen.getByText("保存していない変更があります")).toBeInTheDocument();
    await save(user);
    expect(await screen.findByText(/サイトの設定を保存しました/)).toBeInTheDocument();

    const put = requests.find((r) => r.method === "PUT");
    expect(put?.path).toBe("/api/site");
    expect(put?.body).toEqual({
      name: "山田工務店 本店",
      logoMediaId: null,
      primaryColor: "#0a4f8f",
      font: "mincho",
      timeZone: "Asia/Seoul",
      contactEmail: "info@example.jp",
      menu: [],
    } satisfies SiteSettingsInput);
    expect(db.site).toMatchObject({ name: "山田工務店 本店", font: "mincho" });
    expect(screen.queryByText("保存していない変更があります")).not.toBeInTheDocument();
  });

  it("shows a server error at the field named by error.field", async () => {
    failWith(
      "put",
      "/api/site",
      422,
      "invalid_setting",
      "お問い合わせの送り先メールアドレスの形式が正しくありません",
      "contactEmail",
    );
    const { user } = await openSettings();
    const email = screen.getByLabelText(/送り先メールアドレス/);
    await user.type(email, "info@example.jp");
    await save(user);
    await waitFor(() => expect(email).toHaveFocus());
    expect(email).toHaveAccessibleDescription(/形式が正しくありません/);
  });

  it("validates the name and e-mail before sending", async () => {
    const { user } = await openSettings();
    const name = screen.getByLabelText(/サイトの名前/);
    await user.clear(name);
    await user.type(screen.getByLabelText(/送り先メールアドレス/), "info@");
    await save(user);
    await waitFor(() => expect(name).toHaveFocus());
    expect(name).toHaveAccessibleDescription(/サイトの名前を入力してください/);
    expect(screen.getByLabelText(/送り先メールアドレス/)).toHaveAccessibleDescription(
      /形式が正しくありません/,
    );
  });

  it("picks a logo from the media library and removes it", async () => {
    db.media.push(logo);
    const requests = recordRequests();
    const { user } = await openSettings();
    await user.click(screen.getByRole("button", { name: "ロゴの画像を選ぶ" }));
    const dialog = await screen.findByRole("dialog", { name: "ロゴの画像を選ぶ" });
    await user.click(await within(dialog).findByRole("button", { name: "ロゴ" }));
    // Only the upload form asks for alt text; the logo's alt is the site name.
    expect(within(dialog).getAllByLabelText(/代替テキスト/)).toHaveLength(1);
    await user.click(within(dialog).getByRole("button", { name: "この画像をロゴにする" }));
    expect(screen.getByRole("img", { name: "いまのロゴ：ロゴ" })).toBeInTheDocument();
    await save(user);
    await screen.findByText(/サイトの設定を保存しました/);
    expect(requests.find((r) => r.method === "PUT")?.body).toMatchObject({ logoMediaId: 50 });
    expect(db.site.logoMediaId).toBe(50);

    await user.click(screen.getByRole("button", { name: "ロゴを外す" }));
    expect(screen.getByText(/ロゴは設定していません/)).toBeInTheDocument();
  });

  it("hides the settings from editors", async () => {
    db.me.currentSiteId = editorSite;
    renderApp("/settings");
    expect(await screen.findByText(/サイトの管理者だけが使えます/)).toBeInTheDocument();
    expect(screen.queryByLabelText(/サイトの名前/)).toBeNull();
    const nav = screen.getByRole("navigation", { name: "メインメニュー" });
    expect(within(nav).queryByRole("link", { name: "サイトの設定" })).toBeNull();
    expect(within(nav).queryByRole("link", { name: "お問い合わせ" })).toBeNull();
  });
});

describe("menu editor", () => {
  const menuItems = () =>
    within(screen.getByRole("region", { name: "メニュー" }))
      .queryAllByRole("listitem")
      .map((li) => li.querySelector("p")?.textContent);

  it("adds a page and a URL, reorders with buttons, removes, and saves", async () => {
    const requests = recordRequests();
    const { user } = await openSettings();

    await user.click(screen.getByRole("button", { name: "メニューに項目を追加" }));
    let dialog = await screen.findByRole("dialog", { name: "メニューに項目を追加" });
    await user.selectOptions(
      within(dialog).getByRole("combobox", { name: /ページや記事/ }),
      "会社概要",
    );
    await user.click(within(dialog).getByRole("button", { name: "追加する" }));

    await user.click(screen.getByRole("button", { name: "メニューに項目を追加" }));
    dialog = await screen.findByRole("dialog", { name: "メニューに項目を追加" });
    await user.click(within(dialog).getByRole("radio", { name: /URLを入力する/ }));
    const url = within(dialog).getByRole("textbox", { name: /^URL/ });
    await user.type(url, "example.jp");
    await user.click(within(dialog).getByRole("button", { name: "追加する" }));
    expect(url).toHaveAccessibleDescription(/「\/」か「https:\/\/」で始めて/);
    expect(within(dialog).getByLabelText(/表示する名前/)).toHaveAccessibleDescription(
      /表示する名前を入力してください/,
    );
    await user.clear(url);
    await user.type(url, "https://example.jp/shop/");
    await user.type(within(dialog).getByLabelText(/表示する名前/), "通販サイト");
    await user.click(within(dialog).getByRole("button", { name: "追加する" }));
    expect(menuItems()).toEqual(["会社概要", "通販サイト"]);
    expect(screen.queryByText(/親のない固定ページと「お知らせ」/)).toBeNull();

    // Move up: focus stays on the moved item (its 下へ, as 上へ is now disabled).
    await user.click(screen.getByRole("button", { name: "「通販サイト」を上へ移動" }));
    expect(menuItems()).toEqual(["通販サイト", "会社概要"]);
    expect(screen.getByRole("button", { name: "「通販サイト」を上へ移動" })).toBeDisabled();
    expect(screen.getByRole("button", { name: "「通販サイト」を下へ移動" })).toHaveFocus();
    expect(screen.getByText("「通販サイト」を1番目に移動しました。")).toBeInTheDocument();

    // Edit the page's label.
    await user.click(screen.getByRole("button", { name: "「会社概要」を編集" }));
    dialog = await screen.findByRole("dialog", { name: "メニューの項目を編集" });
    await user.type(within(dialog).getByLabelText(/表示する名前/), "私たちについて");
    await user.click(within(dialog).getByRole("button", { name: "変更する" }));
    expect(menuItems()).toEqual(["通販サイト", "私たちについて"]);

    await save(user);
    await screen.findByText(/サイトの設定を保存しました/);
    expect(requests.find((r) => r.method === "PUT")?.body).toMatchObject({
      menu: [
        { label: "通販サイト", contentId: null, url: "https://example.jp/shop/" },
        { label: "私たちについて", contentId: 4, url: null },
      ],
    });

    await user.click(screen.getByRole("button", { name: "「通販サイト」をメニューから外す" }));
    expect(menuItems()).toEqual(["私たちについて"]);
    expect(
      screen.getByRole("button", { name: "「私たちについて」をメニューから外す" }),
    ).toHaveFocus();
    await expectNoAxeViolations();
  });

  it("notes unpublished contents and shows menu errors from the server", async () => {
    db.site.menu = [{ label: null, contentId: 6, url: null }];
    failWith(
      "put",
      "/api/site",
      422,
      "invalid_menu",
      "メニューの1番目は、ページかURLのどちらか一方を指定してください",
      "menu",
    );
    const { user } = await openSettings();
    expect(screen.getByText(/公開していないため、いまはサイトに出ません/)).toBeInTheDocument();
    await save(user);
    const error = await screen.findByText(/メニューの1番目は/);
    await waitFor(() => expect(error).toHaveFocus());
  });
});
