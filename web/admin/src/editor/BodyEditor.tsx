import { Box, Paper, Text } from "@mantine/core";
import { type Editor, EditorContent, useEditor } from "@tiptap/react";
import { useRef } from "react";
import type { BodyNode } from "../api/types";
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
  return (
    <Box>
      <Text id={labelId} fw={700}>
        本文
      </Text>
      <Text id={helpId} size="sm" c="dimmed" mb={4}>
        Shift＋Enterで段落の中で改行します。ツールバーの中は左右の矢印キーで移動できます。
      </Text>
      <Paper withBorder radius="sm" style={{ borderColor: "var(--mantine-color-gray-4)" }}>
        <Toolbar editor={editor} />
        <EditorContent editor={editor} />
      </Paper>
    </Box>
  );
}
