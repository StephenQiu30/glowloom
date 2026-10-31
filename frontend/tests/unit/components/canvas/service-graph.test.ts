import { describe, it, expect } from "vitest";
import { emptyPromptDraft } from "@/components/generation/prompt-document";
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
  it("preserves a copied resource's independent generator input", () => {
    const dto = document();
    const input = emptyPromptDraft();
    dto.nodes![0].config!.draft = {
      composer: input as unknown as NonNullable<API.CanvasDraft["composer"]>,
    };
    const state = serviceGraph(dto, canvas, project);
    const copied = structuredClone(state.nodes[0]);
    copied.id = "44444444-4444-4444-8444-444444444444";
    state.nodes.push(copied);
    const saved = serviceDocumentFromEditor(dto, state);
    expect(
      saved.nodes!.find((n) => n.id === copied.id)?.config?.draft?.composer,
    ).toEqual(input);
    expect(saved.nodes![0]).toEqual(dto.nodes![0]);
  });

  it("keeps director scene edits and object identity through save and reopen", () => {
    const dto = document();
    const state = serviceGraph(dto, canvas, project);
    const director = createCanvasNode("director", { x: 800, y: 200 });
    state.nodes.push(director);
    const saved = serviceDocumentFromEditor(dto, state);
    const tool = saved.nodes!.find((item) => item.id === director.id)!;
    expect(tool.node_type).toBe("director");
    expect(tool.node_action).toBe("tool");
    const sceneID = tool.config!.director!.id;
    const reopened = serviceGraph(saved, canvas, project);
    const config = reopened.nodes.find((item) => item.id === director.id)!
      .serviceData!.config!.director!;
    config.mode = "dynamic";
    config.workspaces!.dynamic!.background = "#112233";
    config.workspaces!.dynamic!.shots![0].fps = 60;
    const modified = serviceDocumentFromEditor(saved, reopened);
    const scene = modified.nodes!.find((item) => item.id === director.id)!
      .config!.director!;
    expect(scene).toMatchObject({
      id: sceneID,
      mode: "dynamic",
      version: 2,
      workspaces: {
        dynamic: {
          background: "#112233",
          shots: [expect.objectContaining({ fps: 60 })],
        },
      },
    });
    expect(
      serviceDocumentFromEditor(
        modified,
        serviceGraph(modified, canvas, project),
      ),
    ).toEqual(modified);
    expect(modified.nodes![0]).toEqual(dto.nodes![0]);
  });

  it("creates, fills, reopens and clears a subject pin without replacing its node", () => {
    const dto = document();
    const state = serviceGraph(dto, canvas, project);
    const subjectNode = createCanvasNode("subject", { x: 400, y: 300 });
    state.nodes.push(subjectNode);
    const created = serviceDocumentFromEditor(dto, state);
    expect(
      created.nodes?.find((n) => n.id === subjectNode.id)?.config?.subject,
    ).toEqual({ version: 1 });
    const next = serviceGraph(created, canvas, project);
    const target = next.nodes.find((n) => n.id === subjectNode.id)!;
    target.serviceData!.config!.subject = {
      version: 1,
      subject_id: project,
      version_id: canvas,
    };
    const filled = serviceDocumentFromEditor(created, next);
    expect(
      filled.nodes?.find((n) => n.id === subjectNode.id)?.config?.subject,
    ).toEqual({ version: 1, subject_id: project, version_id: canvas });
    const reopened = serviceGraph(filled, canvas, project);
    expect(serviceDocumentFromEditor(filled, reopened)).toEqual(filled);
    reopened.nodes.find(
      (n) => n.id === subjectNode.id,
    )!.serviceData!.config!.subject = { version: 1 };
    const cleared = serviceDocumentFromEditor(filled, reopened);
    expect(
      cleared.nodes?.find((n) => n.id === subjectNode.id)?.config?.subject,
    ).toEqual({ version: 1 });
    expect(cleared.nodes?.[0]).toEqual(created.nodes?.[0]);
  });

  it("saves generator input independently of the visible resource body", () => {
    const dto = document();
    const state = serviceGraph(dto, canvas, project);
    const input = emptyPromptDraft();
    input.document = {
      type: "doc",
      content: [
        { type: "paragraph", content: [{ type: "text", text: "续写正文" }] },
      ],
    };
    state.nodes[0].serviceData!.config!.draft = {
      composer: input as unknown as NonNullable<API.CanvasDraft["composer"]>,
    };
    const saved = serviceDocumentFromEditor(dto, state);
    expect(saved.nodes![0].node_type).toBe("text");
    expect(saved.nodes![0].config?.text).toBe("原文");
    expect(saved.nodes![0].config?.draft?.composer).toEqual(input);
    const reopened = serviceGraph(saved, canvas, project);
    expect(reopened.nodes[0].metadata?.content).toBe("原文");
    expect(serviceDocumentFromEditor(saved, reopened)).toEqual(saved);
  });
  it("retains text background across service projection and reset without changing document content", () => {
    const dto = document();
    const state = serviceGraph(dto, canvas, project);
    state.nodes[0].metadata = {
      ...state.nodes[0].metadata,
      textBackground: "cyan",
    };
    const saved = serviceDocumentFromEditor(dto, state);
    const reopened = serviceGraph(saved, canvas, project);
    expect(reopened.nodes[0].metadata?.textBackground).toBe("cyan");
    expect(saved.nodes![0].config?.text).toBe("原文");
    reopened.nodes[0].metadata!.textBackground = "default";
    expect(
      serviceGraph(serviceDocumentFromEditor(saved, reopened), canvas, project)
        .nodes[0].metadata?.textBackground,
    ).toBe("default");
  });
  it("creates and reopens a formal timeline tool without flattening its configuration", () => {
    const dto = document();
    const state = serviceGraph(dto, canvas, project);
    const timeline = createCanvasNode("timeline", { x: 500, y: 200 });
    state.nodes.push(timeline);
    const saved = serviceDocumentFromEditor(dto, state);
    const tool = saved.nodes!.find((item) => item.id === timeline.id)!;
    expect(tool.node_action).toBe("tool");
    expect(tool.config?.timeline?.aspect_ratio).toBe("auto");
    const reopened = serviceGraph(saved, canvas, project);
    const editable = reopened.nodes.find((item) => item.id === timeline.id)!;
    editable.serviceData!.config!.timeline!.aspect_ratio = "2.35:1";
    const modified = serviceDocumentFromEditor(saved, reopened);
    expect(
      modified.nodes!.find((item) => item.id === timeline.id)?.config?.timeline
        ?.aspect_ratio,
    ).toBe("2.35:1");
    expect(modified.nodes![0]).toEqual(dto.nodes![0]);
  });
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

