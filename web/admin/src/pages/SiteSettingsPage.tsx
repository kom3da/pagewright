import {
  Box,
  Button,
  Group,
  Image,
  Input,
  NativeSelect,
  Radio,
  Stack,
  Text,
  TextInput,
} from "@mantine/core";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useBlocker } from "@tanstack/react-router";
import { type FormEvent, type MouseEvent, useEffect, useRef, useState } from "react";
import { api, errorField, errorMessage, isApiError } from "../api/client";
import { contentsQuery, meQuery, siteQuery } from "../api/queries";
import type {
  ContentSummary,
  Media,
  SiteFont,
  SiteSettings,
  SiteSettingsInput,
  ThemeInfo,
  ThemeSettingDecl,
} from "../api/types";
import { AdminOnly } from "../components/AdminOnly";
import { ColorField, colorProblem } from "../components/ColorField";
import { MenuEditor, type MenuRow, menuItemProblem, toMenuRows } from "../components/MenuEditor";
import { confirm } from "../components/ui/confirm";
import { Dialog } from "../components/ui/Dialog";
import { Form } from "../components/ui/Form";
import { ErrorMessage, Loading } from "../components/ui/feedback";
import { notify } from "../components/ui/notify";
import { PageHeader } from "../components/ui/PageHeader";
import { Section } from "../components/ui/Section";
import { MediaPicker, UploadForm } from "../editor/MediaDialog";
import { DEFAULT_FOOTER_COLOR, DEFAULT_PRIMARY_COLOR, isHexColor } from "../lib/color";
import { pageRows } from "../lib/pageTree";
import classes from "./SiteSettingsPage.module.css";

const TITLE = "サイトの設定";
const DESCRIPTION =
  "公開サイト全体に関わる設定です。保存すると、公開サイトが自動で作り直されます。";

/** A short list of common zones; a saved zone outside the list is added to the choices. */
const timeZones: [string, string][] = [
  ["Asia/Tokyo", "日本（東京）"],
  ["Asia/Seoul", "韓国（ソウル）"],
  ["Asia/Shanghai", "中国（上海）"],
  ["Asia/Taipei", "台湾（台北）"],
  ["Asia/Singapore", "シンガポール"],
  ["Asia/Bangkok", "タイ（バンコク）"],
  ["Australia/Sydney", "オーストラリア（シドニー）"],
  ["Europe/London", "イギリス（ロンドン）"],
  ["Europe/Paris", "フランス（パリ）"],
  ["America/New_York", "アメリカ東部（ニューヨーク）"],
  ["America/Los_Angeles", "アメリカ西部（ロサンゼルス）"],
  ["Pacific/Honolulu", "ハワイ（ホノルル）"],
  ["UTC", "協定世界時（UTC）"],
];

const DEFAULT_TIME_ZONE = "Asia/Tokyo";

/** The screen's sections, in order. The ids are the h2 ids, used by the table of contents. */
const sections = [
  ["site-basic", "基本"],
  ["site-look", "見た目"],
  ["site-pages", "トップページと案内"],
  ["site-theme", "テーマの項目"],
  ["site-menu", "メニュー"],
  ["site-contact", "お問い合わせの送り先"],
  ["site-time", "日付と時刻"],
] as const;
type SectionId = (typeof sections)[number][0];
const sectionTitle = Object.fromEntries(sections) as Record<SectionId, string>;

/** Theme name → key → value, all as strings ("" = not set; page ids as digits). */
type ThemeValues = Record<string, Record<string, string>>;

type FormState = {
  name: string;
  logo: Media | null;
  /** "" = the server's default theme (never chosen on this site). */
  theme: string;
  primaryColor: string;
  footerColor: string;
  font: SiteFont;
  frontPageId: string;
  privacyPageId: string;
  themeSettings: ThemeValues;
  timeZone: string;
  contactEmail: string;
  menu: MenuRow[];
};

