import { expect, it } from "vitest";
import {
  richDocumentSchema,
  canonicalPlainText,
  canonicalRichDocument,
  scalarSlice,
} from "./rich-document";

it("规范正文保留空块、HR零宽、列表和Unicode scalar，不trim或合并空白", () => {
  const document = richDocumentSchema.parse({
    type: "doc",
    content: [
      {
        type: "heading",
        attrs: { level: 2, text_align: "center" },
        content: [],
      },
      {
        type: "paragraph",
        content: [
          { type: "text", text: " A😀é\r\n中  " },
          { type: "hardBreak" },
          { type: "text", text: "尾" },
        ],
      },
      { type: "horizontalRule" },
      {
        type: "orderedList",
        attrs: { start: 7 },
        content: [
          {
            type: "listItem",
            content: [
              { type: "paragraph", content: [{ type: "text", text: "甲" }] },
            ],
          },
          {
            type: "listItem",
            content: [
              { type: "paragraph", content: [{ type: "text", text: "乙" }] },
            ],
          },
        ],
      },
      {
        type: "codeBlock",
        attrs: { language: "go" },
        content: [{ type: "text", text: "a\r\nb\rc" }],
      },
    ],
  });
  expect(canonicalPlainText(document)).toBe(
    "\n\n A😀é\n中  \n尾\n\n\n\n甲\n乙\n\na\nb\nc",
  );
  expect(scalarSlice("😀é中", 0, 3)).toBe("😀é");
  expect(() => scalarSlice("😀", 0, 2)).toThrow();
});

it("闭集拒未知节点/属性/marks、危险链接、无效字符和错误嵌套", () => {
  for (const node of [
    { type: "image", attrs: { src: "https://example.test" } },
    { type: "paragraph", attrs: { textAlign: "right" } },
    { type: "heading", attrs: { level: 4 } },
    { type: "text", text: "\uD800" },
    { type: "text", text: "a\u0000b" },
    { type: "listItem", content: [] },
    {
      type: "paragraph",
      content: [{ type: "text", text: "x", marks: [{ type: "mention" }] }],
    },
    {
      type: "paragraph",
      content: [
        {
          type: "text",
          text: "x",
          marks: [{ type: "link", attrs: { href: "javascript:alert(1)" } }],
        },
      ],
    },
    {
      type: "codeBlock",
      content: [{ type: "text", text: "x", marks: [{ type: "bold" }] }],
    },
  ])
    expect(
      richDocumentSchema.safeParse({ type: "doc", content: [node] }).success,
    ).toBe(false);
});

it("富文本格式独立于plain且规范marks顺序、EOL和精确颜色", () => {
  const original = {
    type: "doc",
    content: [
      {
        type: "paragraph",
        content: [
          {
            type: "text",
            text: "正文",
            marks: [
              { type: "textColor", attrs: { color: "rgb(255, 0, 0)" } },
              { type: "bold" },
            ],
          },
        ],
      },
    ],
  };
  const canonical = canonicalRichDocument(original);
  expect(canonical.content?.[0].content?.[0].marks).toEqual([
    { type: "bold" },
    { type: "textColor", attrs: { color: "#ff0000" } },
  ]);
  expect(canonicalPlainText(canonical)).toBe("正文");
  expect(canonicalRichDocument({ type: "doc" })).toEqual({ type: "doc" });
});
