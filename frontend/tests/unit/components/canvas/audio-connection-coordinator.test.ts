import { afterEach, expect, it, vi } from "vitest";
import { getSubjectVersion } from "@/api/subjects";
import { setCreationScope } from "@/lib/request";
import type { UnifiedSubject } from "@/components/subjects/unified-contracts";
import { createCanvasAudioConnector } from "@/components/canvas/lib/canvas-audio-connection";
import {
  createCanvasAudioConnectionDraft,
  canvasAudioComposerForPersistence,
  type CanvasAudioModelCatalog,
} from "@/components/canvas/lib/canvas-audio-connection-capability";
import { previewCanvasAudioConnection } from "@/components/canvas/lib/canvas-audio-connection-preview";
import { createCanvasNode } from "@/components/canvas/lib/canvas-node-factory";
import type { CanvasNodeData } from "@/components/canvas/types/canvas";
import type { CanvasYjsLiveSession } from "@/components/canvas/yjs-live-session";
import type { CanvasEditorState } from "@/components/canvas/lib/canvas-document-adapter";
vi.mock("@/api/subjects", () => ({ getSubjectVersion: vi.fn() }));
afterEach(() => vi.clearAllMocks());
function fixture() {
  const identity = {
    projectID: crypto.randomUUID(),
    scopeID: crypto.randomUUID(),
  };
  const model: API.ModelResponse = {
    id: crypto.randomUUID(),
    capability: "audio.generate",
    status: "active",
    provider_status: "active",
    credential_present: true,
    input_roles: ["reference_audio"],
    current_version: {
      id: crypto.randomUUID(),
      modes: ["omni_audio"],
      limits: { roles: { reference_audio: { types: ["audio"] } } },
      param_schema: [],
    },
  };
  const catalog: CanvasAudioModelCatalog = {
    ...identity,
    status: "ready",
    models: [model],
  };
  const source = createCanvasNode("audio", { x: 0, y: 0 });
  const target = createCanvasNode("audio", { x: 500, y: 0 });
  const draft = createCanvasAudioConnectionDraft(catalog, identity).draft;
  target.serviceData = {
    id: target.id,
    node_type: "audio",
    node_action: "generate",
    config: { draft: { composer: canvasAudioComposerForPersistence(draft) } },
  };
  let graph: CanvasEditorState = {
    nodes: [source, target],
    connections: [],
    viewport: { x: 0, y: 0, k: 1 },
    backgroundMode: "dots",
    showImageInfo: false,
  };
  let writable = true,
    ready = true,
    epoch = "one";
  let inputDrafts = {};
  let inputReady = true;
  const change = vi.fn((next: CanvasEditorState) => {
    graph = next;
  });
  const waitForSaved = vi.fn(async (): Promise<API.DocumentDTO> => ({
    nodes: graph.nodes.map((node) => ({
      id: node.id,
      config: node.serviceData?.config,
    })),
    edges: graph.connections.map((edge) => ({ id: edge.id })),
  }));
  const client = {
    getSnapshot: () => ({ document: graph, writable, inputDrafts }),
    getDocumentEpoch: () => epoch,
    canAcceptConnectionGraph: () => ready && writable,
    canAcceptAudioConnectionInput: () => inputReady,
    connectionMediaNode: (node: CanvasNodeData) => node,
    change,
    waitForSaved,
  } as unknown as CanvasYjsLiveSession;
  const read = vi.fn(async () => catalog);
  const connector = createCanvasAudioConnector(
    client,
    identity.projectID,
    identity.scopeID,
    read,
  );
  return {
    identity,
    model,
    catalog,
    source,
    target,
    draft,
    client,
    change,
    waitForSaved,
    read,
    connector,
    request: { sourceNodeIds: [source.id], targetNodeId: target.id },
    graph: () => graph,
    setGraph: (value: CanvasEditorState) => {
      graph = value;
    },
    revoke: () => {
      writable = false;
    },
    changeEpoch: () => {
      epoch = "two";
    },
    unknown: () => {
      ready = false;
    },
    blockInput: () => {
      inputReady = false;
    },
    retainDraft: () => {
      inputDrafts = { [target.id]: { draft } };
    },
  };
}
it("persists preview's exact draft and all edges as one graph", async () => {
  const f = fixture();
  const target = structuredClone(f.target);
  target.id = crypto.randomUUID();
  target.serviceData!.id = target.id;
  expect(
    await f.connector.connect({
      sourceNodeIds: [f.source.id],
      newTarget: target,
    }),
  ).toBe(target.id);
  expect(f.change).toHaveBeenCalledTimes(1);
  expect(
    f.graph().nodes.find((node) => node.id === target.id)?.serviceData?.config
      ?.draft?.composer,
  ).toEqual(f.draft);
  expect(f.graph().connections).toHaveLength(1);
});
it.each(["version", "permission", "source", "target", "epoch", "dispose"])(
  "rejects %s changes during formal lookup without a partial graph",
  async (kind) => {
    const f = fixture();
    f.read.mockImplementationOnce(async () => {
      if (kind === "version") f.model.current_version!.id = crypto.randomUUID();
      if (kind === "permission") f.revoke();
      if (kind === "source" || kind === "target")
        f.setGraph({
          ...f.graph(),
          nodes: f
            .graph()
            .nodes.filter(
              (node) =>
                node.id !== (kind === "source" ? f.source.id : f.target.id),
            ),
        });
      if (kind === "epoch") f.changeEpoch();
      if (kind === "dispose") f.connector.dispose();
      return f.catalog;
    });
    await expect(f.connector.connect(f.request)).rejects.toThrow();
    expect(f.change).not.toHaveBeenCalled();
  },
);
it("preserves unrelated movement while validating capability", async () => {
  const f = fixture();
  f.read.mockImplementationOnce(async () => {
    f.setGraph({
      ...f.graph(),
      nodes: f
        .graph()
        .nodes.map((node) =>
          node.id === f.source.id
            ? { ...node, position: { x: 10, y: 10 } }
            : node,
        ),
    });
    return f.catalog;
  });
  await f.connector.connect(f.request);
  expect(f.graph().nodes[0].position).toEqual({ x: 10, y: 10 });
});
it("requires the exact existing draft and rejects retained unsaved forms", async () => {
  const f = fixture();
  f.retainDraft();
  await expect(f.connector.connect(f.request)).rejects.toThrow();
  expect(f.change).not.toHaveBeenCalled();
});
it("supports a new source and edges atomically and rejects duplicate source IDs/collisions", async () => {
  const f = fixture();
  const source = createCanvasNode("audio", { x: 0, y: 0 });
  await f.connector.connect({
    sourceNodeIds: [source.id],
    newSources: [source],
    targetNodeId: f.target.id,
  });
  expect(f.graph().nodes.some((node) => node.id === source.id)).toBe(true);
  expect(f.change).toHaveBeenCalledTimes(1);
  await expect(
    f.connector.connect({
      sourceNodeIds: [source.id, source.id],
      targetNodeId: f.target.id,
    }),
  ).rejects.toThrow();
  await expect(
    f.connector.connect({
      sourceNodeIds: [f.source.id],
      newSources: [f.source],
      targetNodeId: f.target.id,
    }),
  ).rejects.toThrow();
  expect(f.change).toHaveBeenCalledTimes(1);
});
it("does not create a second node while the original save is unconfirmed", async () => {
  const f = fixture();
  f.waitForSaved.mockImplementationOnce(async () => {
    f.unknown();
    throw new Error("未确认");
  });
  await expect(f.connector.connect(f.request)).rejects.toThrow();
  await expect(
    f.connector.connect({
      sourceNodeIds: [f.source.id],
      newTarget: createCanvasNode("audio", { x: 500, y: 0 }),
    }),
  ).rejects.toThrow();
  expect(f.change).toHaveBeenCalledTimes(1);
});
it("does not report success when readback lacks a created edge", async () => {
  const f = fixture();
  f.waitForSaved.mockResolvedValue({ nodes: [{ id: f.target.id }], edges: [] });
  await expect(f.connector.connect(f.request)).rejects.toThrow("尚未确认");
  expect(f.change).toHaveBeenCalledTimes(1);
});
it("accepts an ordinary empty video source when the formal model supports video", async () => {
  const f = fixture();
  f.source.type = "video";
  f.model.input_roles!.push("reference_video");
  f.model.current_version!.limits!.roles.reference_video = { types: ["video"] };
  expect(
    previewCanvasAudioConnection(f.catalog, f.identity, [f.source], f.target),
  ).toEqual(f.draft);
  await f.connector.connect(f.request);
  expect(f.change).toHaveBeenCalledTimes(1);
});
it("preview rejects unknown pinned subject media and all unsupported local physical kinds", () => {
  const f = fixture();
  const subject = createCanvasNode("subject", { x: 0, y: 0 });
  subject.serviceData!.config!.subject = {
    version: 1,
    subject_id: crypto.randomUUID(),
    version_id: crypto.randomUUID(),
  };
  expect(() =>
    previewCanvasAudioConnection(f.catalog, f.identity, [subject]),
  ).toThrow("尚未就绪");
  subject.serviceData!.config!.subject = {
    version: 1,
    local: {
      description: "",
      primary: {
        asset_id: crypto.randomUUID(),
        asset_revision: 1,
        kind: "image",
      },
      references: [],
      voice: {
        name: "voice",
        media: {
          asset_id: crypto.randomUUID(),
          asset_revision: 1,
          kind: "audio",
        },
      },
    },
  };
  f.model.input_roles!.push("subject");
  f.model.current_version!.limits!.roles.subject = { types: ["image"] };
  expect(() =>
    previewCanvasAudioConnection(f.catalog, f.identity, [subject]),
  ).toThrow("不支持");
  f.model.current_version!.limits!.roles.subject.types.push("audio");
  expect(() =>
    previewCanvasAudioConnection(f.catalog, f.identity, [subject]),
  ).not.toThrow();
});
it("a failed formal lookup never publishes a draft/node/edge prefix", async () => {
  const f = fixture();
  f.read.mockRejectedValue(new Error("offline"));
  await expect(
    f.connector.connect({
      sourceNodeIds: [f.source.id],
      newTarget: createCanvasNode("audio", { x: 500, y: 0 }),
    }),
  ).rejects.toThrow("offline");
  expect(f.change).not.toHaveBeenCalled();
});