/** Keys are request field names, as in error.field; theme settings are `themeSettings.<key>`. */
type Errors = Record<string, string | undefined>;
const fieldNames = new Set([
  "name",
  "logoMediaId",
  "theme",
  "primaryColor",
  "footerColor",
  "font",
  "frontPageId",
  "privacyPageId",
  "timeZone",
  "contactEmail",
  "menu",
]);
const themeField = (key: string) => `themeSettings.${key}`;

const toForm = (s: SiteSettings): FormState => ({
  name: s.name,
  logo: s.logo,
  theme: s.theme ?? "",
  primaryColor: s.primaryColor ?? "",
  footerColor: s.footerColor ?? "",
  font: s.font ?? "gothic",
  frontPageId: s.frontPageId ? String(s.frontPageId) : "",
  privacyPageId: s.privacyPageId ? String(s.privacyPageId) : "",
  themeSettings: Object.fromEntries(
    Object.entries(s.themeSettings ?? {}).map(([theme, values]) => [
      theme,
      Object.fromEntries(Object.entries(values).map(([k, v]) => [k, String(v)])),
    ]),
  ),
  timeZone: s.timeZone || DEFAULT_TIME_ZONE,
  contactEmail: s.contactEmail ?? "",
  menu: toMenuRows(s.menu),
});

/**
 * Theme values to send. Only installed themes and declared keys are kept (the server
 * refuses the rest); empty values are left out.
 */
function themeSettingsInput(values: ThemeValues, themes: ThemeInfo[]) {
  const out: NonNullable<SiteSettingsInput["themeSettings"]> = {};
  for (const t of themes) {
    const entries = t.settings.flatMap((d) => {
      const v = values[t.name]?.[d.key]?.trim() ?? "";
      if (!v) return [];
      return [[d.key, d.type === "page" ? Number(v) : v] as const];
    });
    if (entries.length > 0) out[t.name] = Object.fromEntries(entries);
  }
  return out;
}

/** PUT /api/site replaces everything, so every setting is sent. Empty values mean defaults. */
const toInput = (f: FormState, themes: ThemeInfo[]): SiteSettingsInput => {
  const themeSettings = themeSettingsInput(f.themeSettings, themes);
  return {
    name: f.name.trim(),
    logoMediaId: f.logo?.id ?? null,
    ...(f.theme && { theme: f.theme }),
    ...(f.primaryColor && { primaryColor: f.primaryColor.toLowerCase() }),
    ...(f.footerColor && { footerColor: f.footerColor.toLowerCase() }),
    font: f.font,
    ...(f.frontPageId && { frontPageId: Number(f.frontPageId) }),
    ...(f.privacyPageId && { privacyPageId: Number(f.privacyPageId) }),
    ...(Object.keys(themeSettings).length > 0 && { themeSettings }),
    timeZone: f.timeZone,
    ...(f.contactEmail.trim() && { contactEmail: f.contactEmail.trim() }),
    menu: f.menu.map(({ label, contentId, url }) => ({ label, contentId, url })),
  };
};

const emailPattern = /^[^\s@<>()]+@[^\s@<>()]+\.[^\s@<>()]+$/;
const telPattern = /^[0-9+()\- ]{1,30}$/;
const settingUrlPattern = /^(https?:\/\/|\/(?!\/))/;
const DEFAULT_SETTING_MAX = 100;

/** Problem with a theme setting value, mirroring CheckSettings in internal/build. */
function themeValueProblem(d: ThemeSettingDecl, raw: string) {
  const v = raw.trim();
  if (!v || d.type === "page") return undefined;
  const max = d.maxLength || DEFAULT_SETTING_MAX;
  if ([...v].length > max) return `${d.label}は${max}文字以内で入力してください。`;
  if (d.type === "tel" && !telPattern.test(v)) {
    return `${d.label}は数字とハイフンで入力してください（例：03-1234-5678）。`;
  }
  if (d.type === "url" && !settingUrlPattern.test(v)) {
    return `${d.label}は「https://」か「/」で始まるURLで入力してください（例：https://example.jp/）。`;
  }
  return undefined;
}

