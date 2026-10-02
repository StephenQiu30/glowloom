import { z } from "zod";

export const SCRIPT_MAX_SCALARS = 1_500_000;
export const SCRIPT_MAX_DOCUMENT_BYTES = 8 * 1024 * 1024;
export const scriptScalarText = z
  .string()
  .refine(
    (value) => !/[\uD800-\uDFFF\u0000]/u.test(value),
    "文本包含无效 Unicode 字符。",
  );
export type RichMark = {
  type:
    | "bold"
    | "italic"
    | "underline"
    | "strike"
    | "code"
    | "link"
    | "textColor"
    | "highlight";
  attrs?: { href?: string; color?: string };
};
export type RichDocument = {
  type:
    | "doc"
    | "paragraph"
    | "heading"
    | "text"
    | "hardBreak"
    | "bulletList"
    | "orderedList"
    | "listItem"
    | "blockquote"
    | "codeBlock"
    | "horizontalRule";
  attrs?: {
    level?: number;
    text_align?: "" | "left" | "center" | "right" | "justify";
    start?: number;
    language?: string;
  };
  text?: string;
  marks?: RichMark[];
  content?: RichDocument[];
};

export function normalizeScriptText(value: string) {
  return value.replace(/\r\n?/g, "\n");
}
export function validRichLink(value: string) {
  if (
    !value ||
    new TextEncoder().encode(value).length > 2048 ||
    /[\uD800-\uDFFF\u0000\r\n\t ]/u.test(value) ||
    value.startsWith("\\") ||
    value.startsWith("//")
  )
    return false;
  const scheme = /^([a-zA-Z][a-zA-Z\d+.-]*):/.exec(value)?.[1];
  if (/%(?![0-9a-f]{2})/i.test(value)) return false;
  if (!scheme) return !value.split(/[/?#]/, 1)[0].includes(":");
  if (scheme === "mailto")
    return value.length > 7 && !value.slice(7).startsWith("/");
  if (
    (scheme !== "http" && scheme !== "https") ||
    !value.startsWith(`${scheme}://`)
  )
    return false;
  try {
    const url = new URL(value);
    return Boolean(url.hostname && !url.username && !url.password);
  } catch {
    return false;
  }
}
const namedColors = new Set(
  "aliceblue antiquewhite aqua aquamarine azure beige bisque black blanchedalmond blue blueviolet brown burlywood cadetblue chartreuse chocolate coral cornflowerblue cornsilk crimson cyan darkblue darkcyan darkgoldenrod darkgray darkgreen darkgrey darkkhaki darkmagenta darkolivegreen darkorange darkorchid darkred darksalmon darkseagreen darkslateblue darkslategray darkslategrey darkturquoise darkviolet deeppink deepskyblue dimgray dimgrey dodgerblue firebrick floralwhite forestgreen fuchsia gainsboro ghostwhite gold goldenrod gray green greenyellow grey honeydew hotpink indianred indigo ivory khaki lavender lavenderblush lawngreen lemonchiffon lightblue lightcoral lightcyan lightgoldenrodyellow lightgray lightgreen lightgrey lightpink lightsalmon lightseagreen lightskyblue lightslategray lightslategrey lightsteelblue lightyellow lime limegreen linen magenta maroon mediumaquamarine mediumblue mediumorchid mediumpurple mediumseagreen mediumslateblue mediumspringgreen mediumturquoise mediumvioletred midnightblue mintcream mistyrose moccasin navajowhite navy oldlace olive olivedrab orange orangered orchid palegoldenrod palegreen paleturquoise palevioletred papayawhip peachpuff peru pink plum powderblue purple rebeccapurple red rosybrown royalblue saddlebrown salmon sandybrown seagreen seashell sienna silver skyblue slateblue slategray slategrey snow springgreen steelblue tan teal thistle tomato turquoise violet wheat white whitesmoke yellow yellowgreen transparent".split(
    " ",
  ),
);
function colorChannels(value: string) {
  const match = /^(rgb|rgba)\((.*)\)$/.exec(value);
  if (!match) return null;
  const parts = match[2].split(",").map((part) => part.trim());
  if (
    parts.length !== (match[1] === "rgb" ? 3 : 4) ||
    parts.some(
      (part) => !/^[+-]?(?:\d+(?:\.\d*)?|\.\d+)(?:[eE][+-]?\d+)?$/.test(part),
    )
  )
    return null;
  const channels = parts.map(Number);
  return channels.every(
    (n, i) => Number.isFinite(n) && n >= 0 && n <= (i === 3 ? 1 : 255),
  )
    ? channels
    : null;
}
export function validRichColor(value: string) {
  return (
    /^#(?:[\da-f]{3}|[\da-f]{4}|[\da-f]{6}|[\da-f]{8})$/i.test(value) ||
    namedColors.has(value.toLowerCase()) ||
    colorChannels(value) !== null
  );
}
export function canonicalRichColor(value: string) {
  const lower = value.toLowerCase();
  if (/^#[\da-f]{3,4}$/.test(lower))
    return "#" + [...lower.slice(1)].map((char) => char + char).join("");
  const channels = colorChannels(lower);
  if (lower.startsWith("rgb(") && channels?.every(Number.isInteger))
    return "#" + channels.map((n) => n.toString(16).padStart(2, "0")).join("");
  return lower;
}
const markSchema: z.ZodType<RichMark> = z
  .object({
    type: z.enum([
      "bold",
      "italic",
      "underline",
      "strike",
      "code",
      "link",
      "textColor",
      "highlight",
    ]),
    attrs: z
      .object({
        href: scriptScalarText.optional(),
        color: scriptScalarText.optional(),
      })
      .strict()
      .optional(),
  })
  .strict()
  .refine((mark) => {
    if (mark.type === "link")
      return Boolean(
        mark.attrs?.href && !mark.attrs.color && validRichLink(mark.attrs.href),
      );
    if (mark.type === "textColor" || mark.type === "highlight")
      return Boolean(
        mark.attrs?.color &&
        !mark.attrs.href &&
        validRichColor(mark.attrs.color),
      );
    return !mark.attrs || (!mark.attrs.href && !mark.attrs.color);
  });
const nodeSchema: z.ZodType<RichDocument> = z.lazy(() =>
  z
    .object({
      type: z.enum([
        "doc",
        "paragraph",
        "heading",
        "text",
        "hardBreak",
        "bulletList",
        "orderedList",
        "listItem",
        "blockquote",
        "codeBlock",
        "horizontalRule",
      ]),
      attrs: z
        .object({
          level: z.number().int().nonnegative().optional(),
          text_align: z
            .enum(["", "left", "center", "right", "justify"])
            .optional(),
          start: z.number().int().min(0).max(1_000_000).optional(),
          language: scriptScalarText
            .refine(
              (value) =>
                new TextEncoder().encode(value).length <= 64 &&
                !/[\r\n]/.test(value),
            )
            .optional(),
        })
        .strict()
        .optional(),
      text: scriptScalarText.optional(),
      marks: z.array(markSchema).optional(),
      content: z.array(nodeSchema).optional(),
    })
    .strict(),
);
const blockTypes = new Set([
  "paragraph",
  "heading",
  "bulletList",
  "orderedList",
  "blockquote",
  "codeBlock",
  "horizontalRule",
]);
function placements(node: RichDocument, parent: string): boolean {
  if (!parent) return node.type === "doc";
  if (["doc", "blockquote", "listItem"].includes(parent))
    return blockTypes.has(node.type);
  if (parent === "paragraph" || parent === "heading")
    return node.type === "text" || node.type === "hardBreak";
  if (parent === "codeBlock") return node.type === "text";
  if (parent === "bulletList" || parent === "orderedList")
    return node.type === "listItem";
  return false;
}
function plain(node: RichDocument): string {
  if (node.type === "text") return normalizeScriptText(node.text!);
  if (node.type === "hardBreak") return "\n";
  const separator = ["paragraph", "heading", "codeBlock"].includes(node.type)
    ? ""
    : node.type === "bulletList" || node.type === "orderedList"
      ? "\n"
      : "\n\n";
  return (node.content ?? []).map(plain).join(separator);
}
export const richDocumentBudget = z.unknown().superRefine((value, ctx) => {
  const stack = [{ value, depth: 1 }];
  let count = 0;
  while (stack.length) {
    const current = stack.pop()!;
    if (++count > 200_000 || current.depth > 64) {
      ctx.addIssue({ code: "custom", message: "富文本超过节点或深度预算。" });
      return;
    }
    if (
      typeof current.value === "object" &&
      current.value !== null &&
      "content" in current.value &&
      Array.isArray(current.value.content)
    )
      for (const child of current.value.content)
        stack.push({ value: child, depth: current.depth + 1 });
  }
  try {
    if (
      new TextEncoder().encode(JSON.stringify(value)).length >
      SCRIPT_MAX_DOCUMENT_BYTES
    )
      ctx.addIssue({ code: "custom", message: "富文本超过 8MiB。" });
  } catch {
    ctx.addIssue({ code: "custom", message: "富文本不是有效 JSON 树。" });
  }
});
export const richDocumentSchema = richDocumentBudget
  .pipe(nodeSchema)
  .superRefine((document, ctx) => {
    function visit(
      node: RichDocument,
      parent: string,
      path: (string | number)[],
    ) {
      const attrs = node.attrs;
      if (
        !placements(node, parent) ||
        (node.type === "heading" && (!attrs?.level || attrs.level > 3)) ||
        (attrs?.level && node.type !== "heading") ||
        (attrs?.start !== undefined && node.type !== "orderedList") ||
        (attrs?.language && node.type !== "codeBlock") ||
        (attrs?.text_align && !["paragraph", "heading"].includes(node.type)) ||
        (node.type !== "text" && (node.text || node.marks?.length)) ||
        (node.type === "text" && (!node.text || node.content?.length)) ||
        (["hardBreak", "horizontalRule"].includes(node.type) &&
          node.content?.length) ||
        (node.type === "listItem" && node.content?.[0]?.type !== "paragraph") ||
        (["bulletList", "orderedList"].includes(node.type) &&
          !node.content?.length) ||
        (parent === "codeBlock" && node.marks?.length) ||
        new Set(node.marks?.map((mark) => mark.type)).size !==
          (node.marks?.length ?? 0)
      )
        ctx.addIssue({
          code: "custom",
          path,
          message: "节点、属性或格式不符合剧本富文本合同。",
        });
      node.content?.forEach((child, index) =>
        visit(child, node.type, [...path, "content", index]),
      );
    }
    visit(document, "", []);
    if (Array.from(plain(document)).length > SCRIPT_MAX_SCALARS)
      ctx.addIssue({
        code: "custom",
        message: "正文超过 150 万 Unicode 字符。",
      });
  });
export function canonicalRichDocument(value: unknown): RichDocument {
  const document = richDocumentSchema.parse(value);
  function canonical(node: RichDocument): RichDocument {
    const output: RichDocument = { type: node.type };
    if (node.attrs) {
      output.attrs = {};
      if (node.attrs.level) output.attrs.level = node.attrs.level;
      if (node.attrs.text_align)
        output.attrs.text_align = node.attrs.text_align;
      if (node.attrs.start !== undefined) output.attrs.start = node.attrs.start;
      if (node.attrs.language) output.attrs.language = node.attrs.language;
      if (!Object.keys(output.attrs).length) delete output.attrs;
    }
    if (node.text) output.text = normalizeScriptText(node.text);
    if (node.marks?.length)
      output.marks = node.marks
        .map((mark) =>
          mark.type === "link"
            ? { type: mark.type, attrs: { href: mark.attrs!.href } }
            : mark.type === "textColor" || mark.type === "highlight"
              ? {
                  type: mark.type,
                  attrs: { color: canonicalRichColor(mark.attrs!.color!) },
                }
              : { type: mark.type },
        )
        .sort((a, b) => a.type.localeCompare(b.type, "en"));
    if (node.content?.length) output.content = node.content.map(canonical);
    return output;
  }
  return canonical(document);
}
export function canonicalPlainText(document: RichDocument) {
  return plain(richDocumentSchema.parse(document));
}
export function scalarSlice(text: string, start: number, end: number) {
  scriptScalarText.parse(text);
  const scalars = Array.from(text);
  if (
    !Number.isSafeInteger(start) ||
    !Number.isSafeInteger(end) ||
    start < 0 ||
    end < start ||
    end > scalars.length ||
    end > SCRIPT_MAX_SCALARS
  )
    throw new Error("Unicode 字符范围无效。");
  return scalars.slice(start, end).join("");
}
