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
    <div className="space-y-1">
      <p id={labelId} className="font-semibold">
        本文
      </p>
      <p id={helpId} className="text-sm text-slate-700">
        Shift＋Enterで段落の中で改行します。ツールバーの中は左右の矢印キーで移動できます。
      </p>
      <div className="rounded-md border border-slate-500 bg-white">
        <Toolbar editor={editor} />
        <EditorContent editor={editor} />
      </div>
    </div>
  );
}