it.each(["valid", "scope", "version"])(
  "resolves the pinned subject's actual physical media and exact identity: %s",
  async (kind) => {
    const f = fixture();
    const source = createCanvasNode("subject", { x: 0, y: 0 });
    const pin = {
      subject_id: crypto.randomUUID(),
      version_id: crypto.randomUUID(),
    };
    source.serviceData!.config!.subject = { version: 1, ...pin };
    f.setGraph({ ...f.graph(), nodes: [...f.graph().nodes, source] });
    f.model.input_roles!.push("subject");
    f.model.current_version!.limits!.roles.subject = {
      types: ["image", "audio"],
    };
    const media = {
      binding_id: crypto.randomUUID(),
      asset_id: crypto.randomUUID(),
      asset_revision: 1,
      kind: "image" as const,
      mime_type: "image/png",
      file_name: "primary.png",
    };
    const subject: UnifiedSubject = {
      id: pin.subject_id,
      version_id: kind === "version" ? crypto.randomUUID() : pin.version_id,
      owner_scope_id:
        kind === "scope" ? crypto.randomUUID() : f.identity.scopeID,
      revision: 1,
      name: "主体",
      description: "",
      favorite: false,
      created_at: "2026-10-29T00:00:00Z",
      updated_at: "2026-10-29T00:00:00Z",
      primary: media,
      references: [],
      voice: {
        name: "voice",
        media: {
          ...media,
          binding_id: crypto.randomUUID(),
          asset_id: crypto.randomUUID(),
          kind: "audio",
          mime_type: "audio/wav",
        },
      },
    };
    vi.mocked(getSubjectVersion).mockResolvedValue(subject);
    const result = f.connector.connect({
      sourceNodeIds: [source.id],
      targetNodeId: f.target.id,
    });
    if (kind === "valid") {
      expect(
        previewCanvasAudioConnection(
          f.catalog,
          f.identity,
          [source],
          f.target,
          new Map([[source.id, subject]]),
        ),
      ).toEqual(f.draft);
      const stale = { ...subject, version_id: crypto.randomUUID() };
      expect(() =>
        previewCanvasAudioConnection(
          f.catalog,
          f.identity,
          [source],
          f.target,
          new Map([[source.id, stale]]),
        ),
      ).toThrow("尚未就绪");
      await expect(result).resolves.toBe(f.target.id);
      expect(f.change).toHaveBeenCalledTimes(1);
    } else {
      await expect(result).rejects.toThrow();
      expect(f.change).not.toHaveBeenCalled();
    }
    expect(getSubjectVersion).toHaveBeenCalledWith(
      { id: pin.subject_id, version: pin.version_id },
      { signal: expect.any(AbortSignal) },
    );
  },
);

