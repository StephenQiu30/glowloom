import { describe, it, expect } from "vitest";
import { createCanvasNode } from "@/components/canvas/lib/canvas-node-factory";
import {
  serviceGraph,
  serviceGraphOperations,
  richTextConfig,
} from "@/components/canvas/service-graph";
import { emptyPromptDraft } from "@/components/generation/prompt-document";
const canvas = "11111111-1111-4111-8111-111111111111",
  project = "22222222-2222-4222-8222-222222222222",
  node = "33333333-3333-4333-8333-333333333333";
function document(): API.DocumentDTO {
  return {
    id: canvas,
    project_id: project,
    revision: "5",
    schema_version: 1,
    operation_version: 1,
    permissions: { read: true, write: true },
    nodes: [
      {
        id: node,
        node_type: "text",
        node_action: "resource",
        title: "正文",
        x: 10,
        y: 20,
        width: 320,
        height: 200,
        config: { format: "plain", format_version: 1, text: "原文" },
      },
    ],
    edges: [],
  };
}
describe("正式画布服务接线", () => {
  it("creates nodes with service-compatible UUIDs", () => {
    const created = createCanvasNode("text", { x: 10, y: 20 });
    expect(created.id).toMatch(
      /^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/,
    );
  });
  it("saves parameters with an exact baseline and restores them on reopen", () => {
    const dto = document();
    const before = serviceGraph(dto, canvas, project);
    const after = structuredClone(before);
    after.nodes[0].metadata!.fontSize = 28;
    after.nodes[0].documentData!.parameters = {
      mode: "GENERATE",
      modelSelection: { connectionId: project, modelId: "image-model" },
      modelParameters: { size: "1024x1024", count: 2 },
    };
    const ops = serviceGraphOperations(before, after);
    expect(ops).toHaveLength(1);
    expect(ops[0]).toMatchObject({
      type: "set",
      path: ["config", "editor"],
      expected_exists: false,
    });
    dto.nodes![0].config!.editor = ops[0].value as API.EditorConfig;
    const reopened = serviceGraph(
      JSON.parse(JSON.stringify(dto)),
      canvas,
      project,
    );
    expect(reopened.nodes[0].metadata!.fontSize).toBe(28);
    expect(reopened.nodes[0].documentData!.parameters).toEqual(
      after.nodes[0].documentData!.parameters,
    );
    expect(serviceGraphOperations(reopened, structuredClone(reopened))).toEqual(
      [],
    );
  });
  it("replaces media with original asset CAS and preserves stable reference keys", () => {
    const dto = document();
    dto.nodes![0] = {
      ...dto.nodes![0],
      node_type: "image",
      config: {},
      media_ref: { asset_id: project },
      content_version: "1",
    };
    const before = serviceGraph(dto, canvas, project),
      after = structuredClone(before);
    after.nodes[0].documentData!.fileReferences![0].fileId = canvas;
    after.nodes[0].documentData!.fileReferences![0].version = "2";
    const ops = serviceGraphOperations(before, after);
    expect(ops).toContainEqual({
      type: "replace_media",
      node_id: node,
      expected_media_ref: { asset_id: project },
      new_media_ref: { asset_id: canvas },
    });
    const editor = ops.find((op) => op.path?.toString() === "config,editor")!
      .value as API.EditorConfig;
    dto.nodes![0].config!.editor = editor;
    dto.nodes![0].media_ref = { asset_id: canvas };
    const reopened = serviceGraph(dto, canvas, project);
    expect(reopened.nodes[0].documentData!.fileReferences).toEqual(
      after.nodes[0].documentData!.fileReferences,
    );
  });
  it("preserves typed connection ports, bindings and identity when edited", () => {
    const dto = document();
    dto.nodes!.push({ ...dto.nodes![0], id: project });
    const before = serviceGraph(dto, canvas, project),
      after = structuredClone(before);
    const connection = {
      id: canvas,
      fromNodeId: node,
      toNodeId: project,
      documentData: {
        id: canvas,
        fromNodeId: node,
        toNodeId: project,
        fromPortId: "text",
        toPortId: "prompt",
        purpose: "PROMPT",
        order: 0,
        binding: { kind: "NODE_VALUE" },
      },
    };
    after.connections.push(connection);
    const ops = serviceGraphOperations(before, after);
    const edge = ops.find((op) => op.type === "connect")!.edge as NonNullable<
      API.DocumentDTO["edges"]
    >[number];
    expect(edge.binding).toMatchObject({
      schemaVersion: 1,
      fromPortId: "text",
      toPortId: "prompt",
      binding: { kind: "NODE_VALUE" },
    });
    dto.edges = [edge];
    const reopened = serviceGraph(dto, canvas, project);
    expect(reopened.connections[0].documentData).toEqual(
      connection.documentData,
    );
    const changed = structuredClone(reopened);
    changed.connections[0].documentData!.order = 3;
    expect(serviceGraphOperations(reopened, changed)[0]).toMatchObject({
      type: "set_edge",
      node_id: canvas,
      expected: edge,
    });
  });
  it("marks local media replacement as pending until the uploaded identity is available", () => {
    const dto = document();
    dto.nodes![0] = {
      ...dto.nodes![0],
      node_type: "image",
      config: {},
      media_ref: { asset_id: project },
      content_version: "1",
    };
    const before = serviceGraph(dto, canvas, project),
      after = structuredClone(before);
    after.nodes[0].metadata!.content = "blob:replacement";
    expect(() => serviceGraphOperations(before, after)).toThrow("上传后保存");
    after.nodes[0].documentData!.fileReferences![0].fileId = canvas;
    expect(
      serviceGraphOperations(before, after).some(
        (op) => op.type === "replace_media",
      ),
    ).toBe(true);
  });
  it("persists layer changes", () => {
    const before = serviceGraph(document(), canvas, project);
    const after = structuredClone(before);
    after.nodes[0].documentData!.zIndex = 8;
    expect(serviceGraphOperations(before, after)).toEqual([
      { type: "set", node_id: node, path: ["z_index"], value: 8 },
    ]);
  });
  it("uses UUID identity and rejects another canvas", () => {
    expect(() => serviceGraph(document(), project, canvas)).toThrow("身份");
  });
  it("preserves headings, formatting, duplicate references and label allocations on reopen", () => {
    const dto = document();
    const composer = emptyPromptDraft();
    composer.references = [
      {
        id: "ref-a",
        source: { kind: "media", sourceId: project, sourceVersion: "1" },
        bindingGeneration: 1,
        mediaKind: "image",
        role: "reference_image",
        order: 0,
      },
    ];
    composer.labelState = {
      highWater: { image: 4, video: 0, audio: 0, subject: 0 },
      allocations: { "ref-a": { labelKind: "image", ordinal: 4 } },
    };
    composer.document.content = [
      {
        type: "paragraph",
        content: [
          { type: "reference", attrs: { referenceId: "ref-a" } },
          { type: "reference", attrs: { referenceId: "ref-a" } },
        ],
      },
    ];
    dto.nodes![0].config = {
      format: "tiptap",
      format_version: 1,
      composer: composer as unknown as API.PromptDraft,
    };
    const before = serviceGraph(dto, canvas, project),
      after = structuredClone(before);
    after.nodes[0].metadata!.richText = {
      schemaVersion: 1,
      content: {
        type: "doc",
        content: [
          {
            type: "heading",
            attrs: { level: 2 },
            content: [
              { type: "text", text: "更新", marks: [{ type: "bold" }] },
              { type: "reference", attrs: { referenceId: "ref-a" } },
              { type: "reference", attrs: { referenceId: "ref-a" } },
            ],
          },
        ],
      },
    };
    const ops = serviceGraphOperations(before, after);
    expect(ops).toHaveLength(1);
    expect(ops[0]).toMatchObject({
      type: "set",
      path: ["config", "text_content"],
      expected_exists: true,
    });
    const saved = ops[0].value as API.NodeConfig;
    expect(saved.composer!.labelState).toEqual(composer.labelState);
    expect(saved.composer!.references).toEqual(composer.references);
    dto.nodes![0].config = saved;
    const reopened = serviceGraph(
      JSON.parse(JSON.stringify(dto)),
      canvas,
      project,
    );
    expect(reopened.nodes[0].metadata!.richText).toEqual(
      after.nodes[0].metadata!.richText,
    );
    expect(serviceGraphOperations(reopened, structuredClone(reopened))).toEqual(
      [],
    );
  });
  it("does not persist camera or preview URLs", () => {
    const before = serviceGraph(document(), canvas, project),
      after = structuredClone(before);
    after.viewport = { x: 100, y: 200, k: 2 };
    expect(serviceGraphOperations(before, after)).toEqual([]);
  });
  it("preserves protected business fields when moving existing nodes", () => {
    const dto = document();
    dto.nodes![0] = {
      ...dto.nodes![0],
      node_type: "generation",
      node_action: "tool",
      config: { generation: { prompt: "原始配置" } },
    };
    const before = serviceGraph(dto, canvas, project),
      after = structuredClone(before);
    after.nodes[0].position = { x: 88, y: 99 };
    expect(serviceGraphOperations(before, after)).toEqual([
      {
        type: "set",
        node_id: node,
        path: ["position"],
        value: { x: 88, y: 99 },
      },
    ]);
    expect(after.nodes[0].serviceData!.config).toEqual(dto.nodes![0].config);
  });
  it("rejects a missing reference binding rather than flattening the atom", () => {
    const state = serviceGraph(document(), canvas, project);
    state.nodes[0].metadata!.richText = {
      schemaVersion: 1,
      content: {
        type: "doc",
        content: [
          {
            type: "paragraph",
            content: [{ type: "reference", attrs: { referenceId: "missing" } }],
          },
        ],
      },
    };
    expect(() => richTextConfig(state.nodes[0])).toThrow();
  });
});
