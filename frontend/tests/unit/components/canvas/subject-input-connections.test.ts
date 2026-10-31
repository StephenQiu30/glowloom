import { describe, expect, it } from "vitest";
import {
  connectCanvasSubjectSources,
  detachSubjectSources,
  reconcileSubjectSourceRelations,
} from "@/components/canvas/lib/canvas-subject-connections";
import { createCanvasNode } from "@/components/canvas/lib/canvas-node-factory";
import {
  subjectConfig,
  setSubjectConfig,
  importCanvasSubjects,
} from "@/components/canvas/subject/subject-node-state";
import type { CanvasEditorState } from "@/components/canvas/lib/canvas-document-adapter";
import type { UnifiedSubject } from "@/components/subjects/unified-contracts";
import {
  captureCanvasSelection,
  instantiateCanvasClipboard,
} from "@/components/canvas/lib/canvas-clipboard";

const projectID = "22222222-2222-4222-8222-222222222222";
function fixture(kinds = ["image"]) {
  const sources = kinds.map((kind) => {
    const node = createCanvasNode(kind, { x: 0, y: 0 });
    const asset: API.AssetSummary = {
      id: crypto.randomUUID(),
      project_id: projectID,
      revision: 2,
      kind,
      duration_ms:
        kind === "audio" ? 3000 : kind === "video" ? 10000 : undefined,
      file_name: "就绪素材",
    };
    node.serviceData = {
      id: node.id,
      node_type: kind,
      node_action: "resource",
      media_ref: { asset_id: asset.id },
    };
    return { node, asset, sourceNodeId: node.id };
  });
  const target = createCanvasNode("subject", { x: 600, y: 0 });
  const state: CanvasEditorState = {
    nodes: [...sources.map((s) => s.node), target],
    connections: [],
    backgroundMode: "dots",
    showImageInfo: false,
    viewport: { x: 0, y: 0, k: 1 },
  };
  return { state, target, sources, ids: sources.map((s) => s.node.id) };
}
function connect(f: ReturnType<typeof fixture>) {
  return connectCanvasSubjectSources(
    f.state,
    f.ids,
    f.target.id,
    f.sources,
    projectID,
  );
}

