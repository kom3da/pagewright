import { ActionIcon, Button, Divider, Tooltip } from "@mantine/core";
import { type Editor, useEditorState } from "@tiptap/react";
import {
  Bold,
  Code,
  Heading2,
  Heading3,
  Heading4,
  ImagePlus,
  Italic,
  Link2,
  List,
  ListOrdered,
  type LucideIcon,
  Mail,
  Minus,
  Pilcrow,
  Quote,
  Redo2,
  Table,
  Undo2,
} from "lucide-react";
import { type KeyboardEvent, type ReactNode, useLayoutEffect, useRef, useState } from "react";
import { LinkDialog } from "./LinkDialog";
import { MediaDialog } from "./MediaDialog";
import { RubyDialog } from "./RubyDialog";
import classes from "./Toolbar.module.css";

type ButtonProps = {
  label: string;
  icon?: LucideIcon;
  text?: string;
  pressed?: boolean;
  disabled?: boolean;
  onClick: () => void;
};

/**
 * Toolbar button: an icon button with a tooltip, or a small text button. Uses aria-disabled
 * (with Mantine's data-disabled look) instead of disabled so that it stays reachable with
 * the arrow keys (WAI-ARIA toolbar pattern).
 */
function ToolButton({ label, icon: Icon, text, pressed, disabled, onClick }: ButtonProps) {
  const common = {
    variant: pressed ? "light" : "subtle",
    color: pressed ? "brand" : "gray",
    "aria-pressed": pressed,
    "aria-disabled": disabled || undefined,
    "data-disabled": disabled || undefined,
    onClick: () => {
      if (!disabled) onClick();
    },
    className: classes.button,
  } as const;
  const button = Icon ? (
    <ActionIcon {...common} size="2.25rem" aria-label={label}>
      <Icon aria-hidden="true" size={18} />
    </ActionIcon>
  ) : (
    <Button {...common} size="sm" px="xs">
      {text}
    </Button>
  );
  return text === label ? button : <Tooltip label={label}>{button}</Tooltip>;
}

function Separator() {
  return <Divider orientation="vertical" aria-hidden="true" mx={4} my={6} />;
}

/** Arrow-key navigation between toolbar buttons with a single Tab stop. */
function useRovingFocus() {
  const ref = useRef<HTMLDivElement>(null);
  const buttons = () => Array.from(ref.current?.querySelectorAll("button") ?? []);

  useLayoutEffect(() => {
    const all = buttons();
    for (const b of all) {
      if (b.dataset.roving) continue;
      b.dataset.roving = "1";
      b.tabIndex = -1;
    }
    if (all.length > 0 && !all.some((b) => b.tabIndex === 0)) all[0].tabIndex = 0;
  });

  const onKeyDown = (e: KeyboardEvent) => {
    const all = buttons();
    const i = all.indexOf(document.activeElement as HTMLButtonElement);
    if (i < 0) return;
    const next = {
      ArrowRight: (i + 1) % all.length,
      ArrowLeft: (i - 1 + all.length) % all.length,
      Home: 0,
      End: all.length - 1,
    }[e.key];
    if (next === undefined) return;
    e.preventDefault();
    all[i].tabIndex = -1;
    all[next].tabIndex = 0;
    all[next].focus();
  };

  return { ref, onKeyDown };
}

