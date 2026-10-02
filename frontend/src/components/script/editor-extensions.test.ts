import { Editor } from "@tiptap/core";
import { expect, it } from "vitest";
import { scriptEditorExtensions } from "./editor-extensions";
import { editorToRich, richToEditor } from "./tiptap-adapter";
import { canonicalRichDocument } from "./rich-document";

it("真实ProseMirror装载空doc/blockquote、组合code格式、0起号与空attrs不补正文或丢格式", () => {
  for (const value of [
    { type: "doc" },
    {
      type: "doc",
      content: [{ type: "blockquote" }, { type: "paragraph", attrs: {} }],
    },
    {
      type: "doc",
      content: [
        {
          type: "paragraph",
          content: [
            {
              type: "text",
              text: "组合",
              marks: [
                { type: "code" },
                { type: "bold" },
                { type: "link", attrs: { href: "https://example.test" } },
                { type: "highlight", attrs: { color: "#1234" } },
              ],
            },
          ],
        },
      ],
    },
    {
      type: "doc",
      content: [
        {
          type: "orderedList",
          attrs: { start: 0 },
          content: [
            {
              type: "listItem",
              content: [
                { type: "paragraph", content: [{ type: "text", text: "零" }] },
              ],
            },
          ],
        },
      ],
    },
    {
      type: "doc",
      content: [
        {
          type: "orderedList",
          content: [{ type: "listItem", content: [{ type: "paragraph" }] }],
        },
      ],
    },
    {
      type: "doc",
      content: [
        {
          type: "orderedList",
          attrs: { start: 1 },
          content: [{ type: "listItem", content: [{ type: "paragraph" }] }],
        },
      ],
    },
  ]) {
    const rich = canonicalRichDocument(value);
    const editor = new Editor({
      element: document.createElement("div"),
      extensions: scriptEditorExtensions(),
      content: richToEditor(rich),
    });
    try {
      expect(editorToRich(editor.getJSON())).toEqual(rich);
    } finally {
      editor.destroy();
    }
  }
});
