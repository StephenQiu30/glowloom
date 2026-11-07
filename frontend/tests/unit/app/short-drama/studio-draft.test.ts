import { expect, it } from "vitest";
import { createStudioDraft } from "@/app/short-drama/studio-draft";
import { validatePromptDraft } from "@/components/generation/prompt-document";
import {
  studioReferenceItems,
  validateStudioInputReferences,
} from "@/app/short-drama/studio-input";
import type { StyleDetail } from "@/components/styles/style-api";
const style: StyleDetail = {
  id: "style",
  name: "水墨",
  category: "2d",
  revision: 1,
  prompt_fragment: "保留纸张肌理与墨色层次",
  negative_prompt: "霓虹",
  cover_asset_id: null,
  reference_asset_ids: [],
  reference_items: [],
  owned: true,
  can_edit: true,
  references_available: true,
};
it("将已复制的真实项目媒体和冻结风格描述带入可审阅Agent草稿", () => {
  const draft = createStudioDraft(
    {
      kind: "create",
      prompt: "雨夜重逢",
      stylePreset: style,
    },
    [
      {
        id: "project-video",
        project_id: "project",
        kind: "video",
        revision: 2,
      },
      { id: "novel", project_id: "project", kind: "document", revision: 1 },
    ],
  );
  expect(() => validatePromptDraft(draft)).not.toThrow();
  expect(JSON.stringify(draft.document)).toContain("保留纸张肌理与墨色层次");
  expect(JSON.stringify(draft.document)).toContain("避免：霓虹");
  expect(draft.references.map((r) => r.source)).toEqual([
    { kind: "media", sourceId: "project-video", sourceVersion: "2" },
    { kind: "media", sourceId: "novel", sourceVersion: "1" },
  ]);
  expect(draft.labelState.highWater.document).toBe(1);
});
it("风格参考图与创作参考合并后超过30张时先拒绝，重复素材只计一次", () => {
  const reference_items = Array.from({ length: 31 }, (_, i) => ({
    id: `image-${i}`,
    revision: 1,
    asset_id: `image-${i}`,
    asset_revision: 1,
    title: "图",
  }));
  const input = {
    kind: "create" as const,
    prompt: "",
    stylePreset: { ...style, reference_items },
  };
  expect(validateStudioInputReferences(input)).toContain("30");
  expect(
    studioReferenceItems({
      ...input,
      stylePreset: {
        ...style,
        reference_items: [reference_items[0]!, reference_items[0]!],
      },
    }),
  ).toHaveLength(1);
});
it("同一素材冲突版本显示可恢复错误，不把不一致快照交给项目复制", () => {
  const ref = {
    id: "image",
    revision: 1,
    asset_id: "image",
    asset_revision: 1,
    title: "图",
  };
  expect(
    validateStudioInputReferences({
      kind: "create",
      prompt: "",
      stylePreset: {
        ...style,
        reference_items: [ref, { ...ref, revision: 2 }],
      },
    }),
  ).toContain("版本");
});
