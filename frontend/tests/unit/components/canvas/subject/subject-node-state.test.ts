import { expect, it } from "vitest";
import { registerBuiltinNodes } from "@/components/canvas/nodes/builtin-nodes";
import { createCanvasNode } from "@/components/canvas/lib/canvas-node-factory";
import {
  importCanvasSubjects,
  materializeSubject,
  localSubjectConfig,
  projectSubjectMedia,
  requireSubjectLocal,
  subjectConfig,
} from "@/components/canvas/subject/subject-node-state";
import type { CanvasEditorState } from "@/components/canvas/lib/canvas-document-adapter";
import type { UnifiedSubject } from "@/components/subjects/unified-contracts";
registerBuiltinNodes();
function graph(): CanvasEditorState {
  return {
    nodes: [createCanvasNode("subject", { x: 100, y: 100 })],
    connections: [],
    viewport: { x: 0, y: 0, k: 1 },
    backgroundMode: "dots",
    showImageInfo: false,
  };
}
const subjects = [1, 2].map(
  (i) =>
    ({
      id: `11111111-1111-4111-8111-11111111111${i}`,
      version_id: `22222222-2222-4222-8222-22222222222${i}`,
      name: `主体${i}`,
    }) as UnifiedSubject,
);
it("fills the original node and adds distinct pinned nodes in one graph mutation", () => {
  const original = graph();
  const target = original.nodes[0];
  const next = importCanvasSubjects(original, target.id, subjects);
  expect(next.nodes[0].id).toBe(target.id);
  expect(next.nodes.map((n) => n.title)).toEqual(["主体1", "主体2"]);
  expect(new Set(next.nodes.map((n) => n.id)).size).toBe(2);
  expect(subjectConfig(next.nodes[1])).toEqual({
    version: 1,
    subject_id: subjects[1].id,
    version_id: subjects[1].version_id,
  });
  expect(next.nodes[1].position.x).toBeGreaterThan(
    target.position.x + target.width,
  );
  expect(subjectConfig(original.nodes[0])).toEqual({ version: 1 });
  expect(next.connections).toBe(original.connections);
});
it("rejects missing target, duplicate selections and incomplete versions without changing the graph", () => {
  const original = graph();
  expect(() => importCanvasSubjects(original, "missing", subjects)).toThrow();
  expect(() =>
    importCanvasSubjects(original, original.nodes[0].id, [
      subjects[0],
      subjects[0],
    ]),
  ).toThrow();
  expect(() =>
    importCanvasSubjects(original, original.nodes[0].id, [
      subjects[0],
      { ...subjects[1], version_id: "" },
    ]),
  ).toThrow();
  expect(subjectConfig(original.nodes[0])).toEqual({ version: 1 });
  expect(original.nodes).toHaveLength(1);
});
it("rejects malformed pins instead of silently treating them as an empty node", () => {
  const target = graph().nodes[0];
  target.serviceData!.config!.subject = {
    version: 1,
    subject_id: subjects[0].id,
  };
  expect(() => subjectConfig(target)).toThrow();
});

it("materializes a fixed pin without mutating the library and drops its identity once every binding is replaced", () => {
  const primary = {
    asset_id: "33333333-3333-4333-8333-333333333333",
    asset_revision: 4,
    kind: "image",
    binding_id: "44444444-4444-4444-8444-444444444444",
  };
  const subject = {
    ...subjects[0],
    description: "库说明",
    primary,
    references: [],
    voice: undefined,
  } as unknown as UnifiedSubject;
  const pin = {
    version: 1 as const,
    subject_id: subject.id,
    version_id: subject.version_id,
  };
  const local = materializeSubject(pin, subject);
  local.description = "局部说明";
  expect(subject.description).toBe("库说明");
  expect(localSubjectConfig(pin, local).subject_id).toBe(subject.id);
  local.primary = {
    asset_id: "55555555-5555-4555-8555-555555555555",
    asset_revision: 1,
    kind: "video",
  };
  expect(localSubjectConfig(pin, local)).toEqual({ version: 1, local });
  expect(subject.primary).toEqual(primary);
  expect(() =>
    materializeSubject(pin, { ...subject, version_id: subjects[1].version_id }),
  ).toThrow();
});
it("rejects cross-project assets and malformed local roles before adoption", () => {
  const project = "11111111-1111-4111-8111-111111111111";
  const asset = {
    id: "33333333-3333-4333-8333-333333333333",
    project_id: project,
    revision: 2,
    kind: "image",
  };
  const primary = projectSubjectMedia(asset, project);
  expect(primary).toEqual({
    asset_id: asset.id,
    asset_revision: 2,
    kind: "image",
  });
  expect(() => projectSubjectMedia(asset, subjects[1].id)).toThrow();
  expect(() =>
    projectSubjectMedia({ ...asset, revision: 0 }, project),
  ).toThrow();
  expect(() =>
    requireSubjectLocal(
      { description: "", primary, references: [primary] },
      false,
    ),
  ).toThrow();
  expect(() =>
    requireSubjectLocal(
      {
        description: "",
        primary: { ...primary, kind: "audio" },
        references: [],
      },
      false,
    ),
  ).toThrow();
  expect(() =>
    requireSubjectLocal(
      {
        description: "",
        primary: { ...primary, binding_id: subjects[0].id },
        references: [],
      },
      false,
    ),
  ).toThrow();
});
it("clones local drafts so cancelling edits leaves the saved node untouched", () => {
  const local = { description: "原文", references: [] };
  const draft = materializeSubject({ version: 1, local });
  draft.description = "未保存";
  expect(local.description).toBe("原文");
});
