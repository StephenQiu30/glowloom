import { z } from "zod";
import {
  canonicalRichDocument,
  richDocumentBudget,
  type RichDocument,
  type RichMark,
} from "./rich-document";

export type EditorNode = {
  type: string;
  attrs?: Record<string, string | number | boolean | null>;
  text?: string;
  marks?: {
    type: string;
    attrs?: Record<string, string | number | boolean | null>;
  }[];
  content?: EditorNode[];
};
const attrsSchema = z.record(
  z.string(),
  z.union([z.string(), z.number(), z.boolean(), z.null()]),
);
const editorNodeSchema: z.ZodType<EditorNode> = z.lazy(() =>
  z
    .object({
      type: z.string(),
      attrs: attrsSchema.optional(),
      text: z.string().optional(),
      marks: z
        .array(
          z
            .object({ type: z.string(), attrs: attrsSchema.optional() })
            .strict(),
        )
        .optional(),
      content: z.array(editorNodeSchema).optional(),
    })
    .strict(),
);
function unsupported(path: string): never {
  throw new Error(`富文本格式未受支持：${path}。原稿保留，请先确认格式。`);
}

export function richToEditor(value: RichDocument): EditorNode {
  const document = canonicalRichDocument(value);
  function node(input: RichDocument): EditorNode {
    const output: EditorNode = { type: input.type };
    if (
      ["paragraph", "heading", "orderedList", "codeBlock"].includes(input.type)
    ) {
      output.attrs = {};
      if (input.type === "paragraph" || input.type === "heading")
        output.attrs.textAlign = input.attrs?.text_align || null;
      if (input.type === "heading") output.attrs.level = input.attrs!.level!;
      if (input.type === "orderedList")
        output.attrs.start = input.attrs?.start ?? null;
      if (input.type === "codeBlock")
        output.attrs.language = input.attrs?.language || null;
    }
    if (input.text) output.text = input.text;
    if (input.marks)
      output.marks = input.marks.map((mark) =>
        mark.type === "textColor"
          ? { type: "textStyle", attrs: { color: mark.attrs!.color! } }
          : mark.attrs
            ? { type: mark.type, attrs: { ...mark.attrs } }
            : { type: mark.type },
      );
    if (input.content) output.content = input.content.map(node);
    return output;
  }
  return node(document);
}
export function editorToRich(value: unknown): RichDocument {
  const parsed = richDocumentBudget.pipe(editorNodeSchema).safeParse(value);
  if (!parsed.success)
    unsupported(
      parsed.error.issues.map((issue) => issue.path.join(".")).join(", "),
    );
  function node(input: EditorNode, path: string): RichDocument {
    const output: RichDocument = { type: input.type as RichDocument["type"] };
    const attrs = input.attrs ?? {};
    const allowed = [
      ...(["paragraph", "heading"].includes(input.type) ? ["textAlign"] : []),
      ...(input.type === "heading" ? ["level"] : []),
      ...(input.type === "orderedList" ? ["start", "type"] : []),
      ...(input.type === "codeBlock" ? ["language"] : []),
    ];
    for (const key of Object.keys(attrs))
      if (!allowed.includes(key) || (key === "type" && attrs[key] !== null))
        unsupported(`${path}.attrs.${key}`);
    const target: NonNullable<RichDocument["attrs"]> = {};
    if (attrs.textAlign !== undefined && attrs.textAlign !== null) {
      if (
        !["left", "center", "right", "justify"].includes(
          String(attrs.textAlign),
        )
      )
        unsupported(`${path}.attrs.textAlign`);
      target.text_align = attrs.textAlign as NonNullable<
        RichDocument["attrs"]
      >["text_align"];
    }
    if (input.type === "heading") {
      if (typeof attrs.level !== "number") unsupported(`${path}.attrs.level`);
      target.level = attrs.level;
    }
    if (input.type === "orderedList") {
      if (attrs.start !== undefined && attrs.start !== null) {
        if (typeof attrs.start !== "number") unsupported(`${path}.attrs.start`);
        target.start = attrs.start;
      }
    }
    if (
      input.type === "codeBlock" &&
      attrs.language !== undefined &&
      attrs.language !== null
    ) {
      if (typeof attrs.language !== "string")
        unsupported(`${path}.attrs.language`);
      target.language = attrs.language;
    }
    if (Object.keys(target).length) output.attrs = target;
    if (input.text !== undefined) output.text = input.text;
    if (input.marks?.length)
      output.marks = input.marks.map((mark, index): RichMark => {
        const markPath = `${path}.marks.${index}`;
        if (mark.type === "textStyle" || mark.type === "highlight") {
          if (
            Object.keys(mark.attrs ?? {}).some((key) => key !== "color") ||
            typeof mark.attrs?.color !== "string" ||
            !mark.attrs.color
          )
            unsupported(markPath);
          return {
            type: mark.type === "textStyle" ? "textColor" : "highlight",
            attrs: { color: mark.attrs.color },
          };
        }
        if (mark.type === "link") {
          if (
            Object.entries(mark.attrs ?? {}).some(
              ([key, value]) =>
                key !== "href" &&
                (!["target", "rel", "class", "title"].includes(key) ||
                  value !== null),
            ) ||
            typeof mark.attrs?.href !== "string"
          )
            unsupported(markPath);
          return { type: "link", attrs: { href: mark.attrs.href } };
        }
        if (
          !["bold", "italic", "underline", "strike", "code"].includes(
            mark.type,
          ) ||
          Object.keys(mark.attrs ?? {}).length
        )
          unsupported(markPath);
        return { type: mark.type as RichMark["type"] };
      });
    if (input.content?.length)
      output.content = input.content.map((child, index) =>
        node(child, `${path}.content.${index}`),
      );
    return output;
  }
  return canonicalRichDocument(node(parsed.data, "document"));
}
