import { cleanup, render } from "@testing-library/react";
import { afterEach, expect, it } from "vitest";
import { RichTextContent } from "@/components/canvas/runtime/components/editor/rich-text/rich-text-content";
import {
  documentText,
  parseRichTextDocument,
} from "@/components/canvas/runtime/components/editor/lib/rich-text-document";
import type { RichTextDocument } from "@/components/canvas/runtime/components/editor/types/rich-text";

afterEach(cleanup);
it("退出编辑或重开后有序列表保留起始编号和嵌套格式", () => {
  const document: RichTextDocument = {
    schemaVersion: 1,
    content: {
      type: "doc",
      content: [
        {
          type: "orderedList",
          attrs: { start: 7 },
          content: [
            {
              type: "listItem",
              content: [
                {
                  type: "paragraph",
                  content: [
                    {
                      type: "text",
                      text: "第七镜",
                      marks: [{ type: "bold" }, { type: "underline" }],
                    },
                  ],
                },
              ],
            },
          ],
        },
      ],
    },
  };
  const { container, rerender } = render(
    <RichTextContent
      value={documentText(document.content)}
      document={document}
    />,
  );
  expect(container.querySelector("ol")?.start).toBe(7);
  expect(container.querySelector("u strong")?.textContent).toBe("第七镜");
  const reopened = JSON.parse(JSON.stringify(document)) as RichTextDocument;
  rerender(
    <RichTextContent
      value={documentText(reopened.content)}
      document={reopened}
    />,
  );
  expect(container.querySelector("ol")?.start).toBe(7);
});

it("非编辑展示保留段落与标题对齐", () => {
  const document: RichTextDocument = {
    schemaVersion: 1,
    content: {
      type: "doc",
      content: [
        {
          type: "paragraph",
          attrs: { textAlign: "center" },
          content: [{ type: "text", text: "居中" }],
        },
        {
          type: "heading",
          attrs: { level: 2, textAlign: "right" },
          content: [{ type: "text", text: "靠右" }],
        },
      ],
    },
  };
  const { container } = render(
    <RichTextContent
      value={documentText(document.content)}
      document={document}
    />,
  );
  expect(container.querySelector("p")?.style.textAlign).toBe("center");
  expect(container.querySelector("h2")?.style.textAlign).toBe("right");
});

it("文档导入拒绝非法对齐值和对齐属性所在节点", () => {
  for (const alignment of ["justify", "url(x)", "", null, 3])
    expect(() =>
      parseRichTextDocument({
        schemaVersion: 1,
        content: {
          type: "doc",
          content: [{ type: "paragraph", attrs: { textAlign: alignment } }],
        },
      }),
    ).toThrow("文档对齐方式无效");
  expect(() =>
    parseRichTextDocument({
      schemaVersion: 1,
      content: {
        type: "doc",
        attrs: { textAlign: "left" },
        content: [{ type: "paragraph" }],
      },
    }),
  ).toThrow("文档对齐方式无效");
});