export function Toolbar({ editor }: { editor: Editor }) {
  const roving = useRovingFocus();
  const [dialog, setDialog] = useState<"link" | "ruby" | "image" | null>(null);

  const s = useEditorState({
    editor,
    selector: ({ editor: e }) => ({
      paragraph: e.isActive("paragraph"),
      h2: e.isActive("heading", { level: 2 }),
      h3: e.isActive("heading", { level: 3 }),
      h4: e.isActive("heading", { level: 4 }),
      bold: e.isActive("bold"),
      italic: e.isActive("italic"),
      code: e.isActive("code"),
      link: e.isActive("link"),
      ruby: e.isActive("ruby"),
      bulletList: e.isActive("bulletList"),
      orderedList: e.isActive("orderedList"),
      blockquote: e.isActive("blockquote"),
      table: e.isActive("table"),
      emptySelection: e.state.selection.empty,
      canUndo: e.can().undo(),
      canRedo: e.can().redo(),
    }),
  });

  const chain = () => editor.chain().focus();
  const closeDialog = () => setDialog(null);

  let tableTools: ReactNode = null;
  if (s.table) {
    tableTools = (
      <>
        <Separator />
        <ToolButton text="行を追加" label="行を追加" onClick={() => chain().addRowAfter().run()} />
        <ToolButton
          text="列を追加"
          label="列を追加"
          onClick={() => chain().addColumnAfter().run()}
        />
        <ToolButton text="行を削除" label="行を削除" onClick={() => chain().deleteRow().run()} />
        <ToolButton text="列を削除" label="列を削除" onClick={() => chain().deleteColumn().run()} />
        <ToolButton text="表を削除" label="表を削除" onClick={() => chain().deleteTable().run()} />
      </>
    );
  }

  return (
    <>
      <div
        ref={roving.ref}
        role="toolbar"
        aria-label="本文の書式"
        onKeyDown={roving.onKeyDown}
        className={classes.toolbar}
      >
        <ToolButton
          label="本文（段落）"
          icon={Pilcrow}
          pressed={s.paragraph}
          onClick={() => chain().setParagraph().run()}
        />
        <ToolButton
          label="大見出し"
          icon={Heading2}
          pressed={s.h2}
          onClick={() => chain().toggleHeading({ level: 2 }).run()}
        />
        <ToolButton
          label="中見出し"
          icon={Heading3}
          pressed={s.h3}
          onClick={() => chain().toggleHeading({ level: 3 }).run()}
        />
        <ToolButton
          label="小見出し"
          icon={Heading4}
          pressed={s.h4}
          onClick={() => chain().toggleHeading({ level: 4 }).run()}
        />
        <Separator />
        <ToolButton
          label="太字"
          icon={Bold}
          pressed={s.bold}
          onClick={() => chain().toggleBold().run()}
        />
        <ToolButton
          label="斜体"
          icon={Italic}
          pressed={s.italic}
          onClick={() => chain().toggleItalic().run()}
        />
        <ToolButton
          label="コード"
          icon={Code}
          pressed={s.code}
          onClick={() => chain().toggleCode().run()}
        />
        <ToolButton
          label="リンク"
          icon={Link2}
          pressed={s.link}
          onClick={() => setDialog("link")}
        />
        <ToolButton
          label="ルビ（ふりがな）"
          text="ルビ"
          pressed={s.ruby}
          disabled={s.emptySelection && !s.ruby}
          onClick={() => {
            if (s.ruby) editor.commands.extendMarkRange("ruby");
            setDialog("ruby");
          }}
        />
        <Separator />
        <ToolButton
          label="箇条書き"
          icon={List}
          pressed={s.bulletList}
          onClick={() => chain().toggleBulletList().run()}
        />
        <ToolButton
          label="番号付きリスト"
          icon={ListOrdered}
          pressed={s.orderedList}
          onClick={() => chain().toggleOrderedList().run()}
        />
        <ToolButton
          label="引用"
          icon={Quote}
          pressed={s.blockquote}
          onClick={() => chain().toggleBlockquote().run()}
        />
        <ToolButton
          label="区切り線"
          icon={Minus}
          onClick={() => chain().setHorizontalRule().run()}
        />
        <ToolButton label="画像を入れる" icon={ImagePlus} onClick={() => setDialog("image")} />
        <ToolButton
          label="表を入れる"
          icon={Table}
          disabled={s.table}
          onClick={() => chain().insertTable({ rows: 3, cols: 3, withHeaderRow: true }).run()}
        />
        <ToolButton
          label="お問い合わせフォームを入れる"
          icon={Mail}
          onClick={() => chain().insertContactForm().run()}
        />
        {tableTools}
        <Separator />
        <ToolButton
          label="元に戻す"
          icon={Undo2}
          disabled={!s.canUndo}
          onClick={() => chain().undo().run()}
        />
        <ToolButton
          label="やり直す"
          icon={Redo2}
          disabled={!s.canRedo}
          onClick={() => chain().redo().run()}
        />
      </div>
      <LinkDialog editor={editor} open={dialog === "link"} onClose={closeDialog} />
      <RubyDialog editor={editor} open={dialog === "ruby"} onClose={closeDialog} />
      <MediaDialog editor={editor} open={dialog === "image"} onClose={closeDialog} />
    </>
  );
}
