import {
  emptyPromptDraft,
  validatePromptDraft,
  type PromptDraft,
} from "@/components/generation/prompt-document";
import type { CanvasNodeData } from "./types";

export function textDraft(node: CanvasNodeData): PromptDraft {
  if (node.metadata?.composer) return structuredClone(node.metadata.composer);
  const draft = emptyPromptDraft();
  draft.document.content = (node.metadata?.content ?? "")
    .split("\n")
    .map((text) => ({
      type: "paragraph",
      ...(text ? { content: [{ type: "text" as const, text }] } : {}),
    }));
  return draft;
}
export function validateCanvasComposer(
  value: unknown,
): asserts value is PromptDraft {
  validatePromptDraft(value);
  if (
    value.modelVersion !== null ||
    value.mode !== null ||
    Object.keys(value.params).length
  )
    throw new Error("文本节点不能包含生成配置");
}
export function textConfig(node: CanvasNodeData) {
  const composer = node.metadata?.composer;
  if (composer) {
    validateCanvasComposer(composer);
    return { format: "tiptap", format_version: 1, composer };
  }
  return {
    text: node.metadata?.content ?? "",
    format: "plain",
    format_version: 1,
  };
}

/** Keep existing plain text OT until this node actually owns reference metadata. */
export function withTextDraft(
  node: CanvasNodeData,
  draft: PromptDraft,
): CanvasNodeData {
  validateCanvasComposer(draft);
  if (
    !node.metadata?.composer &&
    !draft.references.length &&
    Object.values(draft.labelState.highWater).every((value) => value === 0)
  ) {
    const content = draft.document.content
      .map((paragraph) =>
        (paragraph.content ?? [])
          .map((item) =>
            item.type === "text"
              ? item.text
              : item.type === "hardBreak"
                ? "\n"
                : "",
          )
          .join(""),
      )
      .join("\n");
    return { ...node, metadata: { ...node.metadata, content } };
  }
  return {
    ...node,
    metadata: { ...node.metadata, content: "", composer: draft },
  };
}
