import {
  emptyPromptDraft,
  validatePromptDraft,
  labelKind,
  sameReference,
} from "@/components/generation/prompt-document";
import { expect, it } from "vitest";
import {
  canvasSubjectReference,
  localCanvasSubjectReference,
  requireCanvasSubjectVersion,
} from "@/components/canvas/lib/canvas-subject-source";
import type { UnifiedSubject } from "@/components/subjects/unified-contracts";

const subject: UnifiedSubject = {
  id: "11111111-1111-4111-8111-111111111111",
  version_id: "22222222-2222-4222-8222-222222222222",
  owner_scope_id: "33333333-3333-4333-8333-333333333333",
  revision: 9,
  name: "固定主体",
  description: "第一版内容",
  primary: {
    binding_id: "44444444-4444-4444-8444-444444444444",
    asset_id: "55555555-5555-4555-8555-555555555555",
    asset_revision: 2,
    kind: "image",
    mime_type: "image/png",
    file_name: "main.png",
  },
  references: [
    {
      binding_id: "66666666-6666-4666-8666-666666666666",
      asset_id: "55555555-5555-4555-8555-555555555555",
      asset_revision: 2,
      kind: "image",
      mime_type: "image/png",
      file_name: "reference.png",
    },
  ],
  voice: {
    name: "声音",
    media: {
      binding_id: "77777777-7777-4777-8777-777777777777",
      asset_id: "88888888-8888-4888-8888-888888888888",
      asset_revision: 3,
      kind: "audio",
      mime_type: "audio/mp4",
      file_name: "voice.m4a",
    },
  },
  favorite: false,
  created_at: "2026-10-28T00:00:00Z",
  updated_at: "2026-10-29T00:00:00Z",
};
const pin = { subject_id: subject.id, version_id: subject.version_id };

it("accepts an old pinned content version even when the head revision has advanced", () => {
  expect(
    requireCanvasSubjectVersion(subject, subject.owner_scope_id, pin),
  ).toBe(subject);
  expect(() =>
    requireCanvasSubjectVersion(
      { ...subject, revision: 10 },
      subject.owner_scope_id,
      pin,
    ),
  ).not.toThrow();
});

it("rejects a late response for another version, subject, or current scope", () => {
  for (const changed of [
    { ...subject, version_id: subject.id },
    { ...subject, id: subject.version_id },
    { ...subject, owner_scope_id: subject.id },
  ]) {
    expect(() =>
      requireCanvasSubjectVersion(changed, subject.owner_scope_id, pin),
    ).toThrow();
    expect(() =>
      canvasSubjectReference(changed, subject.owner_scope_id, pin),
    ).toThrow();
  }
});

it("preserves primary, ordered auxiliary and voice binding proofs as one subject reference", () => {
  const reference = canvasSubjectReference(
    subject,
    subject.owner_scope_id,
    pin,
  );
  expect(reference.role).toBe("subject");
  expect(reference.source).toEqual({
    kind: "subject",
    sourceId: subject.id,
    sourceVersion: subject.version_id,
    description: "固定主体：第一版内容",
    media: [subject.primary, ...subject.references, subject.voice!.media].map(
      (media) => ({
        sourceId: media.asset_id,
        sourceVersion: String(media.asset_revision),
        mediaKind: media.kind,
        bindingId: media.binding_id,
      }),
    ),
  });
  expect(() =>
    canvasSubjectReference(
      {
        ...subject,
        voice: {
          ...subject.voice,
          media: { ...subject.voice!.media, binding_id: "" },
        },
      },
      subject.owner_scope_id,
      pin,
    ),
  ).toThrow();
});

it("local subject references use canvas input revision and preserve only the edited physical inputs", () => {
  const canvasID = "99999999-9999-4999-8999-999999999999";
  const nodeID = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa";
  const dto: API.DocumentDTO = {
    id: canvasID,
    project_id: subject.owner_scope_id,
    permissions: { read: true, write: true },
    nodes: [
      {
        id: nodeID,
        node_type: "subject",
        node_action: "resource",
        title: "本地角色",
        input_revision: 3,
        config: {
          subject: {
            version: 1,
            subject_id: subject.id,
            version_id: subject.version_id,
            local: {
              description: "调整后的说明",
              primary: subject.primary,
              references: [],
              voice: subject.voice,
            },
          },
        },
      },
    ],
  };
  const result = localCanvasSubjectReference(
    dto,
    subject.owner_scope_id,
    canvasID,
    nodeID,
    3,
  );
  expect(result.source).toMatchObject({
    kind: "canvas_subject",
    sourceId: nodeID,
    sourceVersion: "3",
    description: "本地角色：调整后的说明",
  });
  if (result.source.kind === "canvas_subject")
    expect(result.source.media).toHaveLength(2);
  expect(() =>
    localCanvasSubjectReference(
      dto,
      subject.owner_scope_id,
      canvasID,
      nodeID,
      2,
    ),
  ).toThrow();
  expect(() =>
    localCanvasSubjectReference(
      { ...dto, project_id: canvasID },
      subject.owner_scope_id,
      canvasID,
      nodeID,
      3,
    ),
  ).toThrow();
  expect(() =>
    localCanvasSubjectReference(
      { ...dto, permissions: { read: false, write: false } },
      subject.owner_scope_id,
      canvasID,
      nodeID,
      3,
    ),
  ).toThrow();
  const draft = emptyPromptDraft();
  draft.references = [result];
  draft.labelState = {
    highWater: { image: 0, video: 0, audio: 0, subject: 1 },
    allocations: { [result.id]: { labelKind: "subject", ordinal: 1 } },
  };
  expect(() => validatePromptDraft(draft)).not.toThrow();
  expect(labelKind(result)).toBe("subject");
  expect(
    sameReference(result, {
      ...result,
      source: {
        ...result.source,
        description: "不同说明",
      } as typeof result.source,
    }),
  ).toBe(false);
});
