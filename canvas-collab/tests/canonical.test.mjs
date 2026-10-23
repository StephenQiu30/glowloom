import assert from "node:assert/strict";
import test from "node:test";
import {
  canonicalJson,
  canonicalGraphJson,
  parseStoredGraph,
} from "../src/canonical.mjs";

test("business hashes use Unicode code points, decimal expansion and lexicographic numeric keys", () => {
  assert.equal(
    canonicalJson({ 2: 1e-7, 10: 1e21, "😀": -0, "\ue000": 2 }),
    '{"10":1000000000000000000000,"2":0.0000001,"":2,"😀":0}',
  );
  assert.equal(
    canonicalJson([1.25e-7, true, null, "\n"]),
    '[0.000000125,true,null,"\\n"]',
  );
  assert.throws(
    () => canonicalJson({ value: Infinity }),
    /INVALID_JSON_NUMBER/,
  );
});

test("Java long model identities remain exact strings in Yjs and exact integers in business hashes", () => {
  const raw =
    '{"connections":[],"nodes":[{"id":"n","parameters":{"modelSelection":{"connectionId":9223372036854775806,"modelId":"m"}}}],"schemaVersion":2}';
  const graph = parseStoredGraph(raw);
  assert.equal(
    graph.nodes[0].parameters.modelSelection.connectionId,
    "9223372036854775806",
  );
  assert.equal(canonicalGraphJson(graph), raw);
  assert.throws(
    () =>
      parseStoredGraph(
        '{"nodes":[],"connections":[],"schemaVersion":2,"other":9007199254740993}',
      ),
    /UNREPRESENTABLE_NUMBER/,
  );
  assert.throws(
    () =>
      canonicalGraphJson({
        nodes: [
          {
            parameters: {
              modelSelection: { connectionId: 9223372036854775806 },
            },
          },
        ],
      }),
    /INVALID_MODEL_IDENTITY/,
  );
});
