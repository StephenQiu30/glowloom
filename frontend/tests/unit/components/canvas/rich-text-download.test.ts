import { expect, it } from "vitest";
import { documentMarkdown } from "@/components/canvas/runtime/components/editor/lib/rich-text-download";

it("exports headings, lists, marks and readable references without exposing private binding URLs", async () => {
  const markdown = await documentMarkdown(
    {
      schemaVersion: 1,
      content: {
        type: "doc",
        content: [
          {
            type: "heading",
            attrs: { level: 2 },
            content: [{ type: "text", text: "标题" }],
          },
          {
            type: "paragraph",
            content: [
              { type: "text", text: "加粗", marks: [{ type: "bold" }] },
              { type: "text", text: "下划线", marks: [{ type: "underline" }] },
              { type: "reference", attrs: { referenceId: "private-id" } },
            ],
          },
          {
            type: "bulletList",
            content: [
              {
                type: "listItem",
                content: [
                  {
                    type: "paragraph",
                    content: [{ type: "text", text: "第一项" }],
                  },
                ],
              },
            ],
          },
          {
            type: "blockquote",
            content: [
              { type: "paragraph", content: [{ type: "text", text: "引用" }] },
            ],
          },
        ],
      },
    },
    [
      {
        id: "private-id",
        label: "图片 1",
        title: "图片",
        kind: "image",
        previewUrl: "https://private.example/token",
      },
    ],
  );
  expect(markdown).toContain("## 标题");
  expect(markdown).toContain("**加粗**");
  expect(markdown).toContain("<u>下划线</u>");
  expect(markdown).toContain("- 第一项");
  expect(markdown).toContain("> 引用");
  expect(markdown).toContain("图片 1");
  expect(markdown).not.toMatch(/private-id|private.example/);
});

it("rejects unsafe links rather than exporting executable content", async () => {
  await expect(
    documentMarkdown({
      schemaVersion: 1,
      content: {
        type: "doc",
        content: [
          {
            type: "paragraph",
            content: [
              {
                type: "text",
                text: "unsafe",
                marks: [
                  { type: "link", attrs: { href: "javascript:alert(1)" } },
                ],
              },
            ],
          },
        ],
      },
    }),
  ).rejects.toThrow("HTTP");
});
