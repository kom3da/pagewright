import { Box, Button, Fieldset, Group, Input, Stack, Text } from "@mantine/core";
import { CircleCheck, TriangleAlert } from "lucide-react";
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
 * contrast ratio, so the user can see why a light color cannot be saved. Both inputs share
 * one hint, ratio and error, which Mantine inputs cannot do, hence the plain `.pw-input`
 * with Mantine's label, description and error parts around it.
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
  const ok = ratio !== null && ratio >= MIN_CONTRAST;
  // An error about the contrast says what the warning beside the preview says: show it
  // there instead, once.
  const errorBeside = Boolean(error) && ratio !== null && !ok;
  const describedBy = [ids.hint, ids.ratio, error && !errorBeside && ids.error]
    .filter(Boolean)
    .join(" ");
  const RatioIcon = ok ? CircleCheck : TriangleAlert;

  return (
    <Fieldset legend={label} variant="unstyled" className={classes.root}>
      <Stack gap="xs">
        <Input.Description id={ids.hint} size="sm">
          {hint}
          {value === "" && `いまは標準の色（${defaultColor}）を使っています。`}
        </Input.Description>
        <Group align="flex-end" gap="sm">
          <Box>
            <Input.Label htmlFor={ids.picker} size="sm" className={classes.label}>
              色を選ぶ
            </Input.Label>
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
            <Input.Label htmlFor={ids.text} size="sm" className={classes.label}>
              色コード（例：#1a73e8）
            </Input.Label>
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
              className={`pw-input ${classes.code}`}
            />
          </Box>
          {value !== "" && (
            <Button variant="subtle" onClick={() => onChange("")}>
              標準の色に戻す
            </Button>
          )}
        </Group>
        <Group gap="sm" wrap="nowrap" align="center">
          {shown && (
            <Box aria-hidden="true" className={classes.preview} style={{ backgroundColor: shown }}>
              文字の見え方
            </Box>
          )}
          <Group gap={6} wrap="nowrap" align="flex-start">
            {ratio !== null && (
              <Box component="span" c={ok ? "green.6" : "red.6"} className={classes.icon}>
                <RatioIcon aria-hidden="true" size={18} />
              </Box>
            )}
            {/* Plain words first; the ratio only backs them up. */}
            <Box id={ids.ratio}>
              {ratio === null ? (
                <Text size="sm">色コードを読み取れないため、見え方を表示できません。</Text>
              ) : (
                <>
                  {errorBeside ? (
                    <Text size="sm" fw={700} c="red.7">
                      {error}
                    </Text>
                  ) : (
                    <Text size="sm" fw={700}>
                      {ok
                        ? "この色の上の白い文字は、はっきり読めます。"
                        : "白い文字が読みにくい色です。もう少し濃い色にしてください。"}
                    </Text>
                  )}
                  <Text size="sm" c="dimmed">
                    白い文字とのコントラスト比 {formatRatio(ratio)}（基準は 4.5:1 以上）
                  </Text>
                </>
              )}
            </Box>
          </Group>
        </Group>
        {error && !errorBeside && (
          <Input.Error id={ids.error} size="sm">
            {error}
          </Input.Error>
        )}
      </Stack>
    </Fieldset>
  );
}
