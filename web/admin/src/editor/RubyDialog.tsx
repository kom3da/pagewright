import type { Editor } from "@tiptap/react";
import { type FormEvent, useState } from "react";
import { Button } from "../components/ui/Button";
import { Dialog } from "../components/ui/Dialog";
import { TextField } from "../components/ui/Field";

type Props = { editor: Editor; open: boolean; onClose: () => void };

export function RubyDialog({ editor, open, onClose }: Props) {
  return (
    <Dialog
      open={open}
      onOpenChange={(o) => !o && onClose()}
      title="ルビ（ふりがな）"
      description="選んだ文字の上に、小さく読みがなを表示します。"
      onCloseFocus={() => editor.commands.focus()}
    >
      {open && <RubyForm editor={editor} onClose={onClose} />}
    </Dialog>
  );
}

function RubyForm({ editor, onClose }: { editor: Editor; onClose: () => void }) {
  // The toolbar has already extended the selection over an existing ruby.
  const isRuby = editor.isActive("ruby");
  const { from, to } = editor.state.selection;
  const base = editor.state.doc.textBetween(from, to);
  const [rt, setRt] = useState<string>(editor.getAttributes("ruby").rt ?? "");
  const [error, setError] = useState<string>();

  const onSubmit = (e: FormEvent) => {
    e.preventDefault();
    e.stopPropagation();
    const value = rt.trim();
    if (!value) {
      setError("ふりがなを入力してください。");
      return;
    }
    editor.chain().focus().setRuby(value).run();
    onClose();
  };

  const remove = () => {
    editor.chain().focus().unsetRuby().run();
    onClose();
  };

  return (
    <form noValidate onSubmit={onSubmit} className="space-y-4">
      <p>
        対象の文字：<strong className="text-lg">{base}</strong>
      </p>
      <TextField
        label="ふりがな"
        hint="例：とうきょう"
        required
        value={rt}
        onChange={(e) => setRt(e.target.value)}
        error={error}
      />
      <div className="flex flex-wrap justify-end gap-3">
        {isRuby && (
          <Button variant="danger" onClick={remove} className="mr-auto">
            ルビを外す
          </Button>
        )}
        <Button onClick={onClose}>キャンセル</Button>
        <Button type="submit" variant="primary">
          {isRuby ? "変更する" : "ルビをつける"}
        </Button>
      </div>
    </form>
  );
}
