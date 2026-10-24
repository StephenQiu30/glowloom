import { cleanup, render } from "@testing-library/react";
import { afterEach, expect, it } from "vitest";
import { RichTextContent } from "@/components/canvas/runtime/components/editor/rich-text/rich-text-content";
import { documentText } from "@/components/canvas/runtime/components/editor/lib/rich-text-document";
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