function validate(f: FormState, theme: ThemeInfo | undefined): Errors {
  const e: Errors = {};
  const name = f.name.trim();
  if (!name) e.name = "サイトの名前を入力してください。";
  else if ([...name].length > 100) e.name = "サイトの名前は100文字以内で入力してください。";
  e.primaryColor = colorProblem("テーマカラー", f.primaryColor);
  e.footerColor = colorProblem("フッターの色", f.footerColor);
  for (const d of theme?.settings ?? []) {
    e[themeField(d.key)] = themeValueProblem(d, f.themeSettings[theme?.name ?? ""]?.[d.key] ?? "");
  }
  if (f.contactEmail.trim() && !emailPattern.test(f.contactEmail.trim())) {
    e.contactEmail = "メールアドレスの形式が正しくありません（例：info@example.jp）。";
  }
  e.menu = f.menu.map(menuItemProblem).find(Boolean);
  for (const k of Object.keys(e)) if (!e[k]) delete e[k];
  return e;
}

/** Focuses el and brings it to the middle of the screen, clear of the sticky save bar. */
function focusInView(el: HTMLElement | null | undefined) {
  if (!el) return;
  el.focus({ preventScroll: true });
  el.scrollIntoView?.({ block: "center" });
}

export function SiteSettingsPage() {
  return (
    <AdminOnly title={TITLE}>
      <SiteSettingsLoader />
    </AdminOnly>
  );
}

function SiteSettingsLoader() {
  const { data, isPending, error } = useQuery(siteQuery);
  if (isPending) {
    return (
      <>
        <PageHeader title={TITLE} description={DESCRIPTION} />
        <Loading />
      </>
    );
  }
  if (error) {
    return (
      <>
        <PageHeader title={TITLE} description={DESCRIPTION} />
        <ErrorMessage>{errorMessage(error)}</ErrorMessage>
      </>
    );
  }
  return <SiteSettingsForm initial={data} />;
}

