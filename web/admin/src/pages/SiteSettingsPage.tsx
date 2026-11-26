import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useBlocker } from "@tanstack/react-router";
import { type FormEvent, type ReactNode, useRef, useState } from "react";
import { api, errorField, errorMessage, isApiError } from "../api/client";
import { contentsQuery, meQuery, siteQuery } from "../api/queries";
import type { Media, SiteFont, SiteSettings, SiteSettingsInput } from "../api/types";
import { AdminOnly } from "../components/AdminOnly";
import { ColorField, colorProblem } from "../components/ColorField";
import { MenuEditor, type MenuRow, menuItemProblem, toMenuRows } from "../components/MenuEditor";
import { useNotify } from "../components/Toaster";
import { Button } from "../components/ui/Button";
import { ConfirmDialog, Dialog } from "../components/ui/Dialog";
import { Field, inputClass, TextField } from "../components/ui/Field";
import { ErrorMessage, Loading, PageHeading } from "../components/ui/misc";
import { MediaPicker, UploadForm } from "../editor/MediaDialog";
import { DEFAULT_FOOTER_COLOR, DEFAULT_PRIMARY_COLOR, isHexColor } from "../lib/color";

const TITLE = "サイトの設定";

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

type FormState = {
  name: string;
  logo: Media | null;
  primaryColor: string;
  footerColor: string;
  font: SiteFont;
  timeZone: string;
  contactEmail: string;
  menu: MenuRow[];
};

type FieldName =
  | "name"
  | "logoMediaId"
  | "primaryColor"
  | "footerColor"
  | "font"
  | "timeZone"
  | "contactEmail"
  | "menu";
type Errors = Partial<Record<FieldName | "root", string>>;
const fieldNames = new Set<string>([
  "name",
  "logoMediaId",
  "primaryColor",
  "footerColor",
  "font",
  "timeZone",
  "contactEmail",
  "menu",
]);

const toForm = (s: SiteSettings): FormState => ({
  name: s.name,
  logo: s.logo,
  primaryColor: s.primaryColor ?? "",
  footerColor: s.footerColor ?? "",
  font: s.font ?? "gothic",
  timeZone: s.timeZone || DEFAULT_TIME_ZONE,
  contactEmail: s.contactEmail ?? "",
  menu: toMenuRows(s.menu),
});

/** PUT /api/site replaces everything, so every setting is sent. Empty values mean defaults. */
const toInput = (f: FormState): SiteSettingsInput => ({
  name: f.name.trim(),
  logoMediaId: f.logo?.id ?? null,
  ...(f.primaryColor && { primaryColor: f.primaryColor.toLowerCase() }),
  ...(f.footerColor && { footerColor: f.footerColor.toLowerCase() }),
  font: f.font,
  timeZone: f.timeZone,
  ...(f.contactEmail.trim() && { contactEmail: f.contactEmail.trim() }),
  menu: f.menu.map(({ label, contentId, url }) => ({ label, contentId, url })),
});

const emailPattern = /^[^\s@<>()]+@[^\s@<>()]+\.[^\s@<>()]+$/;

function validate(f: FormState): Errors {
  const e: Errors = {};
  const name = f.name.trim();
  if (!name) e.name = "サイトの名前を入力してください。";
  else if ([...name].length > 100) e.name = "サイトの名前は100文字以内で入力してください。";
  e.primaryColor = colorProblem("テーマカラー", f.primaryColor);
  e.footerColor = colorProblem("フッターの色", f.footerColor);
  if (f.contactEmail.trim() && !emailPattern.test(f.contactEmail.trim())) {
    e.contactEmail = "メールアドレスの形式が正しくありません（例：info@example.jp）。";
  }
  e.menu = f.menu.map(menuItemProblem).find(Boolean);
  for (const k of Object.keys(e) as (keyof Errors)[]) if (!e[k]) delete e[k];
  return e;
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
        <PageHeading title={TITLE}>{TITLE}</PageHeading>
        <Loading />
      </>
    );
  }
  if (error) {
    return (
      <>
        <PageHeading title={TITLE}>{TITLE}</PageHeading>
        <ErrorMessage>{errorMessage(error)}</ErrorMessage>
      </>
    );
  }
  return <SiteSettingsForm initial={data} />;
}