it("preserves typed stack/background, nested membership and group metadata through authoritative reopen", () => {
  const dto = document();
  const state = serviceGraph(dto, canvas, project);
  const group = createCanvasNode("group", { x: 0, y: 0 });
  group.serviceData = {
    config: { container: { version: 1, kind: "stack", background: "purple" } },
  };
  group.metadata = { colorTags: ["cyan"] };
  state.nodes.push(group);
  state.nodes[0].metadata = { ...state.nodes[0].metadata, groupId: group.id };
  const saved = serviceDocumentFromEditor(dto, state);
  const actual = saved.nodes!.find((n) => n.id === group.id)!;
  expect(actual.config?.container).toEqual({
    version: 1,
    kind: "stack",
    background: "purple",
  });
  expect(actual.config?.editor?.metadata?.colorTags).toEqual(["cyan"]);
  expect(saved.nodes![0].parent_id).toBe(group.id);
  const reopened = serviceGraph(saved, canvas, project);
  const next = reopened.nodes.find((n) => n.id === group.id)!;
  next.serviceData!.config!.container!.background = "orange";
  const changed = serviceDocumentFromEditor(saved, reopened);
  expect(
    changed.nodes!.find((n) => n.id === group.id)!.config!.container!
      .background,
  ).toBe("orange");
  expect(
    serviceDocumentFromEditor(changed, serviceGraph(changed, canvas, project)),
  ).toEqual(changed);
});
