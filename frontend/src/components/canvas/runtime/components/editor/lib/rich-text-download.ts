import type { JSONContent } from "@tiptap/core";
import type { RichTextDocument, RichTextReference } from "../types/rich-text";
import { parseRichTextDocument } from "./rich-text-document";

/** Export references as readable labels, never private asset URLs or binding IDs. */
export async function documentMarkdown(
  document: RichTextDocument,
  references: RichTextReference[] = [],
): Promise<string> {
  const validated = parseRichTextDocument(document);
  const [{ MarkdownManager }, { default: StarterKit }, { Mark }] =
    await Promise.all([
      import("@tiptap/markdown"),
      import("@tiptap/starter-kit"),
      import("@tiptap/core"),
    ]);
  const labels = new Map(
    references.map((ref) => [ref.id, ref.token ?? ref.label]),
  );
  function readable(node: JSONContent): JSONContent {
    if (node.type === "reference" || node.type === "mention") {
      return {
        type: "text",
        text:
          labels.get(String(node.attrs?.referenceId ?? node.attrs?.id)) ??
          String(node.attrs?.token ?? node.attrs?.label ?? "引用素材"),
      };
    }
    return {
      ...node,
      ...(node.content ? { content: node.content.map(readable) } : {}),
    };
  }
  const underline = Mark.create({
    name: "underline",
    renderMarkdown: (node, helpers) => `<u>${helpers.renderChildren(node)}</u>`,
  });
  return new MarkdownManager({
    extensions: [StarterKit.configure({ underline: false }), underline],
  }).serialize(readable(validated.content));
}

export async function downloadRichTextMarkdown(
  document: RichTextDocument,
  title: string,
  references?: RichTextReference[],
) {
  const markdown = await documentMarkdown(document, references);
  const { saveAs } = await import("file-saver");
  const filename =
    title.replace(/[\\/:*?"<>|\u0000-\u001f]/g, "_").trim() || "文本";
  saveAs(
    new Blob([markdown], { type: "text/markdown;charset=utf-8" }),
    `${filename}.md`,
  );
}
