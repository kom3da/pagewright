import { Box, Paper, Text, VisuallyHidden } from "@mantine/core";
import { useMediaQuery } from "@mantine/hooks";
import { type Editor, EditorContent, useEditor } from "@tiptap/react";
import { useRef } from "react";
import type { BodyNode } from "../api/types";
import { DESKTOP_QUERY } from "../theme";
import { extensions } from "./extensions";
import { editorContent } from "./normalize";
import { Toolbar } from "./Toolbar";

// One body editor per screen, so fixed ids are fine.
const labelId = "pw-body-label";
const helpId = "pw-body-help";

/** Creates the Tiptap editor once. `onChange` fires on every document change. */
export function useBodyEditor(body: BodyNode | null | undefined, onChange: () => void) {
  const onChangeRef = useRef(onChange);
  onChangeRef.current = onChange;
  return useEditor({
    extensions,
    content: editorContent(body),
    immediatelyRender: true,
    // The toolbar subscribes to the state it needs; the page does not re-render per keystroke.
    shouldRerenderOnTransaction: false,
    editorProps: {
      attributes: {
        class: "pw-prose",
        role: "textbox",
        "aria-multiline": "true",
        "aria-labelledby": labelId,
        "aria-describedby": helpId,
      },
    },
    onUpdate: () => onChangeRef.current(),
  });
}

export function BodyEditor({ editor }: { editor: Editor }) {
  // Phones rarely have a Shift key: the hint is shown only on wider screens, but stays
  // readable for screen readers (and external keyboards) everywhere.
  const desktop = useMediaQuery(DESKTOP_QUERY, true, { getInitialValueInEffect: false });
  const help = "段落の中で改行するときは Shift＋Enter を押します。";
  return (
    <Box>
      <Text id={labelId} fw={700} mb={4}>
        本文
      </Text>
      <Paper withBorder radius="sm" style={{ borderColor: "var(--mantine-color-gray-4)" }}>
        <Toolbar editor={editor} />
        <EditorContent editor={editor} />
      </Paper>
      {desktop ? (
        <Text id={helpId} size="sm" c="dimmed" mt={4}>
          {help}
        </Text>
      ) : (
        <VisuallyHidden id={helpId}>{help}</VisuallyHidden>
      )}
    </Box>
  );
}
