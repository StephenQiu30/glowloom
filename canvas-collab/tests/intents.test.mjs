import assert from "node:assert/strict";
import test from "node:test";
import {
  trackIntents,
  acknowledgeIntents,
  readIntent,
  applyReadableIntents,
} from "../src/intents.mjs";

const snapshot = () => ({
  title: "canvas",
  graph: {
    schemaVersion: 2,
    connections: [],
    nodes: [
      {
        id: "n",
        type: "text",
        nodeTypeVersion: 1,
        title: "node",
        position: { x: 0, y: 0 },
        width: 200,
        height: 100,
        parameters: {
          mode: "GENERATE",
          modelSelection: {
            connectionId: "9007199254740993",
            modelId: "model",
          },
        },
      },
    ],
  },
});

test("semantic recovery keeps coupled placement/configuration atomic and preserves only local changed fields", () => {
  const before = snapshot(),
    after = structuredClone(before);
  after.graph.nodes[0].position.x = 20;
  after.graph.nodes[0].parameters.modelSelection.modelId = "another";
  const intents = trackIntents([], before, after, 1);
  assert.deepEqual(intents.map((intent) => intent.field).sort(), [
    "parameters",
    "placement",
  ]);
  const current = snapshot();
  current.graph.nodes[0].title = "accepted remote title";
  const readable = applyReadableIntents(current, intents);
  assert.equal(readable.graph.nodes[0].position.x, 20);
  assert.equal(readable.graph.nodes[0].title, "accepted remote title");
  assert.equal(
    readable.graph.nodes[0].parameters.modelSelection.connectionId,
    "9007199254740993",
  );
  assert.deepEqual(readIntent(readable, intents[0]), intents[0].after);
});

test("an ACK removes its exact semantic prefix while later edits retain their desired values and a rebased before image", () => {
  const base = snapshot(),
    first = snapshot();
  first.graph.nodes[0].title = "first";
  const frozen = trackIntents([], base, first, 1);
  const second = structuredClone(first);
  second.graph.nodes[0].title = "second";
  const pending = trackIntents(frozen, first, second, 2);
  const result = acknowledgeIntents(pending, frozen, first);
  assert.equal(result.unapplied.length, 0);
  assert.equal(result.pending.length, 1);
  assert.deepEqual(result.pending[0].before, { exists: true, value: "first" });
  assert.deepEqual(result.pending[0].after, { exists: true, value: "second" });
  const changedByPeer = snapshot();
  changedByPeer.graph.nodes[0].title = "peer";
  const lost = acknowledgeIntents(frozen, frozen, changedByPeer);
  assert.equal(lost.unapplied.length, 1);
  assert.equal(
    applyReadableIntents(changedByPeer, lost.unapplied).graph.nodes[0].title,
    "first",
  );
});

test("creation plus edits coalesce into one recoverable entity and an uncommitted create/delete cancels its intent", () => {
  const empty = snapshot();
  empty.graph.nodes = [];
  const created = snapshot(),
    edited = snapshot();
  edited.graph.nodes[0].title = "last local value";
  const first = trackIntents([], empty, created, 1);
  const last = trackIntents(first, created, edited, 2);
  assert.equal(last.length, 1);
  assert.equal(last[0].field, "entity");
  assert.equal(last[0].after.value.title, "last local value");
  assert.deepEqual(trackIntents(last, edited, empty, 3), []);
});

test("collaborative rich-text merge and edits to an accepted new entity do not count as a lost single-value intent", () => {
  const before = snapshot(),
    local = snapshot();
  local.graph.nodes[0].textDocument = {
    schemaVersion: 1,
    content: {
      type: "doc",
      content: [
        {
          type: "paragraph",
          content: [{ type: "text", text: "local" }],
        },
      ],
    },
  };
  const merged = structuredClone(local);
  merged.graph.nodes[0].textDocument.content.content[0].content.push({
    type: "text",
    text: "peer",
  });
  const text = trackIntents([], before, local, 1);
  assert.equal(acknowledgeIntents(text, text, merged).unapplied.length, 0);
  const empty = snapshot();
  empty.graph.nodes = [];
  const creation = trackIntents([], empty, local, 1);
  merged.graph.nodes[0].title = "peer edited the now-shared entity";
  assert.equal(
    acknowledgeIntents(creation, creation, merged).unapplied.length,
    0,
  );
});
