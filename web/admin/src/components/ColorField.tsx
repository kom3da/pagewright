import { Box, Button, Fieldset, Group, Stack, Text } from "@mantine/core";
import { useId } from "react";
import { contrast, formatRatio, isHexColor, MIN_CONTRAST } from "../lib/color";
import classes from "./ColorField.module.css";

type Props = {
  label: string;
  hint: string;
  /** "#RRGGBB", or "" for the theme's default. */
  value: string;
  defaultColor: string;
  error?: string;
  onChange: (value: string) => void;
};

/** Problem with a color setting that must block saving, or undefined. */
export function colorProblem(label: string, value: string) {
  if (!value) return undefined;
  if (!isHexColor(value)) {
    return `${label}は「#1a73e8」のように、# のあとに英数字6桁で入力してください。`;
  }
  const r = contrast(value);
  if (r < MIN_CONTRAST) {
    return `${label}が明るすぎて、白い文字との差が足りません（コントラスト比 ${formatRatio(r)}）。読みやすさの基準（JIS X 8341-3）では 4.5:1 以上が必要です。もっと濃い色を選んでください。`;
  }
  return undefined;
}

/**
 * A color picker plus a hex text box, with a preview of white text on the color and its
 * contrast ratio, so the user can see why a light color cannot be saved.
 */
export function ColorField({ label, hint, value, defaultColor, error, onChange }: Props) {
  const id = useId();
  const ids = {
    picker: `${id}-picker`,
    text: `${id}-text`,
    hint: `${id}-hint`,
    ratio: `${id}-ratio`,
    error: `${id}-error`,
  };
  const valid = isHexColor(value);
  const shown = value === "" ? defaultColor : valid ? value.toLowerCase() : null;
  const ratio = shown ? contrast(shown) : null;
  const describedBy = [ids.hint, ids.ratio, error && ids.error].filter(Boolean).join(" ");

  return (
    <Fieldset legend={label} variant="unstyled">
      <Stack gap="sm">
        <Text id={ids.hint} size="sm" c="dimmed">
          {hint}
          {value === "" && `いまは標準の色（${defaultColor}）を使っています。`}
        </Text>
        <Group align="flex-end" gap="sm">
          <Box>
            <label htmlFor={ids.picker} className="pw-label">
              色を選ぶ
            </label>
            <input
              id={ids.picker}
              type="color"
              value={shown ?? defaultColor}
              onChange={(e) => onChange(e.target.value)}
              aria-describedby={describedBy}
              className={classes.picker}
            />
          </Box>
          <Box w="13rem">
            <label htmlFor={ids.text} className="pw-label">
              色コード（例：#1a73e8）
            </label>
            <input
              id={ids.text}
              type="text"
              value={value}
              placeholder={defaultColor}
              maxLength={7}
              autoComplete="off"
              autoCapitalize="none"
              spellCheck={false}
              onChange={(e) => onChange(e.target.value.trim())}
              aria-describedby={describedBy}
              aria-invalid={error ? true : undefined}
              className="pw-input"
              style={{ fontFamily: "var(--mantine-font-family-monospace)" }}
            />
          </Box>
          {value !== "" && <Button onClick={() => onChange("")}>標準の色に戻す</Button>}
        </Group>
        <Group gap="sm">
          {shown && (
            <Box aria-hidden="true" className={classes.preview} style={{ backgroundColor: shown }}>
              白い文字の見え方
            </Box>
          )}
          <Text id={ids.ratio} size="sm">
            {ratio === null
              ? "色コードを読み取れないため、見え方を表示できません。"
              : `白い文字とのコントラスト比：${formatRatio(ratio)}（${
                  ratio >= MIN_CONTRAST ? "基準の 4.5:1 以上です" : "基準の 4.5:1 に足りません"
                }）`}
          </Text>
        </Group>
        {error && (
          <Text id={ids.error} size="sm" fw={700} c="red.7">
            {error}
          </Text>
        )}
      </Stack>
    </Fieldset>
  );
}
