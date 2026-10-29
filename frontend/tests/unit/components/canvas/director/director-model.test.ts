import { expect, it } from "vitest";
import {
  emptyDirectorConfig,
  directorFingerprint,
  editDirector,
  readDirector,
  directorTransform,
} from "@/components/canvas/director/director-model";
import { sampleDirectorTransform } from "@/components/canvas/director/director-animation";
import {
  createDirectorPrimitive,
  disposeDirectorObject,
} from "@/components/canvas/director/director-scene";
import { Box3, Mesh, Quaternion, Euler } from "three";
const populated = () =>
  editDirector(emptyDirectorConfig(), { type: "add-object", primitive: "box" });
it("creates the exact persistent empty scene with stable camera/shot identities", () => {
  const scene = emptyDirectorConfig();
  expect(readDirector(JSON.parse(JSON.stringify(scene)))).toEqual(scene);
  expect(scene.objects).toEqual([]);
  expect(scene.shots[0].camera_id).toBe(scene.cameras[0].id);
  expect(scene.active_shot_id).toBe(scene.shots[0].id);
});
it("locks transformations/removal but allows visibility and unlock without changing geometry", () => {
  const scene = populated(),
    selection = { kind: "object" as const, id: scene.objects[0].id };
  const locked = editDirector(scene, {
    type: "entity",
    selection,
    patch: { locked: true },
  });
  expect(() =>
    editDirector(locked, {
      type: "entity",
      selection,
      patch: { transform: directorTransform([2, 1, 0]) },
    }),
  ).toThrow("锁定");
  expect(() => editDirector(locked, { type: "remove", selection })).toThrow(
    "锁定",
  );
  const hidden = editDirector(locked, {
    type: "entity",
    selection,
    patch: { visible: false },
  });
  expect(hidden.objects[0].visible).toBe(false);
  expect(hidden.objects[0].transform).toEqual(scene.objects[0].transform);
});
it("rejects removing an in-use camera and rebinds the active shot after deleting another shot", () => {
  const scene = emptyDirectorConfig();
  expect(() =>
    editDirector(scene, {
      type: "remove",
      selection: { kind: "camera", id: scene.cameras[0].id },
    }),
  ).toThrow("镜头使用");
  expect(() =>
    editDirector(scene, { type: "remove-shot", id: scene.active_shot_id }),
  ).toThrow("至少保留");
  const extra = editDirector(scene, {
    type: "add-shot",
    cameraID: scene.cameras[0].id,
  });
  expect(
    editDirector(extra, { type: "remove-shot", id: extra.active_shot_id })
      .active_shot_id,
  ).toBe(scene.active_shot_id);
});
it("object deletion clears camera follow and target identities atomically", () => {
  const scene = populated(),
    id = scene.objects[0].id;
  scene.cameras[0].follow_object_id = id;
  scene.cameras[0].look_at_object_id = id;
  scene.cameras[0].look_at_mode = "object";
  const next = editDirector(scene, {
    type: "remove",
    selection: { kind: "object", id },
  });
  expect(next.cameras[0].follow_object_id).toBeUndefined();
  expect(next.cameras[0].look_at_object_id).toBeUndefined();
  expect(next.cameras[0].look_at_mode).toBe("coordinates");
  expect(scene.objects).toHaveLength(1);
});
it("upserts same-time frames without duplicating identity or modifying the static base", () => {
  const scene = populated(),
    selection = { kind: "object" as const, id: scene.objects[0].id };
  const first = editDirector(scene, {
    type: "keyframe",
    selection,
    time: 2,
    transform: directorTransform([2, 0, 0]),
  });
  const changed = editDirector(first, {
    type: "keyframe",
    selection,
    time: 2,
    transform: directorTransform([4, 0, 0]),
    easing: "smooth",
  });
  expect(changed.objects[0].keyframes).toHaveLength(1);
  expect(changed.objects[0].keyframes[0].id).toBe(
    first.objects[0].keyframes[0].id,
  );
  expect(changed.objects[0].transform).toEqual(scene.objects[0].transform);
});
it.each(["linear", "smooth", "step"])(
  "evaluates %s position and quaternion rotation deterministically",
  (easing) => {
    const a = directorTransform(),
      b = directorTransform([10, 0, 0]);
    a.rotation[1] = (170 * Math.PI) / 180;
    b.rotation[1] = (-170 * Math.PI) / 180;
    const frames = [
      { id: crypto.randomUUID(), time: 0, transform: a, easing },
      { id: crypto.randomUUID(), time: 10, transform: b, easing: "linear" },
    ];
    const result = sampleDirectorTransform(a, frames, 5);
    expect(result.position[0]).toBe(easing === "step" ? 0 : 5);
    const q = new Quaternion().setFromEuler(
      new Euler(...(result.rotation as [number, number, number])),
    );
    const expected = new Quaternion().setFromEuler(
      new Euler(0, easing === "step" ? (170 * Math.PI) / 180 : Math.PI, 0),
    );
    expect(Math.abs(q.dot(expected))).toBeCloseTo(1, 6);
  },
);
it("rejects malformed geometry, unknown mode, missing assets and invalid fps", () => {
  const scene = populated();
  expect(() => readDirector({ ...scene, mode: "script" } as never)).toThrow();
  expect(() =>
    editDirector(scene, {
      type: "entity",
      selection: { kind: "object", id: scene.objects[0].id },
      patch: { transform: directorTransform([NaN, 0, 0]) },
    }),
  ).toThrow();
  expect(() =>
    readDirector({
      ...scene,
      objects: [{ ...scene.objects[0], kind: "model", asset_id: undefined }],
    }),
  ).toThrow();
  expect(() =>
    editDirector(scene, {
      type: "shot",
      id: scene.active_shot_id,
      patch: { fps: 120 },
    }),
  ).toThrow();
  for (const fps of [24, 25, 30, 50, 60])
    expect(
      editDirector(scene, {
        type: "shot",
        id: scene.active_shot_id,
        patch: { fps },
      }).shots[0].fps,
    ).toBe(fps);
});
it("builds real nonempty Three meshes, including an original geometric mannequin", () => {
  for (const primitive of ["box", "sphere", "cylinder", "plane", "character"]) {
    const scene = editDirector(emptyDirectorConfig(), {
      type: "add-object",
      primitive,
    });
    const group = createDirectorPrimitive(scene.objects[0]);
    expect(group.children.length).toBeGreaterThan(0);
    expect(group.children.every((child) => child instanceof Mesh)).toBe(true);
    expect(new Box3().setFromObject(group).isEmpty()).toBe(false);
    disposeDirectorObject(group);
  }
});

