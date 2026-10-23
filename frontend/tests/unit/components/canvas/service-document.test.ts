import { withTextDraft, textDraft } from "@/components/canvas/text-content";
import { describe, expect, it } from "vitest";
import { emptyPromptDraft } from "@/components/generation/prompt-document";
import {
  fromServiceDocument,
  graphOperations,
  applyChanges,
  textEdit,
} from "@/components/canvas/service-document";
const id = "10000000-0000-4000-8000-000000000001";
const project = "10000000-0000-4000-8000-000000000002";
const node = "10000000-0000-4000-8000-000000000003";
const dto: API.DocumentDTO = {
  id,
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
      title: "Text",
      x: 0,
      y: 0,
      width: 300,
      height: 200,
      config: { text: "a😀b", format: "plain", format_version: 1 },
    },
  ],
  edges: [],
};
describe("service canvas boundary", () => {
  it("creates groups with the required collapsed config and preserves it on copy", () => {
    const group: API.DocumentDTO = {
      ...dto,
      nodes: [
        { ...dto.nodes![0], node_type: "group", config: { collapsed: true } },
      ],
    };
    const loaded = fromServiceDocument(group, id, project);
    expect(
      graphOperations({ nodes: [], connections: [] }, loaded)[0],
    ).toMatchObject({
      type: "create_node",
      node: { config: { collapsed: true } },
    });
    const fresh = { ...loaded, nodes: [{ ...loaded.nodes[0], metadata: {} }] };
    expect(
      graphOperations({ nodes: [], connections: [] }, fresh)[0],
    ).toMatchObject({
      node: { config: { collapsed: false } },
    });
  });
  it("rejects wrong identity and unsupported nodes without dropping data", () => {
    expect(() => fromServiceDocument(dto, id, node)).toThrow();
    expect(() =>
      fromServiceDocument(
        { ...dto, nodes: [{ ...dto.nodes![0], node_type: "director" }] },
        id,
        project,
      ),
    ).toThrow();
    expect(fromServiceDocument(dto, id, project).nodes).toHaveLength(1);
  });
  it("emits changed fields only, and UTF-16 text edits preserve emoji boundaries", () => {
    const before = fromServiceDocument(dto, id, project);
    const after = structuredClone(before);
    after.nodes[0].position.x = 20;
    expect(graphOperations(before, after)).toEqual([
      {
        type: "set",
        node_id: node,
        path: ["position"],
        value: { x: 20, y: 0 },
      },
    ]);
    expect(textEdit("a😀b", "a😃b")).toEqual([
      { retain: 1 },
      { delete: 2 },
      { insert: [{ type: "text", text: "😃" }] },
    ]);
  });
  it("applies ordered public effects without rounding revisions or mutating the baseline", () => {
    const next = applyChanges(dto, [
      {
        revision: "9007199254740994",
        effects: [
          {
            entity: "node",
            id: node,
            path: ["x"],
            after_exists: true,
            after: 40,
            index: 0,
          },
        ],
      },
    ]);
    expect(next.nodes![0].x).toBe(40);
    expect(dto.nodes![0].x).toBe(0);
    expect(() =>
      applyChanges(dto, [{ revision: "9007199254740995", effects: [] }]),
    ).toThrow();
    expect(() =>
      applyChanges(dto, [
        {
          revision: "9007199254740994",
          effects: [
            {
              entity: "node",
              id: node,
              path: ["__proto__"],
              after_exists: true,
              after: {},
              index: 0,
            },
          ],
        },
      ]),
    ).toThrow();
  });
});

it("结构化正文、重复提及与稳定编号经过保存回执和重新读取仍然一致", () => {
  const draft = emptyPromptDraft();
  draft.references = [
    {
      id: "ref-image",
      bindingGeneration: 1,
      mediaKind: "image",
      role: "reference_image",
      order: 0,
      source: {
        kind: "pending_upload",
        clientUploadId: "upload-one",
        uploadAttemptId: "attempt-one",
      },
    },
  ];
  draft.labelState.highWater.image = 3;
  draft.labelState.allocations = {
    "ref-image": { labelKind: "image", ordinal: 3 },
  };
  draft.document.content = [
    {
      type: "paragraph",
      content: [
        { type: "text", text: "参考😀" },
        { type: "reference", attrs: { referenceId: "ref-image" } },
        { type: "reference", attrs: { referenceId: "ref-image" } },
      ],
    },
  ];
  const before = fromServiceDocument(dto, id, project);
  const after = structuredClone(before);
  after.nodes[0].metadata = { ...after.nodes[0].metadata, composer: draft };
  const operations = graphOperations(before, after);
  expect(operations).toEqual([
    {
      type: "set",
      node_id: node,
      path: ["config", "text_content"],
      expected_exists: true,
      expected: { text: "a😀b", format: "plain", format_version: 1 },
      value: { format: "tiptap", format_version: 1, composer: draft },
    },
  ]);
  const reopened = fromServiceDocument(
    {
      ...dto,
      nodes: [
        { ...dto.nodes![0], config: operations[0].value as API.NodeConfig },
      ],
    },
    id,
    project,
  );
  expect(reopened.nodes[0].metadata?.composer).toEqual(draft);
  expect(graphOperations(reopened, structuredClone(reopened))).toEqual([]);
  expect(() =>
    fromServiceDocument(
      {
        ...dto,
        nodes: [
          {
            ...dto.nodes![0],
            config: {
              format: "tiptap",
              format_version: 1,
              composer: { ...draft, references: [] },
            } as unknown as API.NodeConfig,
          },
        ],
      },
      id,
      project,
    ),
  ).toThrow();
});

it("没有引用元数据的 Tiptap 编辑保留纯文本序列操作", () => {
  const before = fromServiceDocument(dto, id, project);
  const draft = textDraft(before.nodes[0]);
  draft.document.content[0].content = [{ type: "text", text: "a😃b" }];
  const after = { ...before, nodes: [withTextDraft(before.nodes[0], draft)] };
  expect(after.nodes[0].metadata?.composer).toBeUndefined();
  expect(graphOperations(before, after)).toEqual([
    {
      type: "text",
      node_id: node,
      path: ["config", "text"],
      text: [
        { retain: 1 },
        { delete: 2 },
        { insert: [{ type: "text", text: "😃" }] },
      ],
    },
  ]);
});
