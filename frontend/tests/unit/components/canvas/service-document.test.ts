import { describe, expect, it } from "vitest";
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