function SiteSettingsForm({ initial }: { initial: SiteSettings }) {
  const queryClient = useQueryClient();
  const themes = initial.themes ?? [];
  const [saved, setSaved] = useState(() => toForm(initial));
  const [form, setForm] = useState(saved);
  const [errors, setErrors] = useState<Errors>({});
  const [logoOpen, setLogoOpen] = useState(false);
  const formRef = useRef<HTMLFormElement>(null);
  const pages = useQuery(contentsQuery("page"));
  const posts = useQuery(contentsQuery("post"));

  // The API does not say which theme is the default; until one is chosen, the first listed
  // theme is shown as selected but nothing is sent, so the server keeps its default.
  const themeName = form.theme || themes[0]?.name || "";
  const theme = themes.find((t) => t.name === themeName);
  // The theme may name its settings the way people look for them (「会社の連絡先」).
  const themeTitle = theme?.settingsTitle || sectionTitle["site-theme"];

  const dirty = JSON.stringify(toInput(form, themes)) !== JSON.stringify(toInput(saved, themes));
  const dirtyRef = useRef(dirty);
  dirtyRef.current = dirty;
  const blocker = useBlocker({
    shouldBlockFn: () => dirtyRef.current,
    enableBeforeUnload: () => dirtyRef.current,
    withResolver: true,
  });
  const blocked = blocker.status === "blocked";
  useEffect(() => {
    if (!blocked) return;
    void confirm({
      title: "保存していない変更があります",
      message: "このまま移動すると、保存していない設定の変更は失われます。",
      cancelLabel: "このページにとどまる",
      confirmLabel: "変更を捨てて移動する",
      danger: true,
    }).then((ok) => (ok ? blocker.proceed?.() : blocker.reset?.()));
  }, [blocked, blocker]);

  // Low contrast is shown as soon as a complete color is entered; other checks run on save.
  const liveColor = (label: string, v: string) =>
    isHexColor(v) ? colorProblem(label, v) : undefined;
  const shown: Errors = {
    ...errors,
    primaryColor: liveColor("テーマカラー", form.primaryColor) ?? errors.primaryColor,
    footerColor: liveColor("フッターの色", form.footerColor) ?? errors.footerColor,
  };

  const clearError = (field: string) =>
    setErrors((e) => ({ ...e, [field]: undefined, root: undefined }));

  const set = <K extends keyof FormState>(key: K, value: FormState[K]) => {
    setForm((f) => ({ ...f, [key]: value }));
    clearError(key === "logo" ? "logoMediaId" : key);
  };

  const setThemeValue = (key: string, value: string) => {
    setForm((f) => ({
      ...f,
      themeSettings: {
        ...f.themeSettings,
        [themeName]: { ...f.themeSettings[themeName], [key]: value },
      },
    }));
    clearError(themeField(key));
  };

  const focusFirstError = () =>
    setTimeout(() =>
      focusInView(
        formRef.current?.querySelector<HTMLElement>("[aria-invalid=true], [data-error-focus]"),
      ),
    );

  const save = useMutation({
    mutationFn: (input: SiteSettingsInput) => api.saveSite(input),
    onSuccess: (site) => {
      queryClient.setQueryData(siteQuery.queryKey, site);
      // The header shows the site name from /api/me.
      void queryClient.invalidateQueries({ queryKey: meQuery.queryKey });
      const next = toForm(site);
      setSaved(next);
      setForm(next);
      notify("サイトの設定を保存しました。公開サイトに反映されるまで少しお待ちください。");
    },
    onError: (e) => {
      const field = errorField(e);
      const known =
        field !== undefined &&
        (fieldNames.has(field) ||
          (theme?.settings.some((d) => themeField(d.key) === field) ?? false));
      if (isApiError(e) && known) {
        setErrors({ [field]: e.message });
        focusFirstError();
      } else {
        setErrors({ root: errorMessage(e) });
      }
    },
  });

  const onSubmit = (e: FormEvent) => {
    e.preventDefault();
    const next = validate(form, theme);
    setErrors(next);
    if (Object.keys(next).length > 0) {
      focusFirstError();
      return;
    }
    save.mutate(toInput(form, themes));
  };

  const contents = [...(pages.data ?? []), ...(posts.data ?? [])];
  const zoneChoices = timeZones.some(([z]) => z === form.timeZone)
    ? timeZones
    : [...timeZones, [form.timeZone, form.timeZone] as [string, string]];
  const pageList = pageRows(pages.data ?? []);

  return (
    <>
      <PageHeader title={TITLE} description={DESCRIPTION} />
      <div className={classes.layout}>
        <TableOfContents titles={{ "site-theme": themeTitle }} />
        <Form ref={formRef} onSubmit={onSubmit} gap="xl" className={classes.form}>
          <Section id="site-basic" title={sectionTitle["site-basic"]}>
            <TextInput
              label="サイトの名前"
              description="公開サイトのいちばん上や、ブラウザのタブに表示されます。"
              required
              maw="40rem"
              value={form.name}
              onChange={(e) => set("name", e.target.value)}
              error={shown.name}
            />
            <Stack gap="xs">
              <Box>
                <Text fw={700}>ロゴ</Text>
                <Input.Description id="logo-hint" size="sm">
                  ロゴの画像を設定すると、サイトの名前の代わりに表示されます。画像の説明（代替テキスト）にはサイトの名前が使われます。
                </Input.Description>
              </Box>
              {form.logo ? (
                <Image
                  src={form.logo.previewUrl}
                  alt={`いまのロゴ：${form.logo.altText || form.logo.filename}`}
                  mah={96}
                  maw={320}
                  w="auto"
                  fit="contain"
                  p="xs"
                  bg="gray.1"
                  radius="sm"
                  className={classes.logo}
                />
              ) : (
                <Text size="sm">ロゴは設定していません（サイトの名前を文字で表示します）。</Text>
              )}
              <Group gap="sm">
                <Button onClick={() => setLogoOpen(true)} aria-describedby="logo-hint">
                  {form.logo ? "ロゴの画像を変える" : "ロゴの画像を選ぶ"}
                </Button>
                {form.logo && (
                  <Button variant="subtle" color="red" onClick={() => set("logo", null)}>
                    ロゴを外す
                  </Button>
                )}
              </Group>
              {shown.logoMediaId && (
                <Input.Error tabIndex={-1} data-error-focus="" size="sm">
                  {shown.logoMediaId}
                </Input.Error>
              )}
            </Stack>
          </Section>

          <Section
            id="site-look"
            title={sectionTitle["site-look"]}
            description="公開サイトのデザインと色、文字の書体です。"
          >
            {themes.length > 0 && (
              <Radio.Group
                label="テーマ（サイトのデザイン）"
                description="ページの並べ方や飾りなど、公開サイト全体の作りが変わります。テーマを切り替えても、それぞれのテーマの項目に入力した内容は残ります。"
                name="theme"
                value={themeName}
                onChange={(v) => set("theme", v)}
                error={shown.theme}
              >
                <Stack gap="xs" mt="xs">
                  {themes.map((t) => (
                    <Radio
                      key={t.name}
                      value={t.name}
                      label={t.title || t.name}
                      description={t.description || undefined}
                      classNames={{ root: classes.themeOption, label: classes.themeTitle }}
                      data-error-focus={shown.theme && t.name === themeName ? "" : undefined}
                    />
                  ))}
                </Stack>
              </Radio.Group>
            )}
            <ColorField
              label="テーマカラー"
              hint="リンクやボタン、見出しの飾りに使う色です。白い文字を乗せたり白い背景の上に置いたりするので、濃い色を選んでください。"
              value={form.primaryColor}
              defaultColor={DEFAULT_PRIMARY_COLOR}
              error={shown.primaryColor}
              onChange={(v) => set("primaryColor", v)}
            />
            <ColorField
              label="フッターの色"
              hint="ページのいちばん下（フッター）の背景色です。白い文字が乗ります。"
              value={form.footerColor}
              defaultColor={DEFAULT_FOOTER_COLOR}
              error={shown.footerColor}
              onChange={(v) => set("footerColor", v)}
            />
            <Radio.Group
              label="文字の書体"
              name="font"
              value={form.font}
              onChange={(v) => set("font", v as SiteFont)}
              error={shown.font}
            >
              <Stack gap="xs" mt="xs">
                {(
                  [
                    ["gothic", "ゴシック体（標準）", "sans-serif"],
                    ["mincho", "明朝体", "serif"],
                  ] as const
                ).map(([value, text, family]) => (
                  <Radio
                    key={value}
                    value={value}
                    label={<span style={{ fontFamily: family }}>{text}</span>}
                    data-error-focus={shown.font && value === form.font ? "" : undefined}
                  />
                ))}
              </Stack>
            </Radio.Group>
          </Section>

          <Section
            id="site-pages"
            title={sectionTitle["site-pages"]}
            description="トップページや、お問い合わせフォームから案内する固定ページを選びます。"
          >
            {pages.isPending ? (
              <Loading />
            ) : (
              <>
                <PageSelect
                  label="トップに載せる固定ページ"
                  description="選んだページのタイトルと本文を、トップページのいちばん上に載せます。どのように見せるかはテーマによって違います。"
                  emptyLabel="載せない"
                  pages={pageList}
                  value={form.frontPageId}
                  onChange={(v) => set("frontPageId", v)}
                  error={shown.frontPageId}
                />
                <PageSelect
                  label="個人情報の取り扱いのページ"
                  description="お問い合わせフォームの「同意する」の欄から、このページにリンクします。お問い合わせフォームを使うときは選んでください。"
                  emptyLabel="選ばない"
                  pages={pageList}
                  value={form.privacyPageId}
                  onChange={(v) => set("privacyPageId", v)}
                  error={shown.privacyPageId}
                />
              </>
            )}
          </Section>

          <Section
            id="site-theme"
            title={themeTitle}
            description={
              theme
                ? `${theme.settingsDescription || "公開サイトに表示する内容です。"}この項目は「${theme.title || theme.name}」テーマのものです。空欄の項目は表示されません。`
                : undefined
            }
          >
            {!theme || theme.settings.length === 0 ? (
              <Text size="sm">このテーマには、ここで入力する項目はありません。</Text>
            ) : pages.isPending && theme.settings.some((d) => d.type === "page") ? (
              <Loading />
            ) : (
              theme.settings.map((d) => (
                <ThemeSettingInput
                  key={`${theme.name}.${d.key}`}
                  decl={d}
                  pages={pageList}
                  value={form.themeSettings[theme.name]?.[d.key] ?? ""}
                  onChange={(v) => setThemeValue(d.key, v)}
                  error={shown[themeField(d.key)]}
                />
              ))
            )}
          </Section>

          <Section
            id="site-menu"
            title={sectionTitle["site-menu"]}
            description="公開サイトのいちばん上に並ぶリンクです。上下のボタンで順番を変えられます。公開していないページや記事は、メニューに入れてもサイトには表示されません。"
          >
            {pages.isPending || posts.isPending ? (
              <Loading />
            ) : (
              <MenuEditor
                items={form.menu}
                onChange={(menu) => set("menu", menu)}
                contents={contents}
                error={shown.menu}
                errorId="menu-error"
              />
            )}
          </Section>

          <Section id="site-contact" title={sectionTitle["site-contact"]}>
            <TextInput
              label="お問い合わせの送り先メールアドレス"
              description="お問い合わせフォームから届いた内容を、このアドレスにメールで知らせます。空欄にすると、このサイトの管理者全員に届きます。"
              type="email"
              inputMode="email"
              autoComplete="email"
              spellCheck={false}
              maw="40rem"
              value={form.contactEmail}
              onChange={(e) => set("contactEmail", e.target.value)}
              error={shown.contactEmail}
            />
          </Section>

          <Section id="site-time" title={sectionTitle["site-time"]}>
            <NativeSelect
              label="タイムゾーン"
              description="公開サイトに表示する日付や、予約公開の時刻の基準になる地域です。日本のサイトなら「日本（東京）」のままにしてください。"
              error={shown.timeZone}
              value={form.timeZone}
              onChange={(e) => set("timeZone", e.currentTarget.value)}
              maw="24rem"
            >
              {zoneChoices.map(([z, label]) => (
                <option key={z} value={z}>
                  {label}
                </option>
              ))}
            </NativeSelect>
          </Section>

          {shown.root && <ErrorMessage>{shown.root}</ErrorMessage>}
          <div className={classes.saveBar}>
            <Text size="sm" fw={700} aria-live="polite" className={classes.status}>
              {dirty && (
                <>
                  <span aria-hidden="true" className={classes.dot} />
                  保存していない変更があります
                </>
              )}
            </Text>
            <Button type="submit" variant="filled" loading={save.isPending}>
              設定を保存する
            </Button>
          </div>
        </Form>
      </div>

      <Dialog open={logoOpen} onOpenChange={setLogoOpen} title="ロゴの画像を選ぶ" wide>
        {logoOpen && (
          <Stack gap="xl">
            <UploadForm
              defaultAlt={form.name}
              submitLabel="アップロードしてロゴにする"
              onUploaded={(m) => {
                set("logo", m);
                setLogoOpen(false);
              }}
            />
            <MediaPicker
              withAlt={false}
              pickLabel="この画像をロゴにする"
              inBody={(id) => id === form.logo?.id || id === saved.logo?.id}
              inUseMessage="この画像はロゴに使っているため削除できません。"
              onPick={(m) => {
                set("logo", m);
                setLogoOpen(false);
              }}
            />
          </Stack>
        )}
      </Dialog>
    </>
  );
}