it.each(["before", "during"])(
  "rejects target IME or unsubmitted input %s catalog resolution",
  async (when) => {
    const f = fixture();
    if (when === "before") f.blockInput();
    else
      f.read.mockImplementationOnce(async () => {
        f.blockInput();
        return f.catalog;
      });
    await expect(f.connector.connect(f.request)).rejects.toThrow();
    expect(f.change).not.toHaveBeenCalled();
  },
);
it("does not silently retry after readback mismatches the persisted composer", async () => {
  const f = fixture();
  f.waitForSaved.mockImplementationOnce(async () => ({
    nodes: [
      {
        id: f.target.id,
        config: {
          draft: {
            composer: canvasAudioComposerForPersistence({
              ...f.draft,
              modelVersion: crypto.randomUUID(),
            }),
          },
        },
      },
    ],
    edges: f.graph().connections.map((edge) => ({ id: edge.id })),
  }));
  await expect(f.connector.connect(f.request)).rejects.toThrow("尚未确认");
  await expect(
    f.connector.connect({
      sourceNodeIds: [f.source.id],
      newTarget: createCanvasNode("audio", { x: 700, y: 0 }),
    }),
  ).rejects.toThrow("上一次");
  expect(f.change).toHaveBeenCalledTimes(1);
});

it("rejects an identity/scope switch after a catalog resolved and before publication", async () => {
  const f = fixture();
  f.read.mockImplementationOnce(async () => {
    setCreationScope(crypto.randomUUID());
    return f.catalog;
  });
  await expect(f.connector.connect(f.request)).rejects.toThrow();
  expect(f.change).not.toHaveBeenCalled();
  setCreationScope(null);
});
