import {
  ActionIcon,
  Button,
  Divider,
  Menu,
  NativeSelect,
  Tooltip,
  VisuallyHidden,
} from "@mantine/core";
import { useMediaQuery } from "@mantine/hooks";
import { type Editor, useEditorState } from "@tiptap/react";
import {
  Bold,
  Check,
  ChevronDown,
  ImagePlus,
  Italic,
  Link2,
  List,
  ListOrdered,
  type LucideIcon,
  Mail,
  Minus,
  Quote,
  Redo2,
  Table,
  Undo2,
} from "lucide-react";
import {
  type KeyboardEvent,
  type ReactNode,
  useId,
  useLayoutEffect,
  useRef,
  useState,
} from "react";
import { DESKTOP_QUERY } from "../theme";
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
  /** Says how to make a disabled button usable (tooltip and description). */
  disabledHint?: string;
  onClick: () => void;
};

/**
 * Toolbar button: an icon button with a tooltip, or a button with visible text (and an
 * optional icon). Uses aria-disabled (with Mantine's data-disabled look) instead of disabled
 * so that it stays reachable with the arrow keys (WAI-ARIA toolbar pattern).
 */
function ToolButton({
  label,
  icon: Icon,
  text,
  pressed,
  disabled,
  disabledHint,
  onClick,
}: ButtonProps) {
  const hintId = useId();
  const hint = disabled ? disabledHint : undefined;
  const common = {
    variant: pressed ? "light" : "subtle",
    color: pressed ? "brand" : "gray",
    "aria-pressed": pressed,
    "aria-disabled": disabled || undefined,
    "data-disabled": disabled || undefined,
    "aria-describedby": hint ? hintId : undefined,
    onClick: () => {
      if (!disabled) onClick();
    },
    className: classes.button,
  } as const;
  if (text) {
    // The visible text starts the accessible name (「画像」→「画像を入れる」).
    return (
      <>
        <Tooltip label={hint} disabled={!hint}>
          <Button
            {...common}
            size="sm"
            px={6}
            aria-label={text === label ? undefined : label}
            leftSection={Icon && <Icon aria-hidden="true" size={18} />}
          >
            {text}
          </Button>
        </Tooltip>
        {hint && <VisuallyHidden id={hintId}>{hint}</VisuallyHidden>}
      </>
    );
  }
  return (
    <Tooltip label={label}>
      <ActionIcon {...common} size="2.25rem" aria-label={label}>
        {Icon && <Icon aria-hidden="true" size={18} />}
      </ActionIcon>
    </Tooltip>
  );
}

function Separator() {
  return <Divider orientation="vertical" aria-hidden="true" mx={2} my={6} />;
}