/** Links to the sections (wide screens only). Moves focus to the section's heading. */
function TableOfContents({ titles }: { titles: Partial<Record<SectionId, string>> }) {
  const jump = (e: MouseEvent<HTMLAnchorElement>, id: string) => {
    // Not a hash navigation: the router would treat it as leaving the page.
    e.preventDefault();
    const heading = document.getElementById(id);
    if (!heading) return;
    heading.setAttribute("tabindex", "-1");
    heading.focus({ preventScroll: true });
    heading.scrollIntoView?.({ block: "start" });
  };
  return (
    <nav aria-labelledby="site-toc-title" className={classes.toc}>
      <Text id="site-toc-title" size="sm" fw={700} c="dimmed" mb="xs">
        設定の項目
      </Text>
      <ul className={classes.tocList}>
        {sections.map(([id, title]) => (
          <li key={id}>
            <a href={`#${id}`} onClick={(e) => jump(e, id)} className={classes.tocLink}>
              {titles[id] ?? title}
            </a>
          </li>
        ))}
      </ul>
    </nav>
  );
}

const pageOptionLabel = (p: ContentSummary & { depth: number }) =>
  `${"　".repeat(p.depth)}${p.title || "（タイトルなし）"}${
    p.status === "published" ? "" : "（公開していません）"
  }`;

