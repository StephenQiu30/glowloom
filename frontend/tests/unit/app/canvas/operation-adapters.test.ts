import { expect, it } from "vitest";
import type { Canvas } from "@/app/canvas/document";
import { canvasOperations } from "@/app/canvas/operation-adapters";
import { applyOperations } from "@/app/canvas/operations";
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
      config: { text: "aaa" },
    },
  ],
  edges: [],
});
it("requires actual text intents while mapping scalar fields separately", () => {
  const base = doc(),
    next = structuredClone(base);
  next.nodes[0].config.text = "aa";
  next.nodes[0].x = 20;
  expect(() => canvasOperations(base, next)).toThrow("unmapped_text_edit");
  const ops = canvasOperations(base, next, {
    text: [
      {
        node_id: "n",
        path: ["config", "text"],
        edits: [[{ retain: 1 }, { delete: 1 }]],
      },
    ],
  });
  expect(ops.find((o) => o.type === "text")?.text).toEqual([
    { retain: 1 },
    { delete: 1 },
  ]);
  expect(applyOperations(base, ops).nodes).toEqual(next.nodes);
});
it("maps an explicit Markdown transaction to a single source CAS, not config", () => {
  const base = doc(),
    next = structuredClone(base);
  next.nodes[0].config.text = "**aaa**";
  const ops = canvasOperations(base, next, { markdownSource: new Set(["n"]) });
  expect(ops).toEqual([
    {
      type: "set",
      node_id: "n",
      path: ["config", "text_source"],
      value: { text: "**aaa**" },
      expected: { text: "aaa" },
      expected_exists: true,
    },
  ]);
});

it("binds batch slots to column IDs across a reorder", () => {
  const base = doc();
  base.nodes = [
    {
      id: "11111111-1111-4111-8111-111111111111",
      title: "Table",
      node_type: "batch_table",
      node_action: "tool",
      x: 0,
      y: 0,
      z_index: 0,
      config: {
        batch_table: {
          version: 1,
          operation: "creative",
          global_prompt: "",
          params: {},
          output_count: 1,
          concurrency: 1,
          mode: "",
          reference_columns: [
            { id: "22222222-2222-4222-8222-222222222222", label: "A" },
            { id: "33333333-3333-4333-8333-333333333333", label: "B" },
          ],
          rows: [
            {
              id: "44444444-4444-4444-8444-444444444444",
              enabled: true,
              prompt: "",
              input_node_ids: [
                "55555555-5555-4555-8555-555555555555",
                null,
              ] as unknown as string[],
            },
          ],
        },
      },
    },
  ];
  const next = structuredClone(base);
  next.nodes[0].config.batch_table!.reference_columns!.reverse();
  next.nodes[0].config.batch_table!.rows![0].input_node_ids = [
    "55555555-5555-4555-8555-555555555555",
    "55555555-5555-4555-8555-555555555555",
  ];
  const ops = canvasOperations(base, next);
  expect(
    ops.find(
      (o) =>
        o.type === "set" &&
        o.path?.at(-1) === "@33333333-3333-4333-8333-333333333333",
    )?.value,
  ).toBe("55555555-5555-4555-8555-555555555555");
  expect(applyOperations(base, ops).nodes).toEqual(next.nodes);
});
