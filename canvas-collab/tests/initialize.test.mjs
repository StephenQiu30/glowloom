import assert from "node:assert/strict";
import test from "node:test";
import * as Y from "yjs";
import { initializeDocument } from "../src/initialize.mjs";
import { readCanvasDocument } from "../src/model.mjs";

test("one-time initializer preserves authoritative long identities and returns validated binary rather than mutating a room", () => {
  const graphJson =
    '{"schemaVersion":2,"nodes":[{"id":"node","type":"image","nodeTypeVersion":1,"title":"图片","position":{"x":0,"y":0},"width":320,"height":200,"zIndex":0,"parameters":{"mode":"GENERATE","modelSelection":{"connectionId":9223372036854775806,"modelId":"model"}},"fileReferences":[]}],"connections":[]}';
  const initial = initializeDocument({ title: "画布", graphJson });
  const doc = new Y.Doc();
  try {
    Y.applyUpdate(doc, Buffer.from(initial.update, "base64"));
    assert.equal(
      readCanvasDocument(doc).graph.nodes[0].parameters.modelSelection
        .connectionId,
      "9223372036854775806",
    );
    assert.equal(initial.modelVersion, 1);
    assert.match(initial.stateHash, /^[a-f0-9]{64}$/);
    assert.throws(
      () =>
        initializeDocument({
          title: "画布",
          graphJson: graphJson.replace('"x":0', '"x":9007199254740993'),
        }),
      /UNREPRESENTABLE_NUMBER/,
    );
  } finally {
    doc.destroy();
  }
});

test("registered schema normalization preserves adjacent text and creates a new representation without rewriting the source", () => {
  const graphJson = JSON.stringify({
    schemaVersion: 2,
    nodes: [
      {
        id: "n",
        type: "text",
        nodeTypeVersion: 1,
        title: "文字",
        position: { x: 0, y: 0 },
        width: 320,
        height: 200,
        zIndex: 0,
        textDocument: {
          schemaVersion: 1,
          content: {
            type: "doc",
            content: [
              {
                type: "paragraph",
                content: [
                  { type: "text", text: "甲 " },
                  { type: "text", text: " " },
                ],
              },
            ],
          },
        },
      },
    ],
    connections: [],
  });
  const initial = initializeDocument({ title: "画布", graphJson });
  const projected = JSON.parse(initial.graphJson);
  assert.equal(
    projected.nodes[0].textDocument.content.content[0].content[0].text,
    "甲  ",
  );
  assert.equal(
    JSON.parse(graphJson).nodes[0].textDocument.content.content[0].content
      .length,
    2,
  );
  assert.notEqual(initial.sourceGraphHash, initial.graphHash);
});
