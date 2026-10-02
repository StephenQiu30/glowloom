import { expect, it } from "vitest";
import { richToEditor, editorToRich } from "./tiptap-adapter";
import { canonicalRichDocument } from "./rich-document";

it("完整富文本闭集往返包含空块、0起号、全部marks与snake对齐", () => {
  const rich = canonicalRichDocument({
    type: "doc",
    content: [
      { type: "heading", attrs: { level: 3, text_align: "justify" } },
      {
        type: "paragraph",
        attrs: { text_align: "right" },
        content: [
          {
            type: "text",
            text: "😀中é",
            marks: [
              { type: "bold" },
              { type: "italic" },
              { type: "underline" },
              { type: "strike" },
              { type: "code" },
              { type: "link", attrs: { href: "/chapter?q=x#part" } },
              { type: "textColor", attrs: { color: "#12ab34" } },
              { type: "highlight", attrs: { color: "rgba(1, 2, 3, 0.123)" } },
            ],
          },
          { type: "hardBreak" },
        ],
      },
      {
        type: "orderedList",
        attrs: { start: 0 },
        content: [
          {
            type: "listItem",
            content: [
              { type: "paragraph" },
              {
                type: "bulletList",
                content: [
                  {
                    type: "listItem",
                    content: [
                      {
                        type: "paragraph",
                        content: [{ type: "text", text: "nested" }],
                      },
                    ],
                  },
                ],
              },
            ],
          },
        ],
      },
      {
        type: "blockquote",
        content: [
          {
            type: "codeBlock",
            attrs: { language: "plain" },
            content: [{ type: "text", text: "a\nb" }],
          },
        ],
      },
      { type: "horizontalRule" },
    ],
  });
  const editor = richToEditor(rich);
  expect(editor.content?.[0].attrs).toMatchObject({
    level: 3,
    textAlign: "justify",
  });
  expect(editorToRich(editor)).toEqual(rich);
});

it("省略无语义空attrs，保留隐式起号和显式0/1的差异", () => {
  for (const document of [
    { type: "doc", content: [{ type: "paragraph", attrs: {} }] },
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
          attrs: {},
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
    const rich = canonicalRichDocument(document);
    expect(editorToRich(richToEditor(rich))).toEqual(rich);
  }
});

it("仅明确的编辑器默认attrs可归一，未知格式有路径错误并保留原数据", () => {
  const value = {
    type: "doc",
    content: [
      {
        type: "paragraph",
        attrs: { textAlign: null },
        content: [{ type: "text", text: "plain" }],
      },
    ],
  };
  expect(editorToRich(value)).toEqual({
    type: "doc",
    content: [
      { type: "paragraph", content: [{ type: "text", text: "plain" }] },
    ],
  });
  for (const value of [
    { type: "doc", content: [{ type: "table" }] },
    { type: "doc", content: [{ type: "paragraph", attrs: { indent: 2 } }] },
    {
      type: "doc",
      content: [
        {
          type: "paragraph",
          content: [
            {
              type: "text",
              text: "x",
              marks: [
                {
                  type: "textStyle",
                  attrs: { color: "red", fontFamily: "serif" },
                },
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
          type: "paragraph",
          content: [
            {
              type: "text",
              text: "x",
              marks: [{ type: "highlight", attrs: { color: null } }],
            },
          ],
        },
      ],
    },
  ]) {
    const before = JSON.stringify(value);
    expect(() => editorToRich(value)).toThrow(/content/);
    expect(JSON.stringify(value)).toBe(before);
  }
});