it("matches Go timestamp normalization without hiding changed capture identities", () => {
  const scene = emptyDirectorConfig();
  scene.shots[0].screenshots = [
    {
      id: crypto.randomUUID(),
      asset_id: crypto.randomUUID(),
      name: "拍摄",
      created_at: "2026-10-29T00:00:00.120Z",
    },
  ];
  const accepted = structuredClone(scene);
  accepted.shots[0].screenshots![0].created_at = "2026-10-29T00:00:00.12Z";
  expect(directorFingerprint(accepted)).toBe(directorFingerprint(scene));
  accepted.shots[0].screenshots![0].asset_id = crypto.randomUUID();
  expect(directorFingerprint(accepted)).not.toBe(directorFingerprint(scene));
});

it("matches Go omitempty normalization after removing the last shot image or animation", () => {
  const scene = populated();
  scene.objects[0].motion_clips = [];
  scene.objects[0].bone_tracks = [];
  scene.objects[0].bone_overrides = {};
  scene.shots[0].screenshots = [];
  const accepted = structuredClone(scene);
  delete accepted.objects[0].motion_clips;
  delete accepted.objects[0].bone_tracks;
  delete accepted.objects[0].bone_overrides;
  delete accepted.shots[0].screenshots;
  expect(directorFingerprint(scene)).toBe(directorFingerprint(accepted));
});
