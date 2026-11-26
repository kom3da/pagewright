import {
  ActionIcon,
  Alert,
  Anchor,
  AppShell,
  Badge,
  Button,
  Card,
  Checkbox,
  createTheme,
  EmptyState,
  Input,
  type MantineColorsTuple,
  Menu,
  Modal,
  NativeSelect,
  Notification,
  Paper,
  Radio,
  Table,
  Textarea,
  TextInput,
  Title,
  Tooltip,
} from "@mantine/core";

// Colors. Contrast ratios are against white (#fff); see DESIGN.md for the full table.

/** The single accent: a calm indigo. Shade 6 (#4353c4) is 6.4:1 on white. */
const brand: MantineColorsTuple = [
  "#eef0fb",
  "#dde1f6",
  "#bac2ec",
  "#95a1e2",
  "#7482d8",
  "#5a69d0",
  "#4353c4",
  "#3c4ab8",
  "#3341a3",
  "#283383",
];

/** Neutral greys (no blue tint). Shade 6 (#52525b) 7.7:1 is the "dimmed" text color. */
const gray: MantineColorsTuple = [
  "#f8f8f9",
  "#f1f1f3",
  "#e6e6e9",
  "#d4d4d8",
  "#a1a1aa",
  "#71717a",
  "#52525b",
  "#3f3f46",
  "#27272a",
  "#18181b",
];

// Status colors. Shade 6 is used for filled backgrounds and light-variant text, so every
// shade 6 is at least 4.5:1 on white (Mantine's defaults are too light for that).
const red: MantineColorsTuple = [
  "#fdf0ef",
  "#fadcda",
  "#f4b7b2",
  "#ee8f88",
  "#e76b62",
  "#de4a40",
  "#c42b21",
  "#a8231b",
  "#8c1d16",
  "#701711",
];
const green: MantineColorsTuple = [
  "#edf8f1",
  "#d6efdf",
  "#ade0c0",
  "#80cf9e",
  "#57bf80",
  "#36a865",
  "#1f7a45",
  "#1b6b3c",
  "#165b33",
  "#114a29",
];
const orange: MantineColorsTuple = [
  "#fdf4ea",
  "#f9e4cc",
  "#f2c896",
  "#eaa95d",
  "#e18f30",
  "#c97514",
  "#a34d00",
  "#8c4300",
  "#753800",
  "#5e2d00",
];

/**
 * Desktop layout from Mantine's `sm` breakpoint (768px): sidebar always visible, lists as
 * tables. Below it: drawer sidebar, lists as cards. Use with useMediaQuery(DESKTOP_QUERY).
 */
export const DESKTOP_QUERY = "(min-width: 48em)";

