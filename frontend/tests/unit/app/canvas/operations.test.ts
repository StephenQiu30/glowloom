import { describe, expect, it } from "vitest";
import {
  usesNode,
  applyOperations,
  transformOperationPair,
  type Operation,
} from "@/app/canvas/operations";
import { removeCanvasNode, type Canvas } from "@/app/canvas/document";
const doc = (): Canvas => ({
  id: "doc",
  project_id: "p",
  revision: 1,
  nodes: [
    {
      id: "n",
      title: "N",
      node_type: "text",
      node_action: "resource",
      x: 0,
      y: 0,
      z_index: 0,
      config: { text: "AB" },
    },
  ],
  edges: [],
});
describe("finite graph OT", () => {
  it("preserves compatible fields and causal steps", () => {
    const d = doc();
    const a: Operation[] = [
      {
        type: "text",
        node_id: "n",
        path: ["config", "text"],
        text: [{ insert: [{ type: "text", text: "X" }] }],
      },
    ];
    const b: Operation[] = [
      {
        type: "text",
        node_id: "n",
        path: ["config", "text"],
        text: [{ retain: 1 }, { delete: 1 }],
      },
      {
        type: "set",
        node_id: "n",
        path: ["position"],
        value: { x: 20, y: 30 },
      },
    ];
    const [ap, bp] = transformOperationPair(d, a, b);
    const ab = applyOperations(applyOperations(d, a), bp),
      ba = applyOperations(applyOperations(d, b), ap);
    expect(ab).toEqual(ba);
    expect(ab.nodes[0].config.text).toBe("XA");
    expect(ab.nodes[0].x).toBe(20);
  });
  it("rejects old invalid text and protects closed fields", () => {
    const d = doc();
    expect(() =>
      transformOperationPair(
        d,
        [
          {
            type: "text",
            node_id: "n",
            path: ["config", "text"],
            text: [{ insert: [{ type: "text", text: "longer" }] }],
          },
        ],
        [
          {
            type: "text",
            node_id: "n",
            path: ["config", "text"],
            text: [{ retain: 3 }],
          },
        ],
      ),
    ).toThrow();
    expect(() =>
      applyOperations(d, [
        {
          type: "set",
          node_id: "n",
          path: ["config"],
          value: { text: "replace" },
        },
      ]),
    ).toThrow();
  });
});

it("deleting a source clears every batch slot without deleting its media", () => {
  const nodes: Canvas["nodes"] = [
    {
      id: "image",
      title: "image",
      node_type: "image",
      node_action: "resource",
      ref_type: "media_asset",
      ref_id: "55555555-5555-4555-8555-555555555555",
      x: 0,
      y: 0,
      z_index: 0,
      config: {},
    },
    {
      id: "table",
      title: "table",
      node_type: "batch_table",
      node_action: "tool",
      x: 0,
      y: 0,
      z_index: 0,
      config: {
        batch_table: {
          version: 1,
          rows: [{ id: "row", input_node_ids: ["image", "image"] }],
        },
      },
    },
  ];
  const result = removeCanvasNode(nodes, "image");
  expect(result).toHaveLength(1);
  expect(result[0].config.batch_table?.rows?.[0].input_node_ids).toEqual([
    null,
    null,
  ]);
  expect(nodes[0].ref_id).toBe("55555555-5555-4555-8555-555555555555");
});

it("detects inserted and restored batch-row graph consumers", () => {
  for (const type of ["insert", "restore_item"] as const) {
    expect(
      usesNode(
        {
          type,
          node_id: "table",
          path: ["config", "batch_table", "rows"],
          item_id: "row",
          value: { id: "row", input_node_ids: [null, "image"] },
        },
        "image",
      ),
    ).toBe(true);
  }
});
