import { afterEach, expect, it } from "vitest";
import { createCanvasNode } from "@/components/canvas/lib/canvas-node-factory";
import {
  beginCanvasTransfer,
  clearCanvasTransfer,
  pendingCanvasTransfer,
  requireTransferReceipt,
  requireTransferSource,
} from "@/components/canvas/lib/canvas-transfer-state";
import { emptyPromptDraft } from "@/components/generation/prompt-document";
import { setCreationScope } from "@/lib/request";
const owner = {
  actorID: "actor",
  sessionID: "session",
  scopeID: "scope",
  projectID: "project",
};
const target: API.DocumentDTO = {
  id: "target",
  project_id: "project",
  permissions: { read: true, write: true },
};
afterEach(() => {
  const intent = pendingCanvasTransfer(owner);
  if (intent) clearCanvasTransfer(intent);
});

it("keeps a single intent with fresh target identities, fixed subject pins and independent draft", () => {
  const subject = createCanvasNode("subject", { x: 0, y: 0 });
  subject.serviceData!.config!.subject = {
    version: 1,
    subject_id: crypto.randomUUID(),
    version_id: crypto.randomUUID(),
  };
  subject.serviceData!.input_revision = 9;
  const image = createCanvasNode("image", { x: 500, y: 0 });
  const draft = emptyPromptDraft();
  image.serviceData = {
    config: {
      draft: { composer: draft as unknown as Record<string, unknown> },
    },
    input_revision: 7,
    last_operation_id: crypto.randomUUID(),
  };
  const intent = beginCanvasTransfer(owner, "source", target, {
    nodes: [subject, image],
    connections: [
      { id: crypto.randomUUID(), fromNodeId: subject.id, toNodeId: image.id },
    ],
  });
  expect(intent.copy.nodes.map((n) => n.id)).not.toContain(subject.id);
  expect(intent.copy.connections[0].fromNodeId).toBe(intent.copy.nodes[0].id);
  expect(intent.copy.nodes[0].serviceData?.config?.subject).toEqual(
    subject.serviceData!.config!.subject,
  );
  expect(intent.copy.nodes[0].serviceData?.input_revision).toBeUndefined();
  expect(intent.copy.nodes[1].serviceData?.last_operation_id).toBeUndefined();
  expect(intent.copy.nodes[1].serviceData?.config?.draft?.composer).toEqual(
    draft,
  );
  expect(() =>
    beginCanvasTransfer(owner, "source", target, {
      nodes: [subject],
      connections: [],
    }),
  ).toThrow("尚未确认");
  expect(pendingCanvasTransfer({ ...owner, sessionID: "other" })).toBeNull();
  expect(pendingCanvasTransfer({ ...owner, scopeID: "other" })).toBeNull();
  expect(pendingCanvasTransfer({ ...owner, actorID: "other" })).toBeNull();
  expect(pendingCanvasTransfer({ ...owner, projectID: "other" })).toBeNull();
  expect(() =>
    requireTransferSource(intent, owner, {
      id: "source",
      project_id: "project",
      permissions: { read: false },
      nodes: [subject.serviceData!],
    }),
  ).toThrow();
  expect(() =>
    requireTransferReceipt(intent, owner, { ...target, nodes: [], edges: [] }),
  ).toThrow("未完整读回");
  requireTransferReceipt(intent, owner, {
    ...target,
    nodes: intent.copy.nodes.map((n) => ({
      id: n.id,
      node_type: n.type,
      title: n.title,
      config: n.serviceData?.config,
      input_revision: 1,
    })),
    edges: intent.copy.connections.map((e) => ({ id: e.id })),
  });
});

it("rejects cross-project, read-only target and external active dependencies", () => {
  const timeline = createCanvasNode("timeline", { x: 0, y: 0 });
  timeline.serviceData!.config!.timeline!.clips = [
    { id: crypto.randomUUID(), node_id: crypto.randomUUID() },
  ];
  const selection = { nodes: [timeline], connections: [] };
  expect(() =>
    beginCanvasTransfer(
      owner,
      "source",
      { ...target, project_id: "foreign" },
      selection,
    ),
  ).toThrow("当前项目");
  expect(() =>
    beginCanvasTransfer(
      owner,
      "source",
      { ...target, permissions: { read: true, write: false } },
      selection,
    ),
  ).toThrow("可编辑");
  expect(() => beginCanvasTransfer(owner, "source", target, selection)).toThrow(
    "同时选择",
  );
});

it("releases the private transfer intent when the active creation context changes", () => {
  const node = createCanvasNode("subject", { x: 0, y: 0 });
  const intent = beginCanvasTransfer(owner, "source", target, {
    nodes: [node],
    connections: [],
  });
  setCreationScope("changed-scope");
  expect(pendingCanvasTransfer(owner)).toBeNull();
  expect(() =>
    requireTransferSource(intent, owner, {
      id: "source",
      project_id: "project",
      permissions: { read: true },
      nodes: [{ id: node.id }],
    }),
  ).toThrow("当前身份");
  setCreationScope(null);
});