export const theme = createTheme({
  primaryColor: "brand",
  primaryShade: 6,
  colors: { brand, gray, red, green, orange },
  white: "#ffffff",
  black: "#18181b",
  autoContrast: true,
  // No animations for people who asked their OS for less motion (also used by the E2E tests).
  respectReducedMotion: true,
  // Outline only for keyboard focus (:focus-visible). See global.css for headings.
  focusRing: "auto",
  cursorType: "pointer",
  fontFamily:
    '"Hiragino Sans", "Hiragino Kaku Gothic ProN", "BIZ UDPGothic", "Noto Sans JP", "Yu Gothic UI", Meiryo, system-ui, sans-serif',
  fontFamilyMonospace: 'ui-monospace, "SFMono-Regular", Menlo, Consolas, monospace',
  // Body text is 15px. Japanese needs more line-height than Latin text.
  fontSizes: { xs: "0.75rem", sm: "0.875rem", md: "0.9375rem", lg: "1.0625rem", xl: "1.25rem" },
  lineHeights: { xs: "1.5", sm: "1.6", md: "1.7", lg: "1.7", xl: "1.6" },
  // 4px grid.
  spacing: { xs: "0.5rem", sm: "0.75rem", md: "1rem", lg: "1.5rem", xl: "2rem" },
  radius: { xs: "0.25rem", sm: "0.375rem", md: "0.5rem", lg: "0.75rem", xl: "1rem" },
  defaultRadius: "sm",
  shadows: {
    xs: "0 1px 2px rgb(24 24 27 / 0.06)",
    sm: "0 1px 3px rgb(24 24 27 / 0.08), 0 1px 2px rgb(24 24 27 / 0.04)",
    md: "0 4px 12px rgb(24 24 27 / 0.08)",
    lg: "0 8px 24px rgb(24 24 27 / 0.12)",
    xl: "0 16px 40px rgb(24 24 27 / 0.16)",
  },
  headings: {
    fontWeight: "700",
    sizes: {
      h1: { fontSize: "1.5rem", lineHeight: "1.4" },
      h2: { fontSize: "1.125rem", lineHeight: "1.5" },
      h3: { fontSize: "1rem", lineHeight: "1.5" },
      h4: { fontSize: "0.9375rem", lineHeight: "1.5" },
    },
  },
  components: {
    // Buttons: the default is the neutral (secondary) look. A primary button must say
    // variant="filled" explicitly, which keeps "one primary per view" visible in code.
    Button: Button.extend({ defaultProps: { variant: "default", size: "md" } }),
    ActionIcon: ActionIcon.extend({
      defaultProps: { variant: "subtle", color: "gray", size: "lg" },
    }),
    Anchor: Anchor.extend({ defaultProps: { underline: "always" } }),
    // Help and error text under fields: 14px, not Mantine's 13px.
    InputWrapper: Input.Wrapper.extend({
      vars: () => ({
        label: {},
        error: { "--input-error-size": "var(--mantine-font-size-sm)" },
        description: { "--input-description-size": "var(--mantine-font-size-sm)" },
        success: {},
      }),
    }),
    TextInput: TextInput.extend({ defaultProps: { size: "md" } }),
    Textarea: Textarea.extend({ defaultProps: { size: "md", autosize: true, minRows: 2 } }),
    NativeSelect: NativeSelect.extend({ defaultProps: { size: "md" } }),
    Checkbox: Checkbox.extend({ defaultProps: { size: "sm" } }),
    Radio: Radio.extend({ defaultProps: { size: "sm" } }),
    Badge: Badge.extend({
      // 13px bold text on a tinted background; never smaller (Japanese needs the pixels).
      defaultProps: { variant: "light", radius: "sm", size: "lg", tt: "none" },
      // Never cut a status short: no ellipsis, and the badge keeps its full width.
      styles: {
        root: {
          fontSize: "0.8125rem",
          fontWeight: 700,
          letterSpacing: 0,
          maxWidth: "none",
          flexShrink: 0,
          overflow: "visible",
        },
        label: { overflow: "visible", textOverflow: "clip" },
      },
    }),
    Alert: Alert.extend({ defaultProps: { variant: "light", radius: "md" } }),
    Card: Card.extend({ defaultProps: { withBorder: true, radius: "md", padding: "lg" } }),
    Paper: Paper.extend({ defaultProps: { radius: "md" } }),
    Modal: Modal.extend({
      defaultProps: {
        size: "lg",
        radius: "md",
        centered: true,
        overlayProps: { backgroundOpacity: 0.4 },
        // Fade only: a transform would make the modal the containing block of nested
        // confirmations (see components/ui/confirm.tsx).
        transitionProps: { transition: "fade", duration: 150 },
        closeButtonProps: { "aria-label": "閉じる" },
      },
      styles: {
        title: { fontSize: "var(--mantine-font-size-lg)", fontWeight: 700, lineHeight: 1.5 },
        header: { borderBottom: "1px solid var(--mantine-color-gray-2)", marginBottom: "1rem" },
      },
    }),
    Menu: Menu.extend({ defaultProps: { radius: "md", shadow: "md" } }),
    Table: Table.extend({
      defaultProps: { verticalSpacing: "sm", horizontalSpacing: "md", highlightOnHover: true },
    }),
    Notification: Notification.extend({ defaultProps: { radius: "md", withBorder: true } }),
    Tooltip: Tooltip.extend({ defaultProps: { openDelay: 300, withArrow: true } }),
    Title: Title.extend({ defaultProps: { c: "gray.9" } }),
    EmptyState: EmptyState.extend({ defaultProps: { withIndicatorBackground: true } }),
    AppShell: AppShell.extend({ defaultProps: { padding: 0 } }),
  },
});