/** Arrow-key navigation between toolbar controls with a single Tab stop. */
function useRovingFocus() {
  const ref = useRef<HTMLDivElement>(null);
  const controls = () =>
    Array.from(ref.current?.querySelectorAll<HTMLElement>("button, select") ?? []);

  useLayoutEffect(() => {
    const all = controls();
    for (const b of all) {
      if (b.dataset.roving) continue;
      b.dataset.roving = "1";
      b.tabIndex = -1;
    }
    if (all.length > 0 && !all.some((b) => b.tabIndex === 0)) all[0].tabIndex = 0;
  });

  const onKeyDown = (e: KeyboardEvent) => {
    const all = controls();
    const i = all.indexOf(document.activeElement as HTMLElement);
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

type Block = "paragraph" | "h2" | "h3" | "h4";
const blockOptions: { value: Block; label: string }[] = [
  { value: "paragraph", label: "本文" },
  { value: "h2", label: "大見出し" },
  { value: "h3", label: "中見出し" },
  { value: "h4", label: "小見出し" },
];

type Item = ButtonProps & {
  key: string;
  /** Shown in the toolbar on phones too; the rest go behind 「ほか」. */
  phone?: boolean;
  /** Starts a group (a separator before it in the toolbar). */
  group?: boolean;
};

const hintId = "pw-toolbar-hint";

export function Toolbar({ editor }: { editor: Editor }) {
  const roving = useRovingFocus();
  const desktop = useMediaQuery(DESKTOP_QUERY, true, { getInitialValueInEffect: false });
  const [dialog, setDialog] = useState<"link" | "ruby" | "image" | null>(null);

  const s = useEditorState({
    editor,
    selector: ({ editor: e }) => ({
      block: (e.isActive("heading", { level: 2 })
        ? "h2"
        : e.isActive("heading", { level: 3 })
          ? "h3"
          : e.isActive("heading", { level: 4 })
            ? "h4"
            : "paragraph") as Block,
      bold: e.isActive("bold"),
      italic: e.isActive("italic"),
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
  const setBlock = (b: Block) => {
    if (b === "paragraph") chain().setParagraph().run();
    else
      chain()
        .setHeading({ level: Number(b.slice(1)) as 2 | 3 | 4 })
        .run();
  };

  const items: Item[] = [
    {
      key: "bold",
      label: "太字",
      icon: Bold,
      pressed: s.bold,
      phone: true,
      onClick: () => chain().toggleBold().run(),
    },
    {
      key: "italic",
      label: "斜体",
      icon: Italic,
      pressed: s.italic,
      onClick: () => chain().toggleItalic().run(),
    },
    {
      key: "link",
      label: "リンク",
      icon: Link2,
      pressed: s.link,
      phone: true,
      onClick: () => setDialog("link"),
    },
    {
      key: "ruby",
      label: "ルビ（ふりがな）",
      text: "ルビ",
      pressed: s.ruby,
      disabled: s.emptySelection && !s.ruby,
      disabledHint: "文字を選んでから押します",
      onClick: () => {
        if (s.ruby) editor.commands.extendMarkRange("ruby");
        setDialog("ruby");
      },
    },
    {
      key: "bulletList",
      label: "箇条書き",
      icon: List,
      pressed: s.bulletList,
      group: true,
      onClick: () => chain().toggleBulletList().run(),
    },
    {
      key: "orderedList",
      label: "番号付きリスト",
      icon: ListOrdered,
      pressed: s.orderedList,
      onClick: () => chain().toggleOrderedList().run(),
    },
    {
      key: "blockquote",
      label: "引用",
      icon: Quote,
      pressed: s.blockquote,
      onClick: () => chain().toggleBlockquote().run(),
    },
    {
      key: "hr",
      label: "区切り線",
      icon: Minus,
      onClick: () => chain().setHorizontalRule().run(),
    },
    {
      key: "image",
      label: "画像を入れる",
      text: "画像",
      icon: ImagePlus,
      phone: true,
      group: true,
      onClick: () => setDialog("image"),
    },
    {
      key: "table",
      label: "表を入れる",
      text: "表",
      icon: Table,
      disabled: s.table,
      onClick: () => chain().insertTable({ rows: 3, cols: 3, withHeaderRow: true }).run(),
    },
    {
      key: "contactForm",
      label: "お問い合わせフォームを入れる",
      text: "お問い合わせフォーム",
      icon: Mail,
      onClick: () => chain().insertContactForm().run(),
    },
  ];

  const history: Item[] = [
    {
      key: "undo",
      label: "元に戻す",
      icon: Undo2,
      disabled: !s.canUndo,
      group: true,
      onClick: () => chain().undo().run(),
    },
    {
      key: "redo",
      label: "やり直す",
      icon: Redo2,
      disabled: !s.canRedo,
      onClick: () => chain().redo().run(),
    },
  ];

  const tableItems: Item[] = s.table
    ? [
        { key: "addRow", label: "行を追加", onClick: () => chain().addRowAfter().run() },
        { key: "addCol", label: "列を追加", onClick: () => chain().addColumnAfter().run() },
        { key: "delRow", label: "行を削除", onClick: () => chain().deleteRow().run() },
        { key: "delCol", label: "列を削除", onClick: () => chain().deleteColumn().run() },
        { key: "delTable", label: "表を削除", onClick: () => chain().deleteTable().run() },
      ].map((t, i) => ({ ...t, text: t.label, group: i === 0 }))
    : [];

  const renderButtons = (list: Item[]) =>
    list.map(({ key, group, phone: _, ...props }) => (
      <span key={key} className={classes.item}>
        {group && <Separator />}
        <ToolButton {...props} />
      </span>
    ));

  // On phones only the everyday items stay in the bar; the rest are behind 「ほか」.
  const shown = desktop ? [...items, ...tableItems, ...history] : items.filter((i) => i.phone);
  const more = desktop ? [] : [...items.filter((i) => !i.phone), ...history];

  return (
    <>
      <div
        ref={roving.ref}
        role="toolbar"
        aria-label="本文の書式"
        aria-describedby={hintId}
        onKeyDown={roving.onKeyDown}
        className={classes.toolbar}
      >
        <VisuallyHidden id={hintId}>左右の矢印キーで項目を移動できます。</VisuallyHidden>
        <NativeSelect
          aria-label="段落の種類"
          size="sm"
          value={s.block}
          onChange={(e) => setBlock(e.currentTarget.value as Block)}
          className={classes.select}
        >
          {blockOptions.map((o) => (
            <option key={o.value} value={o.value}>
              {o.label}
            </option>
          ))}
        </NativeSelect>
        <Separator />
        {renderButtons(shown)}
        {more.length > 0 && (
          <Menu position="bottom-end" withInitialFocusPlaceholder={false}>
            <Menu.Target>
              <Button
                variant="subtle"
                color="gray"
                size="sm"
                px="xs"
                className={classes.button}
                rightSection={<ChevronDown aria-hidden="true" size={16} />}
                aria-label="ほかの書式"
              >
                ほか
              </Button>
            </Menu.Target>
            <Menu.Dropdown>
              {more.map(({ key, ...i }) => (
                <MoreItem key={key} {...i} />
              ))}
            </Menu.Dropdown>
          </Menu>
        )}
        {!desktop && renderButtons(tableItems)}
      </div>
      <LinkDialog editor={editor} open={dialog === "link"} onClose={closeDialog} />
      <RubyDialog editor={editor} open={dialog === "ruby"} onClose={closeDialog} />
      <MediaDialog editor={editor} open={dialog === "image"} onClose={closeDialog} />
    </>
  );
}

/** An item of the phone's 「ほか」 menu. Pressed formats get a check mark. */
function MoreItem({
  label,
  icon: Icon,
  pressed,
  disabled,
  disabledHint,
  onClick,
}: ButtonProps): ReactNode {
  return (
    <Menu.Item
      leftSection={Icon ? <Icon aria-hidden="true" size={16} /> : <span aria-hidden="true" />}
      rightSection={pressed ? <Check aria-hidden="true" size={16} /> : undefined}
      disabled={disabled}
      onClick={onClick}
    >
      {label}
      {disabled && disabledHint && `（${disabledHint}）`}
      {pressed && <VisuallyHidden>（適用中）</VisuallyHidden>}
    </Menu.Item>
  );
}
