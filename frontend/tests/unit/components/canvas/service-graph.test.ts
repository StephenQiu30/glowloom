import { describe, it, expect } from "vitest";
import { createCanvasNode } from "@/components/canvas/lib/canvas-node-factory";
import {
  serviceGraph,
  serviceDocumentFromEditor,
  richTextConfig,
} from "@/components/canvas/service-graph";
import { textToDocument } from "@/components/canvas/runtime/components/editor/lib/rich-text-document";
const canvas = "11111111-1111-4111-8111-111111111111",
  project = "22222222-2222-4222-8222-222222222222",
  node = "33333333-3333-4333-8333-333333333333";
function document(): API.DocumentDTO {
  return {
    id: canvas,
    project_id: project,
    revision: "9007199254740993",
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
describe("Yjs editor projection boundary", () => {
  it("roundtrips without changing protected state or bigint revision", () => {
    const dto = document();
    expect(
      serviceDocumentFromEditor(dto, serviceGraph(dto, canvas, project)),
    ).toEqual(dto);
  });
  it("retains protected fields while moving and resizing", () => {
    const dto = document();
    dto.nodes![0] = {
      ...dto.nodes![0],
      node_type: "generation",
      node_action: "tool",
      config: { generation: { prompt: "原始配置" } },
    };
    const state = serviceGraph(dto, canvas, project);
    state.nodes[0].position = { x: 88, y: 99 };
    state.nodes[0].width = 640;
    const projected = serviceDocumentFromEditor(dto, state);
    expect(projected.nodes![0]).toEqual({
      ...dto.nodes![0],
      x: 88,
      y: 99,
      width: 640,
    });
    expect(dto.nodes![0].x).toBe(10);
  });
  it("does not share the camera or temporary preview URL", () => {
    const dto = document();
    const state = serviceGraph(dto, canvas, project);
    state.viewport = { x: 100, y: 200, k: 2 };
    expect(serviceDocumentFromEditor(dto, state)).toEqual(dto);
  });
  it.each(["image", "video", "audio"])(
    "preserves %s upload identity and dimensions",
    (type) => {
      const dto = document();
      dto.nodes![0] = { ...dto.nodes![0], node_type: type, config: {} };
      const state = serviceGraph(dto, canvas, project);
      state.nodes[0].documentData!.fileReferences = [
        {
          fileId: project,
          purpose: "CONTENT",
          referenceKey: "content",
          version: "1",
        },
      ];
      state.nodes[0].width = 480;
      state.nodes[0].height = 270;
      const projection = serviceDocumentFromEditor(dto, state);
      expect(projection.nodes![0]).toMatchObject({
        media_ref: { asset_id: project },
        width: 480,
        height: 270,
      });
      const reopened = serviceGraph(projection, canvas, project);
      expect(reopened.nodes[0].width).toBe(480);
      expect(reopened.nodes[0].height).toBe(270);
      expect(serviceDocumentFromEditor(projection, reopened)).toEqual(
        projection,
      );
    },
  );
  it("refuses unuploaded business bytes", () => {
    const dto = document();
    dto.nodes![0] = { ...dto.nodes![0], node_type: "image", config: {} };
    const state = serviceGraph(dto, canvas, project);
    state.nodes[0].metadata!.content = "blob:pending";
    expect(() => serviceDocumentFromEditor(dto, state)).toThrow("上传后保存");
  });
  it("keeps explicit free resize after reopening", () => {
    const dto = document();
    dto.nodes![0] = { ...dto.nodes![0], node_type: "image", config: {} };
    expect(
      serviceGraph(dto, canvas, project).nodes[0].metadata?.freeResize,
    ).toBe(false);
    const state = serviceGraph(dto, canvas, project);
    state.nodes[0].metadata!.freeResize = true;
    expect(
      serviceGraph(serviceDocumentFromEditor(dto, state), canvas, project)
        .nodes[0].metadata!.freeResize,
    ).toBe(true);
  });
  it.each(["image", "video"])(
    "keeps continued %s rich input when projecting",
    (type) => {
      const dto = document();
      dto.nodes![0] = { ...dto.nodes![0], node_type: type, config: {} };
      const state = serviceGraph(dto, canvas, project);
      const text = {
        schemaVersion: 1 as const,
        content: textToDocument("中文😀继续输入"),
      };
      state.nodes[0].metadata = {
        ...state.nodes[0].metadata,
        prompt: "中文😀继续输入",
        richText: text,
      };
      state.nodes[0].documentData!.textDocument = text;
      const result = serviceDocumentFromEditor(dto, state);
      expect(
        serviceGraph(result, canvas, project).nodes[0].documentData!
          .textDocument,
      ).toEqual(text);
    },
  );
  it("uses stable UUIDs for new nodes and removes deleted nodes", () => {
    const dto = document();
    const state = serviceGraph(dto, canvas, project);
    const created = createCanvasNode("text", { x: 30, y: 40 });
    expect(created.id).toMatch(/^[0-9a-f-]{36}$/);
    state.nodes = [created];
    const next = serviceDocumentFromEditor(dto, state);
    expect(next.nodes).toHaveLength(1);
    expect(next.nodes![0].id).toBe(created.id);
  });
  it("rejects type mutation and missing reference bindings", () => {
    const dto = document();
    const state = serviceGraph(dto, canvas, project);
    state.nodes[0].type = "image";
    expect(() => serviceDocumentFromEditor(dto, state)).toThrow("类型");
    const text = serviceGraph(dto, canvas, project).nodes[0];
    text.metadata!.richText = {
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
    expect(() => richTextConfig(text)).toThrow();
  });
});
