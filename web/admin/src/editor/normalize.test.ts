import { Editor } from "@tiptap/core";
import { afterEach, describe, expect, it } from "vitest";
import type { BodyNode } from "../api/types";
import { extensions } from "./extensions";
import { normalizeBody } from "./normalize";

const text = (t: string, marks?: BodyNode["marks"]): BodyNode =>
  marks ? { type: "text", text: t, marks } : { type: "text", text: t };
const p = (...content: BodyNode[]): BodyNode => ({ type: "paragraph", content });

// Uses every node and mark in schema/body-v1.md.
const fullDoc: BodyNode = {
  type: "doc",
  content: [
    { type: "heading", attrs: { level: 2 }, content: [text("会社概要")] },
    {
      type: "paragraph",
      content: [
        text("東京", [{ type: "ruby", attrs: { rt: "とうきょう" } }]),
        text("の会社です。"),
        { type: "hardBreak" },
        text("太字", [{ type: "bold" }]),
        text("斜体", [{ type: "italic" }]),
        text("code", [{ type: "code" }]),
        text("リンク", [
          { type: "link", attrs: { href: "https://example.jp/", target: "_blank" } },
        ]),
        text("内部", [{ type: "link", attrs: { href: "/company/" } }]),
      ],
    },
    { type: "heading", attrs: { level: 3 }, content: [text("h3")] },
    { type: "heading", attrs: { level: 4 }, content: [text("h4")] },
    { type: "bulletList", content: [{ type: "listItem", content: [p(text("項目"))] }] },
    {
      type: "orderedList",
      attrs: { start: 3 },
      content: [{ type: "listItem", content: [p(text("三番目"))] }],
    },
    { type: "blockquote", content: [p(text("引用"))] },
    { type: "horizontalRule" },
    {
      type: "image",
      attrs: { src: "/media/1/a.png", alt: "外観", mediaId: 1, title: "写真" },
    },
    { type: "image", attrs: { src: "https://example.com/b.png", alt: "外部", mediaId: null } },
    {
      type: "table",
      content: [
        {
          type: "tableRow",
          content: [
            {
              type: "tableHeader",
              attrs: { colspan: 1, rowspan: 1 },
              content: [p(text("見出し"))],
            },
            { type: "tableHeader", attrs: { colspan: 1, rowspan: 1 }, content: [p(text("値"))] },
          ],
        },
        {
          type: "tableRow",
          content: [
            { type: "tableCell", attrs: { colspan: 1, rowspan: 1 }, content: [p(text("a"))] },
            { type: "tableCell", attrs: { colspan: 1, rowspan: 1 }, content: [p(text("b"))] },
          ],
        },
      ],
    },
    { type: "embed", attrs: { provider: "youtube", url: "https://www.youtube.com/watch?v=x" } },
    { type: "rawHtml", attrs: { html: '<div class="old">移行<script>x</script></div>' } },
  ],
};

let editor: Editor | undefined;
afterEach(() => editor?.destroy());

const load = (content: BodyNode | string) => {
  editor = new Editor({ extensions, content });
  return editor;
};

describe("body JSON v1", () => {
  it("round-trips every node and mark through the editor unchanged", () => {
    expect(normalizeBody(load(fullDoc).getJSON() as BodyNode)).toEqual(fullDoc);
  });

  it("drops attributes and nodes the schema does not define", () => {
    const out = normalizeBody({
      type: "doc",
      content: [
        { type: "orderedList", attrs: { start: 1, type: null }, content: [] },
        { type: "codeBlock", content: [text("x")] },
        p(
          text("a", [
            { type: "link", attrs: { href: "/", target: "_self", rel: "x", class: null } },
            { type: "strike" },
          ]),
        ),
      ],
    });
    expect(out).toEqual({
      type: "doc",
      content: [
        { type: "orderedList", content: [] },
        p(text("a", [{ type: "link", attrs: { href: "/" } }])),
      ],
    });
  });

  it("renders ruby as <ruby> with rt and parses it back", () => {
    const html = load(fullDoc).getHTML();
    expect(html).toContain("<ruby><span>東京</span><rp");
    expect(html).toContain(">とうきょう</rt>");

    const parsed = load(
      "<p><ruby>漢字<rp>(</rp><rt>かんじ</rt><rp>)</rp></ruby>です</p>",
    ).getJSON();
    expect(normalizeBody(parsed as BodyNode)).toEqual({
      type: "doc",
      content: [p(text("漢字", [{ type: "ruby", attrs: { rt: "かんじ" } }]), text("です"))],
    });
  });

  it("shows images from /api/media/{id}/file in the editor but keeps src", () => {
    const html = load(fullDoc).getHTML();
    expect(html).toContain('src="/api/media/1/file"');
    expect(html).toContain('data-src="/media/1/a.png"');
    expect(html).toContain('src="https://example.com/b.png"');
  });

  it("shows rawHtml and embed as read-only placeholders without rendering the HTML", () => {
    const e = load(fullDoc);
    const dom = e.view.dom;
    const raw = dom.querySelector("[data-pw-raw-html]");
    expect(raw?.getAttribute("contenteditable")).toBe("false");
    expect(raw?.textContent).toContain("カスタムHTML");
    expect(dom.querySelector("script")).toBeNull();
    expect(dom.querySelector("[data-pw-embed]")?.textContent).toContain("YouTube");
  });

  it("applies and removes ruby with commands", () => {
    const e = load({ type: "doc", content: [p(text("東京都"))] });
    e.chain().setTextSelection({ from: 1, to: 3 }).setRuby("とうきょう").run();
    expect(normalizeBody(e.getJSON() as BodyNode).content?.[0]).toEqual(
      p(text("東京", [{ type: "ruby", attrs: { rt: "とうきょう" } }]), text("都")),
    );
    e.chain().setTextSelection(2).unsetRuby().run();
    expect(normalizeBody(e.getJSON() as BodyNode).content?.[0]).toEqual(p(text("東京都")));
  });
});
