import { describe, it, expect } from "vitest";
import {
  Collaboration,
  type Change,
  type Envelope,
} from "@/app/canvas/collaboration";
import type { Canvas } from "@/app/canvas/document";
import type { Operation } from "@/app/canvas/operations";
const doc = (): Canvas => ({
  id: "d",
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
const text = (edit: Operation["text"]): Operation => ({
  type: "text",
  node_id: "n",
  path: ["config", "text"],
  text: edit,
});
const change = (
  revision: number,
  actor_id: string,
  op_id: string,
  operations: Operation[],
  value: string,
): Change => ({
  revision,
  actor_id,
  op_id,
  operations,
  effects: [
    {
      entity: "node",
      id: "n",
      path: ["config", "text"],
      index: 0,
      after: value,
      after_exists: true,
    },
  ],
});
describe("authoritative B, immutable P and causal Q", () => {
  it("keeps P exact and waits for ACK plus own continuous changes", () => {
    const sent: Envelope[] = [];
    const c = new Collaboration(
      doc(),
      () => {},
      (e) => sent.push(e),
    );
    c.connection(true, true, "me");
    c.enqueue([text([{ insert: [{ type: "text", text: "X" }] }])], "p");
    const original = JSON.stringify(sent[0]);
    c.enqueue(
      [text([{ retain: 1 }, { insert: [{ type: "text", text: "Y" }] }])],
      "q",
    );
    c.changes([
      change(2, "other", "r", [text([{ retain: 1 }, { delete: 1 }])], "A"),
    ]);
    expect(c.document.nodes[0].config.text).toBe("XYA");
    expect(JSON.stringify(sent[0])).toBe(original);
    c.receipt({ status: "accepted", op_id: "p", revision: 3 });
    expect(c.base.revision).toBe(2);
    expect(sent).toHaveLength(1);
    const own = change(
      3,
      "me",
      "p",
      [text([{ insert: [{ type: "text", text: "X" }] }])],
      "XA",
    );
    c.changes([own]);
    expect(sent).toHaveLength(2);
    expect(sent[1].body.base_revision).toBe(3);
    expect(c.document.nodes[0].config.text).toBe("XYA");
    c.changes([own]);
    expect(sent).toHaveLength(2);
  });
  it("keeps an invalid predicted P resolvable and separates independent Q", () => {
    const sent: Envelope[] = [];
    const c = new Collaboration(
      doc(),
      () => {},
      (e) => sent.push(e),
    );
    c.connection(true, true, "me");
    c.enqueue(
      [{ type: "set", node_id: "n", path: ["title"], value: "mine" }],
      "p",
    );
    c.enqueue(
      [{ type: "set", node_id: "n", path: ["title"], value: "dependent" }],
      "q",
    );
    c.changes([
      {
        revision: 2,
        actor_id: "other",
        op_id: "r",
        operations: [{ type: "delete_node", node_id: "n" }],
        effects: [
          { entity: "node", id: "n", path: [], index: 0, after_exists: false },
        ],
      },
    ]);
    expect(c.pending?.key).toBe("p");
    expect(c.recovery).toHaveLength(1);
    c.receipt({ status: "rejected", op_id: "p", code: "target_deleted" });
    expect(c.pending).toBeUndefined();
    expect(c.recovery).toHaveLength(2);
    expect(sent).toHaveLength(1);
  });
});
