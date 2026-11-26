// Color checks for the site settings, matching internal/site/settings.go.

/** WCAG AA ratio for normal text (JIS X 8341-3). */
export const MIN_CONTRAST = 4.5;

/** The standard theme's colors, used when a setting is empty (themes/corporate). */
export const DEFAULT_PRIMARY_COLOR = "#0a4f8f";
export const DEFAULT_FOOTER_COLOR = "#1d2733";

export const isHexColor = (v: string) => /^#[0-9a-fA-F]{6}$/.test(v);

function luminance(hex: string) {
  const ch = (i: number) => {
    const c = Number.parseInt(hex.slice(i, i + 2), 16) / 255;
    return c <= 0.03928 ? c / 12.92 : ((c + 0.055) / 1.055) ** 2.4;
  };
  return 0.2126 * ch(1) + 0.7152 * ch(3) + 0.0722 * ch(5);
}

/** WCAG contrast ratio of two #RRGGBB colors. */
export function contrast(a: string, b = "#ffffff") {
  const [hi, lo] = [luminance(a), luminance(b)].sort((x, y) => y - x);
  return (hi + 0.05) / (lo + 0.05);
}

/** Ratio for display, rounded down so that 4.49 never shows as 4.5. */
export const formatRatio = (r: number) => `${(Math.floor(r * 10) / 10).toFixed(1)}:1`;
