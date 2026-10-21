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

it("holds remote changes through IME and transforms committed input before sending", () => {
  const sent: Envelope[] = [];
  const c = new Collaboration(
    doc(),
    () => {},
    (e) => sent.push(e),
  );
  c.connection(true, true, "me");
  c.composition(true);
  c.enqueue([text([{ insert: [{ type: "text", text: "中😀" }] }])], "ime");
  c.changes([
    change(2, "other", "delete", [text([{ retain: 1 }, { delete: 1 }])], "A"),
  ]);
  expect(c.base.revision).toBe(1);
  expect(sent).toHaveLength(0);
  expect(c.document.nodes[0].config.text).toBe("中😀AB");
  c.composition(false);
  expect(sent).toHaveLength(1);
  expect(sent[0].body.base_revision).toBe(2);
  expect(c.document.nodes[0].config.text).toBe("中😀A");
});

it("uses new identities for server contribution Undo and Redo", () => {
  const sent: Envelope[] = [];
  const c = new Collaboration(
    doc(),
    () => {},
    (e) => sent.push(e),
  );
  c.connection(true, true, "me");
  c.enqueue(
    [{ type: "set", node_id: "n", path: ["title"], value: "mine" }],
    "own",
  );
  const own: Change = {
    revision: 2,
    actor_id: "me",
    op_id: "own",
    operations: sent[0].body.operations,
    effects: [
      {
        entity: "node",
        id: "n",
        path: ["title"],
        index: 0,
        after: "mine",
        after_exists: true,
      },
    ],
  };
  c.changes([own]);
  c.receipt({ status: "accepted", op_id: "own", revision: 2 });
  expect(c.canUndo).toBe(true);
  expect(c.history("undo")).toBe(true);
  const undo = sent[1];
  expect(undo.key).not.toBe("own");
  expect(undo.body.undo_of).toBe("own");
  expect(undo.body.operations).toEqual([]);
  c.changes([
    {
      revision: 3,
      actor_id: "me",
      op_id: undo.key,
      operations: [{ type: "set", node_id: "n", path: ["title"], value: "N" }],
      effects: [
        {
          entity: "node",
          id: "n",
          path: ["title"],
          index: 0,
          after: "N",
          after_exists: true,
        },
      ],
    },
  ]);
  c.receipt({ status: "accepted", op_id: undo.key, revision: 3 });
  expect(c.canRedo).toBe(true);
  expect(c.history("redo")).toBe(true);
  expect(sent[2].body.undo_of).toBe(undo.key);
  expect(sent[2].key).not.toBe(undo.key);
});

it("requires a terminal original receipt before a verified history-gap snapshot", () => {
  const sent: Envelope[] = [];
  const c = new Collaboration(
    doc(),
    () => {},
    (e) => sent.push(e),
  );
  c.connection(true, true, "me");
  c.enqueue([text([{ insert: [{ type: "text", text: "X" }] }])], "p");
  const latest = doc();
  latest.revision = 200;
  latest.nodes[0].config.text = "XAB!";
  expect(() => c.verifiedResync(latest)).toThrow("request_unconfirmed");
  c.receipt({ status: "accepted", op_id: "p", revision: 2 });
  c.verifiedResync(latest);
  expect(c.pending).toBeUndefined();
  expect(c.base.revision).toBe(200);
  expect(c.document.nodes[0].config.text).toBe("XAB!");
});

it("sends a completed transaction immediately and queues only behind its receipt", () => {
  const sent: Envelope[] = [];
  const c = new Collaboration(
    doc(),
    () => {},
    (e) => sent.push(e),
  );
  c.connection(true, true, "me");
  c.enqueue(
    [{ type: "set", node_id: "n", path: ["title"], value: "first" }],
    "first",
  );
  expect(sent).toHaveLength(1);
  c.enqueue(
    [{ type: "set", node_id: "n", path: ["title"], value: "second" }],
    "second",
  );
  expect(sent).toHaveLength(1);
  expect(c.document.nodes[0].title).toBe("second");
  c.dispose();
});

it("retains the active IME continuation when the connection drops", () => {
  const sent: Envelope[] = [];
  const c = new Collaboration(
    doc(),
    () => {},
    (e) => sent.push(e),
  );
  c.connection(true, true, "me");
  c.composition(true);
  c.connection(false, false, "me");
  c.enqueue([text([{ insert: [{ type: "text", text: "中😀" }] }])], "ime");
  c.composition(false);
  expect(sent).toHaveLength(0);
  expect(c.document.nodes[0].config.text).toBe("中😀AB");
  expect(() =>
    c.enqueue([text([{ insert: [{ type: "text", text: "new" }] }])]),
  ).toThrow("connection_read_only");
  c.connection(true, true, "me");
  expect(sent).toHaveLength(1);
  expect(sent[0].key).toBe("ime");
});

