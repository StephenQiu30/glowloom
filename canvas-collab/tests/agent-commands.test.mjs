import assert from "node:assert/strict";
import test from "node:test";
import { randomUUID } from "node:crypto";
import * as Y from "yjs";
import { computeCandidate } from "../dist/service-runtime.js";
import { readCanvasDocument } from "../dist/model.js";

function baseline() {
  const document = {
    id: randomUUID(),
    project_id: randomUUID(),
    name: "创作画布",
    schema_version: 1,
    operation_version: 1,
    revision: "1",
    permissions: { read: true, write: true },
    nodes: [],
    edges: [],
  };
  return {
    documentId: document.id,
    epoch: randomUUID(),
    writerTerm: "1",
    commitSeq: "0",
    document,
  };
}
test("formal semantic command preserves existing Yjs containers and deterministically prepares structured storyboard/timeline deltas", async () => {
  const request = baseline(),
    initial = await computeCandidate("initialize", request),
    textId = randomUUID(),
    timelineId = randomUUID(),
    trackId = randomUUID();
  const projection = {
    ...structuredClone(request.document),
    nodes: [
      {
        id: textId,
        node_type: "text",
        node_action: "resource",
        title: "分镜 1",
        x: 0,
        y: 0,
        parent_id: null,
        z_index: 1,
        content_version: "1",
        config: {
          text: "雨夜。主角走入街道。",
          format: "plain",
          format_version: 1,
        },
      },
      {
        id: timelineId,
        node_type: "timeline",
        node_action: "resource",
        title: "主时间线",
        x: 400,
        y: 0,
        parent_id: null,
        z_index: 2,
        content_version: "1",
        config: {
          timeline: {
            version: 1,
            aspect_ratio: "16:9",
            fps: 25,
            tracks: [
              {
                id: trackId,
                kind: "text",
                label: "字幕",
                locked: false,
                visible: true,
                muted: false,
              },
            ],
            clips: [],
            subtitle_style: {
              font_size: 32,
              color: "#ffffff",
              position: "bottom",
            },
            snapping: true,
          },
        },
      },
    ],
  };
  const command = {
    ...request,
    state: initial.state,
    updateId: randomUUID(),
    projection,
  };
  const prepared = await computeCandidate("commands", command);
  assert.deepEqual(await computeCandidate("commands", command), prepared);
  assert.equal(prepared.projection.nodes.length, 2);
  assert.equal(
    prepared.projection.nodes.find((n) => n.id === timelineId).config.timeline
      .fps,
    25,
  );
  const doc = new Y.Doc();
  try {
    Y.applyUpdate(doc, Buffer.from(initial.state, "base64"));
    const previous = Y.decodeStateVector(Y.encodeStateVector(doc));
    Y.applyUpdate(doc, Buffer.from(prepared.appliedUpdate, "base64"));
    for (const [id, clock] of previous)
      assert.ok(Y.decodeStateVector(Y.encodeStateVector(doc)).get(id) >= clock);
    assert.equal(readCanvasDocument(doc).graph.nodes.length, 2);
  } finally {
    doc.destroy();
  }
  const second = {
    ...request,
    state: prepared.state,
    document: {
      ...prepared.projection,
      nodes: [...prepared.projection.nodes].sort((a, b) =>
        a.id.localeCompare(b.id),
      ),
    },
    commitSeq: "1",
    updateId: randomUUID(),
    projection: structuredClone(prepared.projection),
  };
  second.projection.nodes.find(
    (n) => n.id === textId,
  ).config.rich_text.content = {
    type: "doc",
    content: [
      {
        type: "paragraph",
        content: [{ type: "text", text: "雨夜。主角停下。" }],
      },
    ],
  };
  const edited = await computeCandidate("commands", second);
  assert.equal(
    edited.projection.nodes.find((n) => n.id === textId).config.rich_text
      .content.content[0].content[0].text,
    "雨夜。主角停下。",
  );
  await assert.rejects(
    () =>
      computeCandidate("commands", {
        ...command,
        projection: { ...projection, id: randomUUID() },
      }),
    /INVALID_COMMAND_PROJECTION/,
  );
  await assert.rejects(
    () =>
      computeCandidate("commands", {
        ...command,
        document: { ...request.document, name: "stale mismatch" },
        projection: { ...projection, name: "stale mismatch" },
      }),
    /PROJECTION_DRIFT/,
  );
});
