import { expect, it } from "vitest";
import {
  emptyPromptDraft,
  validatePromptDraft,
  referenceLabel,
} from "@/components/generation/prompt-document";
import { reconcileLabels } from "@/components/generation/draft-resolution";

it("小说文档引用有独立标号，同时继续接受已有四类计数草稿", () => {
  const legacy = emptyPromptDraft();
  expect(() => validatePromptDraft(legacy)).not.toThrow();
  const draft = structuredClone(legacy);
  draft.references.push({
    id: "novel",
    source: { kind: "media", sourceId: "asset", sourceVersion: "1" },
    bindingGeneration: 1,
    mediaKind: "document",
    role: "reference_document",
    order: 0,
  });
  draft.labelState.highWater.document = 1;
  draft.labelState.allocations.novel = { labelKind: "document", ordinal: 1 };
  expect(() => validatePromptDraft(draft)).not.toThrow();
  expect(referenceLabel(draft.labelState, "novel")).toBe("文档1");
  const recovered = reconcileLabels(draft, legacy);
  expect(recovered.draft.references).toEqual(draft.references);
  expect(recovered.draft.labelState.highWater.document).toBe(1);
});