it("keeps causal Q when Go canonical JSON changes property order only", async () => {
  const { emptyPromptDraft } =
    await import("@/components/generation/prompt-document");
  const d = doc();
  const draft = emptyPromptDraft();
  d.nodes[0].node_type = "image_generation";
  d.nodes[0].node_action = "generate";
  d.nodes[0].config = {
    draft: {
      prompt: "",
      negative_prompt: "",
      aspect_ratio: "",
      candidate_count: 0,
      composer: draft,
    },
  };
  const sent: Envelope[] = [];
  const c = new Collaboration(
    d,
    () => {},
    (e) => sent.push(e),
  );
  c.connection(true, true, "me");
  const edit = (value: string, offset: number): Operation => ({
    type: "text",
    node_id: "n",
    path: ["config", "draft", "composer", "document"],
    text: [
      ...(offset ? [{ retain: offset }] : []),
      { insert: [{ type: "text", text: value }] },
    ],
  });
  c.enqueue([edit("中", 0)], "p");
  c.enqueue([edit("😀", 1)], "q");
  c.receipt({ status: "accepted", op_id: "p", revision: 2 });
  c.changes([
    {
      revision: 2,
      actor_id: "me",
      op_id: "p",
      operations: [edit("中", 0)],
      effects: [
        {
          entity: "node",
          id: "n",
          path: ["config", "draft", "composer", "document"],
          index: 0,
          after_exists: true,
          after: {
            content: [
              { content: [{ text: "中", type: "text" }], type: "paragraph" },
            ],
            type: "doc",
          },
        },
      ],
    },
  ]);
  expect(c.recovery).toEqual([]);
  expect(sent).toHaveLength(2);
  expect(
    c.document.nodes[0].config.draft?.composer?.document.content[0]
      .content?.[0],
  ).toEqual({ type: "text", text: "中😀" });
});

it("offline editing queues behind immutable P and reconnect ACK cannot overwrite Q", () => {
  const sent: Envelope[] = [];
  const c = new Collaboration(
    doc(),
    () => {},
    (e) => sent.push(e),
  );
  c.connection(true, true, "me");
  c.enqueue(
    [
      {
        type: "set",
        node_id: "n",
        path: ["position"],
        value: { x: 10, y: 20 },
      },
    ],
    "p",
  );
  const frozen = JSON.stringify(sent[0]);
  c.connection(false, true, "me");
  c.enqueue(
    [
      {
        type: "set",
        node_id: "n",
        path: ["position"],
        value: { x: 30, y: 40 },
      },
    ],
    "q",
  );
  expect(c.document.nodes[0]).toMatchObject({ x: 30, y: 40 });
  expect(sent).toHaveLength(1);
  c.connection(true, true, "me");
  c.receipt({ status: "accepted", op_id: "p", revision: 2 });
  c.changes([
    {
      revision: 2,
      actor_id: "me",
      op_id: "p",
      operations: sent[0].body.operations,
      effects: [
        {
          entity: "node",
          id: "n",
          path: ["position"],
          index: 0,
          after: { x: 10, y: 20 },
          after_exists: true,
        },
      ],
    },
  ]);
  expect(JSON.stringify(sent[0])).toBe(frozen);
  expect(sent).toHaveLength(2);
  expect(sent[1].body.base_revision).toBe(2);
  expect(c.document.nodes[0]).toMatchObject({ x: 30, y: 40 });
  c.connection(true, false, "me");
  expect(() =>
    c.enqueue([
      { type: "set", node_id: "n", path: ["title"], value: "denied" },
    ]),
  ).toThrow("connection_read_only");
  c.dispose();
});

it("offline flush cannot claim a saved source for generation", async () => {
  const c = new Collaboration(
    doc(),
    () => {},
    () => {},
  );
  c.connection(true, true, "me");
  c.connection(false, true, "me");
  await expect(c.flush()).rejects.toThrow("request_unconfirmed");
  c.enqueue([{ type: "set", node_id: "n", path: ["title"], value: "offline" }]);
  await expect(c.flush()).rejects.toThrow("request_unconfirmed");
  expect(c.document.nodes[0].title).toBe("offline");
  c.dispose();
});

it("bounds disconnected intent backlog without replacing accepted local intent", () => {
  const sent: Envelope[] = [];
  const c = new Collaboration(
    doc(),
    () => {},
    (e) => sent.push(e),
  );
  c.connection(true, true, "me");
  c.connection(false, true, "me");
  const first = "a".repeat(600_000);
  c.enqueue([{ type: "set", node_id: "n", path: ["title"], value: first }]);
  expect(() =>
    c.enqueue([
      {
        type: "set",
        node_id: "n",
        path: ["title"],
        value: "b".repeat(600_000),
      },
    ]),
  ).toThrow("canvas_request_too_large");
  expect(c.document.nodes[0].title).toBe(first);
  expect(sent).toHaveLength(0);
  c.connection(true, true, "me");
  expect(sent).toHaveLength(1);
  expect(sent[0].body.operations).toHaveLength(1);
  c.dispose();
});
