import { describe, expect, it } from "vitest";
import {
  applyOperations,
  transformOperationPair,
  type Operation,
} from "@/app/canvas/operations";
import type { Canvas } from "@/app/canvas/document";
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