function PageSelect({
  label,
  description,
  emptyLabel,
  pages,
  value,
  onChange,
  error,
}: {
  label: string;
  description?: string;
  emptyLabel: string;
  pages: (ContentSummary & { depth: number })[];
  value: string;
  onChange: (value: string) => void;
  error?: string;
}) {
  const missing = value !== "" && !pages.some((p) => String(p.id) === value);
  return (
    <NativeSelect
      label={label}
      description={description}
      error={error}
      value={value}
      onChange={(e) => onChange(e.currentTarget.value)}
      maw="40rem"
    >
      <option value="">{emptyLabel}</option>
      {pages.map((p) => (
        <option key={p.id} value={String(p.id)}>
          {pageOptionLabel(p)}
        </option>
      ))}
      {missing && <option value={value}>（見つからないページ。選び直してください）</option>}
    </NativeSelect>
  );
}

/** Hints added after the theme's own description, by input type. */
const typeHint = {
  text: (max: number) => `${max}文字まで。`,
  tel: () => "数字とハイフンで入力します（例：03-1234-5678）。",
  url: () => "「https://」か「/」で始まるURLを入力します。",
} as const;

/** One setting declared by the theme. Nothing here knows what the keys mean. */
function ThemeSettingInput({
  decl,
  pages,
  value,
  onChange,
  error,
}: {
  decl: ThemeSettingDecl;
  pages: (ContentSummary & { depth: number })[];
  value: string;
  onChange: (value: string) => void;
  error?: string;
}) {
  if (decl.type === "page") {
    return (
      <PageSelect
        label={decl.label}
        description={decl.description || undefined}
        emptyLabel="選ばない"
        pages={pages}
        value={value}
        onChange={onChange}
        error={error}
      />
    );
  }
  const hint = typeHint[decl.type](decl.maxLength || DEFAULT_SETTING_MAX);
  const typeProps = {
    text: { type: "text" },
    tel: { type: "tel", inputMode: "tel" },
    url: { type: "url", inputMode: "url", spellCheck: false },
  } as const;
  return (
    <TextInput
      label={decl.label}
      description={[decl.description, hint].filter(Boolean).join(" ")}
      // The site's own details, not the signed-in person's: no autofill.
      autoComplete="off"
      maw="40rem"
      {...typeProps[decl.type]}
      value={value}
      onChange={(e) => onChange(e.target.value)}
      error={error}
    />
  );
}
