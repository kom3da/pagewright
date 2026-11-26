import type { Editor } from "@tiptap/react";
import { type FormEvent, useState } from "react";
import { Button } from "../components/ui/Button";
import { Dialog } from "../components/ui/Dialog";
import { TextField } from "../components/ui/Field";

type Props = { editor: Editor; open: boolean; onClose: () => void };

export function linkProblem(href: string): string | null {
  if (!href) return "リンク先のURLを入力してください。";
  if (/^(https?:\/\/[^\s]+|mailto:[^\s]+|tel:[0-9+\-()]+|\/[^\s]*|#[^\s]*)$/i.test(href))
    return null;
  return "「https://」で始まるURL、または「/」で始まるサイト内のパスを入力してください。";
}

export function LinkDialog({ editor, open, onClose }: Props) {
  return (
    <Dialog
      open={open}
      onOpenChange={(o) => !o && onClose()}
      title="リンク"
      onCloseFocus={() => editor.commands.focus()}
    >
      {open && <LinkForm editor={editor} onClose={onClose} />}
    </Dialog>
  );
}

function LinkForm({ editor, onClose }: { editor: Editor; onClose: () => void }) {
  const current = editor.getAttributes("link");
  const isLink = editor.isActive("link");
  const needsText = editor.state.selection.empty && !isLink;
  const [href, setHref] = useState<string>(current.href ?? "");
  const [text, setText] = useState("");
  const [newTab, setNewTab] = useState(current.target === "_blank");
  const [errors, setErrors] = useState<{ href?: string; text?: string }>({});

  const onSubmit = (e: FormEvent) => {
    e.preventDefault();
    e.stopPropagation();
    const value = href.trim();
    const next = {
      href: linkProblem(value) ?? undefined,
      text: needsText && !text.trim() ? "表示する文字を入力してください。" : undefined,
    };
    setErrors(next);
    if (next.href || next.text) return;
    const attrs = { href: value, target: newTab ? "_blank" : null };
    if (needsText) {
      editor
        .chain()
        .focus()
        .insertContent({ type: "text", text: text.trim(), marks: [{ type: "link", attrs }] })
        .run();
    } else {
      editor.chain().focus().extendMarkRange("link").setLink(attrs).run();
    }
    onClose();
  };

  const remove = () => {
    editor.chain().focus().extendMarkRange("link").unsetLink().run();
    onClose();
  };

  return (
    <form noValidate onSubmit={onSubmit} className="space-y-4">
      {needsText && (
        <TextField
          label="表示する文字"
          required
          value={text}
          onChange={(e) => setText(e.target.value)}
          error={errors.text}
        />
      )}
      <TextField
        label="リンク先のURL"
        hint="例：https://example.jp/ 、サイト内なら /company/"
        required
        type="url"
        inputMode="url"
        value={href}
        onChange={(e) => setHref(e.target.value)}
        error={errors.href}
      />
      <label className="flex items-center gap-2">
        <input
          type="checkbox"
          checked={newTab}
          onChange={(e) => setNewTab(e.target.checked)}
          className="size-5"
        />
        新しいタブで開く
      </label>
      <div className="flex flex-wrap justify-end gap-3">
        {isLink && (
          <Button variant="danger" onClick={remove} className="mr-auto">
            リンクを外す
          </Button>
        )}
        <Button onClick={onClose}>キャンセル</Button>
        <Button type="submit" variant="primary">
          {isLink ? "変更する" : "リンクにする"}
        </Button>
      </div>
    </form>
  );
}
