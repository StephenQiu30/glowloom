import { z } from "zod";
import { SCRIPT_MAX_DOCUMENT_BYTES, scriptScalarText } from "./rich-document";
import {
  scriptUUID,
  sourceImportItemSchema,
  sourceKindSchema,
  sourceStatusSchema,
  sourceTitleSchema,
} from "./source-model";

export type SourceImportMode = "typed" | "beeftv";
const oldSource = z
  .object({
    id: scriptUUID,
    projectId: scriptUUID,
    kind: sourceKindSchema,
    title: sourceTitleSchema,
    sourceText: scriptScalarText.refine(
      (value) =>
        new TextEncoder().encode(value).length <= SCRIPT_MAX_DOCUMENT_BYTES,
    ),
    wordCount: z.number().int().nonnegative(),
    status: sourceStatusSchema,
    position: z.number().int().nonnegative(),
    createdAt: z.iso.datetime({ offset: true }),
    updatedAt: z.iso.datetime({ offset: true }),
  })
  .strict();
// JSON.parse cannot report duplicate keys; this entry rejects them before mapping.
function rejectDuplicateKeys(text: string) {
  let offset = 0;
  function whitespace() {
    while (/\s/.test(text[offset] ?? "") && offset < text.length) offset++;
  }
  function string() {
    const start = offset++;
    while (offset < text.length) {
      if (text[offset] === "\\") {
        offset += 2;
        continue;
      }
      if (text[offset++] === '"')
        return JSON.parse(text.slice(start, offset)) as string;
    }
    throw new Error("JSON 字符串未结束。");
  }
  function value(depth: number) {
    if (depth > 70) throw new Error("章节 JSON 嵌套过深。");
    whitespace();
    if (text[offset] === '"') {
      string();
      return;
    }
    if (text[offset] === "{") {
      offset++;
      whitespace();
      const keys = new Set<string>();
      if (text[offset] === "}") {
        offset++;
        return;
      }
      for (;;) {
        whitespace();
        const key = string();
        if (keys.has(key))
          throw new Error(`JSON 字段重复：${key}。请先明确保留哪一份输入。`);
        keys.add(key);
        whitespace();
        offset++;
        value(depth + 1);
        whitespace();
        if (text[offset++] === "}") return;
      }
    }
    if (text[offset] === "[") {
      offset++;
      whitespace();
      if (text[offset] === "]") {
        offset++;
        return;
      }
      for (;;) {
        value(depth + 1);
        whitespace();
        if (text[offset++] === "]") return;
      }
    }
    while (offset < text.length && !/[\s,\]}]/.test(text[offset])) offset++;
  }
  value(0);
}
export function parseSourceImport(text: string, mode: SourceImportMode) {
  if (new TextEncoder().encode(text).length > 36 * 1024 * 1024)
    throw new Error("章节 JSON 超过 36MiB，请保留原稿并缩小这次批次。");
  let parsed: unknown;
  try {
    parsed = JSON.parse(text);
  } catch {
    throw new Error("章节 JSON 语法无效。原草稿保留，请检查后再提交。");
  }
  rejectDuplicateKeys(text);
  if (!Array.isArray(parsed) || !parsed.length || parsed.length > 2500)
    throw new Error("每次导入需要 1 至 2500 个完整章节。");
  return parsed.map((item: unknown, index: number) => {
    const checked =
      mode === "typed"
        ? sourceImportItemSchema.safeParse(item)
        : oldSource.safeParse(item);
    if (!checked.success)
      throw new Error(
        `第 ${index + 1} 个章节：${checked.error.issues.map((issue) => `${issue.path.join(".") || "章节"} ${issue.message}`).join("；")}`,
      );
    if (mode === "typed") return sourceImportItemSchema.parse(checked.data);
    const source = oldSource.parse(checked.data);
    return sourceImportItemSchema.parse({
      source_kind: source.kind,
      title: source.title,
      status: source.status,
      original_html: source.sourceText,
      provenance: {
        external_id: source.id,
        external_position: source.position,
      },
    });
  });
}
