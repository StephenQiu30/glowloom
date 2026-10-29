import assert from "node:assert/strict";
import { randomUUID, createHash } from "node:crypto";
import test from "node:test";
import * as Y from "yjs";
import { serviceSnapshot } from "../src/service-document.mjs";
import {
  createCanvasDocument,
  readCanvasDocument,
  applyCanvasChanges,
} from "../src/model.mjs";
import { computeCandidate } from "../src/service-runtime.mjs";

const transform = () => ({
  position: [0, 0, 0],
  rotation: [0, 0, 0],
  scale: [1, 1, 1],
});
function source(name) {
  const duration = 1.3333333730697632;
  const tracks = Array.from({ length: 65 }, (_, bone) => ({
    bone: `Bone${bone}`,
    property: "quaternion",
    times: Array.from({ length: 20 }, (_, index) => (duration * index) / 19),
    values: Array.from({ length: 20 }, (_, index) => {
      const angle = (bone + index + 1) / 91;
      return [
        Math.sin(angle) / Math.sqrt(3),
        Math.sin(angle) / Math.sqrt(3),
        Math.sin(angle) / Math.sqrt(3),
        Math.cos(angle),
      ];
    }).flat(),
  }));
  return {
    id: randomUUID(),
    name,
    kind: "import",
    duration,
    tracks,
    rest: Object.fromEntries(tracks.map(({ bone }) => [bone, transform()])),
  };
}
function scene(object) {
  const camera = {
    id: randomUUID(),
    name: "Camera",
    transform: transform(),
    target: [0, 1, 0],
    focal_length: 35,
    fov: 50,
    aperture: 2.8,
    focus_distance: 8,
    near: 0.1,
    far: 1000,
    look_at_mode: "coordinates",
    keyframes: [],
    locked: false,
  };
  const shot = {
    id: randomUUID(),
    name: "Shot",
    camera_id: camera.id,
    duration: 20,
    fps: 30,
    shot_size: "medium",
    camera_move: "static",
    prompt: "",
  };
  return {
    background: "#171717",
    environment_intensity: 1,
    grid_visible: true,
    objects: [object],
    cameras: [camera],
    lights: [],
    shots: [shot],
    active_shot_id: shot.id,
  };
}

test("a bounded director action source survives native Yjs update, server preparation and reload", async () => {
  const savedPose = source("Saved"),
    imported = source("Imported");
  const existingClip = {
    id: randomUUID(),
    name: "Walk",
    source_animation: "Walk_Loop",
    start: 0,
    duration: imported.duration,
    playback_rate: 1,
    loop: false,
  };
  const actor = {
    id: randomUUID(),
    name: "Actor",
    kind: "actor",
    builtin_actor: "male_standard",
    transform: transform(),
    color: "#ffffff",
    visible: true,
    locked: false,
    cast_shadow: true,
    receive_shadow: true,
    keyframes: [],
    motion_library: [savedPose],
  };
  const director = {
    id: randomUUID(),
    version: 2,
    title: "Scene",
    mode: "dynamic",
    workspaces: {
      static: scene(actor),
      dynamic: scene({
        ...structuredClone(actor),
        motion_library: [savedPose, imported],
        motion_clips: [existingClip],
        active_motion_clip_id: existingClip.id,
      }),
    },
  };
  const dto = {
    id: randomUUID(),
    project_id: randomUUID(),
    name: "Director boundary",
    revision: "1",
    schema_version: 1,
    operation_version: 1,
    permissions: { read: true, write: true },
    scope: { type: "independent" },
    nodes: [
      {
        id: randomUUID(),
        node_type: "director",
        node_action: "manual",
        title: "Director",
        x: 0,
        y: 0,
        width: 500,
        height: 300,
        z_index: 1,
        content_version: "1",
        config: { director },
      },
    ],
    edges: [],
  };
  const doc = createCanvasDocument(serviceSnapshot(dto));
  const reloaded = new Y.Doc();
  try {
    const state = Y.encodeStateAsUpdate(doc),
      vector = Y.encodeStateVector(doc);
    const before = readCanvasDocument(doc),
      after = structuredClone(before);
    const next = after.graph.nodes[0].native.config.director;
    const object = next.workspaces.dynamic.objects[0];
    const clip = {
      ...existingClip,
      id: randomUUID(),
      name: "Imported",
      source_animation: `user:${imported.id}`,
      start: 1.3333333333333333,
    };
    object.motion_clips.push(clip);
    object.active_motion_clip_id = clip.id;
    const size = Buffer.byteLength(JSON.stringify(next));
    assert.ok(size > 200_000 && size <= 512 * 1024);
    applyCanvasChanges(doc, before, after);
    const update = Y.encodeStateAsUpdate(doc, vector);
    assert.ok(update.byteLength < 768 * 1024);
    assert.equal(Y.decodeUpdate(update).structs.length, 1);
    const result = await computeCandidate("prepare", {
      documentId: dto.id,
      epoch: randomUUID(),
      writerTerm: "1",
      commitSeq: "0",
      document: dto,
      state: Buffer.from(state).toString("base64"),
      update: Buffer.from(update).toString("base64"),
      updateId: randomUUID(),
      payloadHash: createHash("sha256").update(update).digest("hex"),
    });
    assert.deepEqual(result.projection.nodes[0].config.director, next);
    Y.applyUpdate(reloaded, Buffer.from(result.state, "base64"));
    assert.deepEqual(
      readCanvasDocument(reloaded).graph.nodes[0].native.config.director,
      next,
    );
    assert.equal(
      result.projection.nodes[0].config.director.workspaces.dynamic.objects[0]
        .motion_clips[1].source_animation,
      `user:${imported.id}`,
    );
  } finally {
    doc.destroy();
    reloaded.destroy();
  }
});
