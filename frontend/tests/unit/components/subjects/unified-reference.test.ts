import { expect, it } from "vitest";
import {
  unifiedSubjectBinding,
  unifiedReferencePreview,
  unifiedSubjectModelIssue,
} from "@/components/subjects/unified-reference";
import type { UnifiedSubject } from "@/components/subjects/unified-contracts";
import {
  emptyPromptDraft,
  validatePromptDraft,
} from "@/components/generation/prompt-document";

const source = {
  binding_id: "11111111-1111-4111-8111-111111111111",
  asset_id: "22222222-2222-4222-8222-222222222222",
  asset_revision: 3,
  kind: "image" as const,
  mime_type: "image/png",
  file_name: "主参考.png",
};
const subject: UnifiedSubject = {
  id: "33333333-3333-4333-8333-333333333333",
  version_id: "44444444-4444-4444-8444-444444444444",
  owner_scope_id: "55555555-5555-4555-8555-555555555555",
  revision: 2,
  name: "几何主体",
  description: "蓝色几何物体",
  primary: source,
  references: [
    { ...source, binding_id: "66666666-6666-4666-8666-666666666666" },
  ],
  voice: {
    name: "音色",
    media: {
      ...source,
      binding_id: "77777777-7777-4777-8777-777777777777",
      kind: "audio",
      mime_type: "audio/wav",
    },
  },
  favorite: false,
  created_at: "2026-10-28T00:00:00Z",
  updated_at: "2026-10-28T00:00:00Z",
};

it("引用冻结版本及完整有序绑定，复用同一素材也不能丢失授权证明", () => {
  const binding = unifiedSubjectBinding(subject);
  expect(binding.source).toEqual({
    kind: "subject",
    sourceId: subject.id,
    sourceVersion: subject.version_id,
    description: "几何主体：蓝色几何物体",
    media: [source, subject.references[0], subject.voice!.media].map(
      (media) => ({
        sourceId: media.asset_id,
        sourceVersion: "3",
        mediaKind: media.kind,
        bindingId: media.binding_id,
      }),
    ),
  });
  const draft = emptyPromptDraft();
  draft.references = [binding];
  draft.labelState.highWater.subject = 1;
  draft.labelState.allocations[binding.id] = {
    labelKind: "subject",
    ordinal: 1,
  };
  expect(() => validatePromptDraft(draft)).not.toThrow();
  // The newly added binding fact survives a normal durable draft round-trip.
  expect(
    JSON.parse(JSON.stringify(draft)).references[0].source.media,
  ).toHaveLength(3);
});

it("没有描述时不添加占位文本；替换时保留原引用角色", () => {
  const binding = unifiedSubjectBinding(
    { ...subject, description: "" },
    "first_frame",
  );
  expect(binding.role).toBe("first_frame");
  expect(binding.source.kind === "subject" && binding.source.description).toBe(
    subject.name,
  );
});

it("主体缩略图只访问固定版本的主参考绑定，不猜测原始私库地址", () => {
  const binding = unifiedSubjectBinding(subject);
  expect(unifiedReferencePreview(binding)).toEqual({
    asset_id: source.asset_id,
    asset_revision: 3,
    kind: "image",
    bound: {
      subjectID: subject.id,
      versionID: subject.version_id,
      bindingID: source.binding_id,
    },
  });
  if (binding.source.kind !== "subject") throw new Error("fixture");
  binding.source.media[0].bindingId = undefined;
  expect(unifiedReferencePreview(binding)).toBeNull();
});

it("不支持附带音色时保留整个主体并明确提示，不默默丢弃音频", () => {
  const references = [unifiedSubjectBinding(subject)];
  expect(unifiedSubjectModelIssue(references)).toBeNull();
  const imageModel: API.ModelResponse = {
    input_roles: ["reference_image"],
    current_version: {
      limits: { roles: { reference_image: { types: ["image"] } } },
    },
  };
  expect(unifiedSubjectModelIssue(references, imageModel)).toContain(
    "音色参考",
  );
  const videoModel: API.ModelResponse = {
    input_roles: ["reference_image", "reference_audio"],
    current_version: {
      limits: {
        roles: {
          reference_image: { types: ["image"] },
          reference_audio: { types: ["audio"] },
        },
      },
    },
  };
  expect(unifiedSubjectModelIssue(references, videoModel)).toBeNull();
  expect(
    references[0].source.kind === "subject" && references[0].source.media,
  ).toHaveLength(3);
});
