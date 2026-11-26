import { type Extensions, Mark, Node } from "@tiptap/core";
import { TableKit } from "@tiptap/extension-table";
import StarterKit from "@tiptap/starter-kit";
import { mediaFileUrl } from "../api/client";

// Editor schema for schema/body-v1.md. Node and mark names follow Tiptap's standard
// extensions; Ruby, Image, Embed and RawHtml are defined here.

declare module "@tiptap/core" {
  interface Commands<ReturnType> {
    ruby: {
      setRuby: (rt: string) => ReturnType;
      unsetRuby: () => ReturnType;
    };
    pwImage: {
      insertImage: (attrs: {
        src: string;
        alt: string;
        mediaId: number | null;
        title?: string | null;
      }) => ReturnType;
    };
  }
}

/** Ruby (furigana): `<ruby>東京<rp>(</rp><rt>とうきょう</rt><rp>)</rp></ruby>`. */
export const Ruby = Mark.create({
  name: "ruby",
  inclusive: false,

  addAttributes() {
    return {
      rt: {
        default: "",
        parseHTML: (el) => el.querySelector("rt")?.textContent ?? "",
        renderHTML: () => ({}),
      },
    };
  },

  parseHTML() {
    return [
      {
        tag: "ruby",
        // Only the base text is content; rt/rp hold the reading, which lives in the attribute.
        contentElement: (el) => {
          const base = el.cloneNode(true) as HTMLElement;
          for (const n of base.querySelectorAll("rt, rp")) n.remove();
          return base;
        },
      },
    ];
  },

  renderHTML({ mark }) {
    return [
      "ruby",
      {},
      ["span", 0],
      ["rp", { contenteditable: "false" }, "("],
      ["rt", { contenteditable: "false" }, String(mark.attrs.rt ?? "")],
      ["rp", { contenteditable: "false" }, ")"],
    ];
  },

  addCommands() {
    return {
      setRuby:
        (rt) =>
        ({ commands }) =>
          commands.setMark(this.name, { rt }),
      unsetRuby:
        () =>
        ({ commands }) =>
          commands.unsetMark(this.name, { extendEmptyMarkRange: true }),
    };
  },
});

const toMediaId = (v: string | null) => (v && /^\d+$/.test(v) ? Number(v) : null);

/**
 * Image block. `src` is the public-site path stored in the body; inside the admin the
 * picture is shown from /api/media/{mediaId}/file when mediaId is set.
 */
export const Image = Node.create({
  name: "image",
  group: "block",
  atom: true,
  draggable: true,
  selectable: true,

  addAttributes() {
    return {
      src: {
        default: null,
        parseHTML: (el) => el.getAttribute("data-src") ?? el.getAttribute("src"),
        renderHTML: (attrs) => ({
          "data-src": attrs.src,
          src: attrs.mediaId != null ? mediaFileUrl(attrs.mediaId) : attrs.src,
        }),
      },
      alt: {
        default: "",
        parseHTML: (el) => el.getAttribute("alt") ?? "",
        renderHTML: (attrs) => ({ alt: attrs.alt ?? "" }),
      },
      mediaId: {
        default: null,
        parseHTML: (el) => toMediaId(el.getAttribute("data-media-id")),
        renderHTML: (attrs) =>
          attrs.mediaId != null ? { "data-media-id": String(attrs.mediaId) } : {},
      },
      title: { default: null },
    };
  },

  parseHTML() {
    return [{ tag: "img[src]" }];
  },

  renderHTML({ HTMLAttributes }) {
    return ["img", HTMLAttributes];
  },

  addCommands() {
    return {
      insertImage:
        (attrs) =>
        ({ commands }) =>
          commands.insertContent({ type: this.name, attrs }),
    };
  },
});

const embedProviders: Record<string, string> = {
  youtube: "YouTube",
  googlemap: "Googleマップ",
};

/** Embedded YouTube / Google Maps. Kept as-is and shown as a read-only block. */
export const Embed = Node.create({
  name: "embed",
  group: "block",
  atom: true,
  selectable: true,

  addAttributes() {
    return {
      provider: {
        default: null,
        parseHTML: (el) => el.getAttribute("data-provider"),
        renderHTML: (attrs) => ({ "data-provider": attrs.provider }),
      },
      url: {
        default: null,
        parseHTML: (el) => el.getAttribute("data-url"),
        renderHTML: (attrs) => ({ "data-url": attrs.url }),
      },
    };
  },

  parseHTML() {
    return [{ tag: "div[data-pw-embed]" }];
  },

  renderHTML({ node, HTMLAttributes }) {
    const provider = embedProviders[node.attrs.provider] ?? String(node.attrs.provider ?? "");
    return [
      "div",
      {
        ...HTMLAttributes,
        "data-pw-embed": "",
        class: "pw-placeholder-block",
        contenteditable: "false",
      },
      `埋め込み（${provider}）：${node.attrs.url ?? ""}`,
    ];
  },
});

/** Raw HTML carried over from migrations. Kept as-is and shown as a read-only block. */
export const RawHtml = Node.create({
  name: "rawHtml",
  group: "block",
  atom: true,
  selectable: true,

  addAttributes() {
    return {
      html: {
        default: "",
        parseHTML: (el) => el.getAttribute("data-html") ?? "",
        renderHTML: (attrs) => ({ "data-html": attrs.html }),
      },
    };
  },

  parseHTML() {
    return [{ tag: "div[data-pw-raw-html]" }];
  },

  renderHTML({ HTMLAttributes }) {
    return [
      "div",
      {
        ...HTMLAttributes,
        "data-pw-raw-html": "",
        class: "pw-placeholder-block",
        contenteditable: "false",
      },
      "カスタムHTML（移行した内容です。この画面では編集できません）",
    ];
  },
});

export const extensions: Extensions = [
  StarterKit.configure({
    heading: { levels: [2, 3, 4] },
    codeBlock: false,
    strike: false,
    underline: false,
    link: {
      openOnClick: false,
      autolink: true,
      defaultProtocol: "https",
      HTMLAttributes: { rel: null, target: null },
    },
  }),
  TableKit.configure({ table: { resizable: false } }),
  Ruby,
  Image,
  Embed,
  RawHtml,
];