function Section({ id, title, children }: { id: string; title: string; children: ReactNode }) {
  return (
    <section
      aria-labelledby={id}
      className="space-y-4 rounded-lg border border-slate-300 bg-white p-4 sm:p-6"
    >
      <h2 id={id} className="text-lg font-bold">
        {title}
      </h2>
      {children}
    </section>
  );
}

function SiteSettingsForm({ initial }: { initial: SiteSettings }) {
  const queryClient = useQueryClient();
  const notify = useNotify();
  const [saved, setSaved] = useState(() => toForm(initial));
  const [form, setForm] = useState(saved);
  const [errors, setErrors] = useState<Errors>({});
  const [logoOpen, setLogoOpen] = useState(false);
  const formRef = useRef<HTMLFormElement>(null);
  const pages = useQuery(contentsQuery("page"));
  const posts = useQuery(contentsQuery("post"));

  const dirty = JSON.stringify(toInput(form)) !== JSON.stringify(toInput(saved));
  const dirtyRef = useRef(dirty);
  dirtyRef.current = dirty;
  const blocker = useBlocker({
    shouldBlockFn: () => dirtyRef.current,
    enableBeforeUnload: () => dirtyRef.current,
    withResolver: true,
  });

  // Low contrast is shown as soon as a complete color is entered; other checks run on save.
  const liveColor = (label: string, v: string) =>
    isHexColor(v) ? colorProblem(label, v) : undefined;
  const shown: Errors = {
    ...errors,
    primaryColor: liveColor("テーマカラー", form.primaryColor) ?? errors.primaryColor,
    footerColor: liveColor("フッターの色", form.footerColor) ?? errors.footerColor,
  };

  const set = <K extends keyof FormState>(key: K, value: FormState[K]) => {
    setForm((f) => ({ ...f, [key]: value }));
    const field = key === "logo" ? "logoMediaId" : key;
    setErrors((e) => ({ ...e, [field]: undefined, root: undefined }));
  };

  const focusFirstError = () =>
    setTimeout(() =>
      formRef.current
        ?.querySelector<HTMLElement>("[aria-invalid=true], [data-error-focus]")
        ?.focus(),
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
      if (isApiError(e) && field && fieldNames.has(field)) {
        setErrors({ [field]: e.message });
        focusFirstError();
      } else {
        setErrors({ root: errorMessage(e) });
      }
    },
  });

  const onSubmit = (e: FormEvent) => {
    e.preventDefault();
    const next = validate(form);
    setErrors(next);
    if (Object.keys(next).length > 0) {
      focusFirstError();
      return;
    }
    save.mutate(toInput(form));
  };

  const contents = [...(pages.data ?? []), ...(posts.data ?? [])];
  const zoneChoices = timeZones.some(([z]) => z === form.timeZone)
    ? timeZones
    : [...timeZones, [form.timeZone, form.timeZone] as [string, string]];

  return (
    <div className="space-y-6">
      <PageHeading title={TITLE}>{TITLE}</PageHeading>
      <p>公開サイト全体に関わる設定です。保存すると、公開サイトが自動で作り直されます。</p>
      <form ref={formRef} noValidate onSubmit={onSubmit} className="space-y-6">
        <Section id="site-basic" title="基本">
          <TextField
            label="サイトの名前"
            hint="公開サイトのいちばん上や、ブラウザのタブに表示されます。"
            required
            value={form.name}
            onChange={(e) => set("name", e.target.value)}
            error={shown.name}
          />
          <div className="space-y-2">
            <p className="font-semibold">ロゴ</p>
            <p className="text-sm text-slate-700" id="logo-hint">
              ロゴの画像を設定すると、サイトの名前の代わりに表示されます。画像の説明（代替テキスト）にはサイトの名前が使われます。
            </p>
            {form.logo ? (
              <img
                src={form.logo.previewUrl}
                alt={`いまのロゴ：${form.logo.altText || form.logo.filename}`}
                className="max-h-24 max-w-xs rounded border border-slate-300 bg-slate-100 p-2"
              />
            ) : (
              <p className="text-sm">ロゴは設定していません（サイトの名前を文字で表示します）。</p>
            )}
            <div className="flex flex-wrap gap-3">
              <Button onClick={() => setLogoOpen(true)} aria-describedby="logo-hint">
                {form.logo ? "ロゴの画像を変える" : "ロゴの画像を選ぶ"}
              </Button>
              {form.logo && (
                <Button variant="ghost" className="text-red-700" onClick={() => set("logo", null)}>
                  ロゴを外す
                </Button>
              )}
            </div>
            {shown.logoMediaId && (
              <p tabIndex={-1} data-error-focus="" className="text-sm font-semibold text-red-700">
                {shown.logoMediaId}
              </p>
            )}
          </div>
        </Section>

        <Section id="site-look" title="見た目">
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
          <fieldset className="space-y-2">
            <legend className="font-semibold">文字の書体</legend>
            {(
              [
                ["gothic", "ゴシック体（標準）", "sans-serif"],
                ["mincho", "明朝体", "serif"],
              ] as const
            ).map(([value, text, family]) => (
              <label key={value} className="flex items-center gap-2">
                <input
                  type="radio"
                  name="font"
                  value={value}
                  checked={form.font === value}
                  onChange={() => set("font", value)}
                  className="size-5"
                />
                <span style={{ fontFamily: family }}>{text}</span>
              </label>
            ))}
            {shown.font && (
              <p tabIndex={-1} data-error-focus="" className="text-sm font-semibold text-red-700">
                {shown.font}
              </p>
            )}
          </fieldset>
        </Section>

        <Section id="site-time" title="日付と時刻">
          <Field
            label="タイムゾーン"
            hint="公開サイトに表示する日付や、予約公開の時刻の基準になる地域です。日本のサイトなら「日本（東京）」のままにしてください。"
            error={shown.timeZone}
          >
            {(ids) => (
              <select
                {...ids}
                value={form.timeZone}
                onChange={(e) => set("timeZone", e.target.value)}
                className={`${inputClass} sm:w-96`}
              >
                {zoneChoices.map(([z, label]) => (
                  <option key={z} value={z}>
                    {label}
                  </option>
                ))}
              </select>
            )}
          </Field>
        </Section>

        <Section id="site-contact" title="お問い合わせ">
          <TextField
            label="お問い合わせの送り先メールアドレス"
            hint="お問い合わせフォームから届いた内容を、このアドレスにメールで知らせます。空欄にすると、このサイトの管理者全員に届きます。"
            type="email"
            inputMode="email"
            autoComplete="email"
            spellCheck={false}
            value={form.contactEmail}
            onChange={(e) => set("contactEmail", e.target.value)}
            error={shown.contactEmail}
          />
        </Section>

        <Section id="site-menu" title="メニュー">
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

        {shown.root && <ErrorMessage>{shown.root}</ErrorMessage>}
        <div className="flex flex-wrap items-center justify-end gap-3">
          <p className="text-sm text-slate-700" aria-live="polite">
            {dirty ? "保存していない変更があります" : ""}
          </p>
          <Button type="submit" variant="primary" disabled={save.isPending}>
            {save.isPending ? "保存しています…" : "設定を保存する"}
          </Button>
        </div>
      </form>

      <Dialog open={logoOpen} onOpenChange={setLogoOpen} title="ロゴの画像を選ぶ" wide>
        {logoOpen && (
          <div className="space-y-8">
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
          </div>
        )}
      </Dialog>

      <ConfirmDialog
        open={blocker.status === "blocked"}
        onOpenChange={(o) => !o && blocker.reset?.()}
        title="保存していない変更があります"
        description="このまま移動すると、保存していない設定の変更は失われます。"
        cancelLabel="このページにとどまる"
        confirmLabel="変更を捨てて移動する"
        danger
        onConfirm={() => blocker.proceed?.()}
      />
    </div>
  );
}
