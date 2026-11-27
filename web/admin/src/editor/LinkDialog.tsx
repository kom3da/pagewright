import { Button, Checkbox, Group, TextInput } from "@mantine/core";
import type { Editor } from "@tiptap/react";
import { type FormEvent, useState } from "react";
import { Dialog } from "../components/ui/Dialog";
import { Form } from "../components/ui/Form";

type Props = { editor: Editor; open: boolean; onClose: () => void };

export function linkProblem(href: string): string | null {
  if (!href) return "リンク先のURLを入力してください。";
  if (/^(https?:\/\/[^\s]+|mailto:[^\s]+|tel:[0-9+\-()]+|\/[^\s]*|#[^\s]*)$/i.test(href))
    return null;
  return "「https://」で始まるURL、「/」で始まるサイト内のパス、電話（tel:）かメール（mailto:）のリンクを入力してください。";
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
    <Form onSubmit={onSubmit} gap="md">
      {needsText && (
        <TextInput
          label="表示する文字"
          required
          value={text}
          onChange={(e) => setText(e.target.value)}
          error={errors.text}
        />
      )}
      <TextInput
        label="リンク先のURL"
        description="例：https://example.jp/ 、サイト内なら /company/ 、電話なら tel:03-1234-5678 、メールなら mailto:info@example.jp"
        required
        type="url"
        inputMode="url"
        value={href}
        onChange={(e) => setHref(e.target.value)}
        error={errors.href}
      />
      <Checkbox
        label="新しいタブで開く"
        checked={newTab}
        onChange={(e) => setNewTab(e.currentTarget.checked)}
      />
      <Group justify="flex-end" gap="sm">
        {isLink && (
          <Button variant="subtle" color="red" onClick={remove} mr="auto">
            リンクを外す
          </Button>
        )}
        <Button onClick={onClose}>キャンセル</Button>
        <Button type="submit" variant="filled">
          {isLink ? "変更する" : "リンクにする"}
        </Button>
      </Group>
    </Form>
  );
}