describe("subject input domain batch", () => {
  it("library replacement removes prior input bindings together with the local assignment", () => {
    const f = fixture(),
      linked = connect(f);
    const subject: UnifiedSubject = {
      id: crypto.randomUUID(),
      owner_scope_id: crypto.randomUUID(),
      revision: 1,
      version_id: crypto.randomUUID(),
      name: "固定主体",
      description: "",
      references: [],
      favorite: false,
      created_at: "2026-10-29T00:00:00Z",
      updated_at: "2026-10-29T00:00:00Z",
      primary: {
        binding_id: crypto.randomUUID(),
        asset_id: crypto.randomUUID(),
        asset_revision: 1,
        kind: "image",
        mime_type: "image/png",
        file_name: "主体.png",
      },
    };
    const result = importCanvasSubjects(linked, f.target.id, [subject]);
    expect(result.connections).toEqual([]);
    expect(subjectConfig(result.nodes.at(-1)!)).toEqual({
      version: 1,
      subject_id: subject.id,
      version_id: subject.version_id,
    });
    expect(linked.connections).toHaveLength(1);
  });
  it("manual replacement removes only previously bound subject relations", () => {
    const f = fixture(),
      linked = connect(f);
    const visualID = crypto.randomUUID();
    linked.connections.push({
      id: visualID,
      fromNodeId: f.target.id,
      toNodeId: f.ids[0],
      serviceData: {
        id: visualID,
        edge_type: "annotation",
        source_node_id: f.target.id,
        target_node_id: f.ids[0],
      },
    });
    const replaced = {
      ...linked,
      nodes: linked.nodes.map((node) =>
        node.id === f.target.id ? setSubjectConfig(node, { version: 1 }) : node,
      ),
    };
    const result = reconcileSubjectSourceRelations(linked, replaced);
    expect(result.connections.map((edge) => edge.id)).toEqual([visualID]);
    expect(linked.connections).toHaveLength(2);
  });
  it("copy remaps included sources and detaches excluded sources without losing pins", () => {
    const f = fixture(),
      linked = connect(f);
    const all = instantiateCanvasClipboard(
      captureCanvasSelection(
        linked.nodes,
        linked.connections,
        new Set(linked.nodes.map((node) => node.id)),
      ),
      { x: 0, y: 0 },
    );
    expect(subjectConfig(all.nodes[1]).local!.primary?.source_node_id).toBe(
      all.nodes[0].id,
    );
    expect(all.connections[0].fromNodeId).toBe(all.nodes[0].id);
    const only = instantiateCanvasClipboard(
      captureCanvasSelection(
        linked.nodes,
        linked.connections,
        new Set([f.target.id]),
      ),
      { x: 0, y: 0 },
    );
    expect(subjectConfig(only.nodes[0]).local!.primary).toEqual({
      asset_id: f.sources[0].asset.id,
      asset_revision: 2,
      kind: "image",
    });
    expect(only.connections).toEqual([]);
  });
  it("commits main, three auxiliaries and voice with asset pins and real source relations", () => {
    const f = fixture(["audio", "image", "image", "image", "image"]),
      before = structuredClone(f.state);
    const result = connect(f);
    expect(f.state).toEqual(before);
    const local = subjectConfig(result.nodes.at(-1)!).local!;
    expect(local.primary).toMatchObject({
      source_node_id: f.ids[1],
      asset_id: f.sources[1].asset.id,
      asset_revision: 2,
    });
    expect(local.references.map((m) => m.source_node_id)).toEqual(
      f.ids.slice(2),
    );
    expect(local.voice?.media.source_node_id).toBe(f.ids[0]);
    expect(result.connections).toHaveLength(5);
    expect(
      result.connections.every(
        (e) =>
          !e.documentData &&
          e.serviceData?.edge_type === "annotation" &&
          e.toNodeId === f.target.id,
      ),
    ).toBe(true);
  });
  it.each([
    ["image", "image", "image", "image", "image"],
    ["video", "image"],
    ["image", "video"],
    ["image", "audio", "audio"],
    ["audio"],
    ["image", "text"],
  ])("rejects unsupported complete selection %j", (...kinds) => {
    const f = fixture(kinds),
      before = structuredClone(f.state);
    expect(() => connect(f)).toThrow();
    expect(f.state).toEqual(before);
  });
  it.each([1999, 5001, undefined, NaN])(
    "rejects audio duration %s without retaining a primary",
    (duration) => {
      const f = fixture(["image", "audio"]);
      f.sources[1].asset.duration_ms = duration;
      const before = structuredClone(f.state);
      expect(() => connect(f)).toThrow();
      expect(f.state).toEqual(before);
    },
  );
  it.each([2000, 5000])("accepts audio duration boundary %i", (duration) => {
    const f = fixture(["image", "audio"]);
    f.sources[1].asset.duration_ms = duration;
    expect(connect(f).connections).toHaveLength(2);
  });
  it.each([15499, 15000])(
    "accepts video whose rounded seconds are at most 15: %i",
    (duration) => {
      const f = fixture(["video"]);
      f.sources[0].asset.duration_ms = duration;
      expect(connect(f).connections).toHaveLength(1);
    },
  );
  it.each([15500, undefined, NaN, 0])(
    "rejects unavailable or long video duration %s",
    (duration) => {
      const f = fixture(["video"]);
      f.sources[0].asset.duration_ms = duration;
      expect(() => connect(f)).toThrow();
    },
  );
  it("rejects missing, duplicate, stale or foreign resolved sources", () => {
    for (const modify of [
      (f: ReturnType<typeof fixture>) => {
        f.sources.pop();
      },
      (f: ReturnType<typeof fixture>) => {
        f.ids.push(f.ids[0]);
      },
      (f: ReturnType<typeof fixture>) => {
        f.sources[0].asset.id = crypto.randomUUID();
      },
      (f: ReturnType<typeof fixture>) => {
        f.sources[0].asset.project_id = crypto.randomUUID();
      },
      (f: ReturnType<typeof fixture>) => {
        f.sources[0].asset.revision = 0;
      },
      (f: ReturnType<typeof fixture>) => {
        f.sources[0].node.metadata!.status = "loading";
      },
    ]) {
      const f = fixture();
      modify(f);
      const before = structuredClone(f.state);
      expect(() => connect(f)).toThrow();
      expect(f.state).toEqual(before);
    }
  });
  it("rejects repeated assets, existing relations and cycles as whole batches", () => {
    const duplicate = fixture(["image", "image"]);
    duplicate.sources[1].asset.id = duplicate.sources[0].asset.id;
    duplicate.sources[1].node.serviceData!.media_ref!.asset_id =
      duplicate.sources[0].asset.id;
    expect(() => connect(duplicate)).toThrow();
    const f = fixture(),
      linked = connect(f);
    expect(() =>
      connectCanvasSubjectSources(
        linked,
        f.ids,
        f.target.id,
        f.sources,
        projectID,
      ),
    ).toThrow();
    const id = crypto.randomUUID();
    f.state.connections = [
      {
        id,
        fromNodeId: f.target.id,
        toNodeId: f.ids[0],
        serviceData: {
          id,
          edge_type: "annotation",
          source_node_id: f.target.id,
          target_node_id: f.ids[0],
        },
      },
    ];
    expect(() => connect(f)).toThrow();
  });
  it("rejects unmaterialized library targets and preserves existing manual local input", () => {
    const f = fixture();
    f.state.nodes[f.state.nodes.length - 1] = setSubjectConfig(f.target, {
      version: 1,
      subject_id: crypto.randomUUID(),
      version_id: crypto.randomUUID(),
    });
    expect(() => connect(f)).toThrow();
    f.state.nodes[f.state.nodes.length - 1] = setSubjectConfig(f.target, {
      version: 1,
      local: {
        description: "手动说明",
        primary: {
          asset_id: crypto.randomUUID(),
          asset_revision: 1,
          kind: "image",
        },
        references: [],
      },
    });
    const result = connect(f),
      local = subjectConfig(result.nodes.at(-1)!).local!;
    expect(local.description).toBe("手动说明");
    expect(local.references[0].source_node_id).toBe(f.ids[0]);
  });
  it("detaches source bindings after edge or source removal while preserving the asset pin", () => {
    const f = fixture(),
      linked = connect(f);
    const detached = detachSubjectSources(linked.nodes, []);
    const original = subjectConfig(linked.nodes.at(-1)!).local!.primary!;
    expect(subjectConfig(detached.at(-1)!).local!.primary).toEqual({
      asset_id: original.asset_id,
      asset_revision: 2,
      kind: "image",
    });
    expect(original.source_node_id).toBe(f.ids[0]);
    expect(detachSubjectSources(linked.nodes, linked.connections).at(-1)).toBe(
      linked.nodes.at(-1),
    );
  });
});
