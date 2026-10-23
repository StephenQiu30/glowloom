import type { JSONContent } from "@tiptap/core";
import type {
  RichTextDocument,
  RichTextReference,
} from "@/components/canvas/runtime/components/editor/types/rich-text";

const blockTypes = new Set([
  "doc",
  "paragraph",
  "heading",
  "bulletList",
  "orderedList",
  "listItem",
  "blockquote",
  "codeBlock",
  "horizontalRule",
  "hardBreak",
  "text",
  "mention",
  "reference",
]);
const markTypes = new Set([
  "bold",
  "italic",
  "underline",
  "strike",
  "code",
  "link",
]);

export function referenceToken(reference: RichTextReference) {
  return reference.token ?? reference.label;
}

/** Reconcile by identity, never by a label that another source may now occupy. */
export function reconcileDocumentReferences(
  document: RichTextDocument,
  references: RichTextReference[],
): RichTextDocument {
  const byId = new Map(
    references.map((reference) => [reference.id, reference]),
  );
  function visit(node: JSONContent): JSONContent | null {
    if (node.type === "mention") {
      const reference = byId.get(String(node.attrs?.id));
      if (!reference) return null;
      const token = referenceToken(reference);
      if (node.attrs?.label === reference.label && node.attrs?.token === token)
        return node;
      return {
        ...node,
        attrs: { ...node.attrs, label: reference.label, token },
      };
    }
    if (!node.content) return node;
    const content = node.content.map(visit).filter((child) => child !== null);
    return content.length === node.content.length &&
      content.every((child, i) => child === node.content![i])
      ? node
      : { ...node, content };
  }
  const content = visit(document.content)!;
  return content === document.content ? document : { ...document, content };
}

/** Plain text and stable reference tokens remain compatible with existing APIs. */
export function textToDocument(
  value: string,
  references: RichTextReference[] = [],
): JSONContent {
  const tokens = [...references]
    .filter((r) => referenceToken(r))
    .sort((a, b) => referenceToken(b).length - referenceToken(a).length);
  const pattern = tokens.length
    ? new RegExp(
        tokens
          .map((r) => referenceToken(r).replace(/[.*+?^${}()|[\]\\]/g, "\\$&"))
          .join("|"),
        "g",
      )
    : null;
  const byToken = new Map(tokens.map((r) => [referenceToken(r), r]));
  return {
    type: "doc",
    content: value.split("\n").map((line) => {
      const content: JSONContent[] = [];
      let cursor = 0;
      if (pattern) {
        for (const match of line.matchAll(pattern)) {
          if (match.index > cursor)
            content.push({
              type: "text",
              text: line.slice(cursor, match.index),
            });
          const reference = byToken.get(match[0])!;
          content.push({
            type: "mention",
            attrs: {
              id: reference.id,
              label: reference.label,
              token: referenceToken(reference),
            },
          });
          cursor = match.index + match[0].length;
        }
      }
      if (cursor < line.length)
        content.push({ type: "text", text: line.slice(cursor) });
      return { type: "paragraph", content };
    }),
  };
}

export function documentText(node: JSONContent): string {
  if (node.type === "text") return node.text ?? "";
  if (node.type === "hardBreak") return "\n";
  if (node.type === "reference") return "[引用]";
  if (node.type === "mention") {
    const token: unknown = node.attrs?.token ?? node.attrs?.label;
    return typeof token === "string" ? token : "";
  }
  const children = node.content ?? [];
  return children
    .map(documentText)
    .join(
      node.type === "paragraph" ||
        node.type === "heading" ||
        node.type === "codeBlock"
        ? ""
        : "\n",
    );
}

export function documentForText(
  value: string,
  document?: RichTextDocument,
  references?: RichTextReference[],
): JSONContent {
  return document?.schemaVersion === 1 &&
    documentText(document.content) === value
    ? document.content
    : textToDocument(value, references);
}

/** Bounded validation for exported documents; unsupported nodes never disappear silently. */
export function parseRichTextDocument(value: unknown): RichTextDocument {
  if (
    !value ||
    typeof value !== "object" ||
    !("schemaVersion" in value) ||
    value.schemaVersion !== 1 ||
    !("content" in value)
  ) {
    throw new Error("不支持的文档版本");
  }
  let count = 0;
  function check(node: unknown, depth: number): void {
    if (++count > 10000 || depth > 32 || !node || typeof node !== "object")
      throw new Error("文档结构或容量无效");
    if (
      !("type" in node) ||
      typeof node.type !== "string" ||
      !blockTypes.has(node.type)
    )
      throw new Error("文档包含不支持的内容类型");
    if ("text" in node && typeof node.text !== "string")
      throw new Error("文档文字无效");
    if ("marks" in node) {
      if (!Array.isArray(node.marks)) throw new Error("文档格式无效");
      for (const mark of node.marks) {
        if (
          !mark ||
          typeof mark !== "object" ||
          !("type" in mark) ||
          !markTypes.has(String(mark.type))
        )
          throw new Error("文档包含不支持的格式");
        if (mark.type === "link" && "attrs" in mark) {
          const attrs = mark.attrs;
          if (
            !attrs ||
            typeof attrs !== "object" ||
            !("href" in attrs) ||
            !safeContentLink(attrs.href)
          )
            throw new Error("文档链接仅支持 HTTP 或 HTTPS");
        }
      }
    }
    if (
      "attrs" in node &&
      node.attrs !== null &&
      typeof node.attrs !== "object"
    )
      throw new Error("文档属性无效");
    if ("content" in node) {
      if (!Array.isArray(node.content)) throw new Error("文档内容无效");
      node.content.forEach((child) => check(child, depth + 1));
    }
  }
  check(value.content, 0);
  if (!(
    value.content &&
    typeof value.content === "object" &&
    "type" in value.content &&
    value.content.type === "doc"
  ))
    throw new Error("文档根节点无效");
  const document = value as RichTextDocument;
  // Tiptap's suggestion trigger is editor state, outside the persisted mention contract.
  // Keep every other attribute so unsupported business fields are never silently lost.
  function persist(node: JSONContent): JSONContent {
    const content = node.content?.map(persist);
    if (
      node.type === "mention" &&
      node.attrs &&
      "mentionSuggestionChar" in node.attrs
    ) {
      const attrs = { ...node.attrs };
      delete attrs.mentionSuggestionChar;
      return { ...node, attrs, ...(content ? { content } : {}) };
    }
    return content ? { ...node, content } : node;
  }
  return { ...document, content: persist(document.content) };
}

export function safeContentLink(value: unknown): string | undefined {
  if (typeof value !== "string") return undefined;
  try {
    const url = new URL(value);
    return url.protocol === "https:" || url.protocol === "http:"
      ? url.href
      : undefined;
  } catch {
    return undefined;
  }
}
