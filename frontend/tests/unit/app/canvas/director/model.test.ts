import { describe, expect, it } from "vitest";
import {
  createDirectorConfig,
  createDirectorObject,
  directorTransformAt,
  putDirectorKeyframe,
  removeDirectorEntity,
  directorShotAt,
  directorBoneAt,
  directorPose,
  moveDirectorShot,
} from "@/app/canvas/director/model";

describe("director scene contract", () => {
  it("creates an editable scene with valid persistent identities and camera shot references", () => {
    const scene = createDirectorConfig();
    expect(scene.version).toBe(1);
    expect(scene.id).toMatch(/^[a-f\d-]{36}$/);
    expect(scene.objects).toEqual([]);
    expect(scene.cameras).toHaveLength(1);
    expect(scene.lights).toHaveLength(2);
    expect(scene.shots?.[0]).toMatchObject({
      camera_id: scene.cameras?.[0].id,
      duration: 5,
      fps: 24,
      camera_move: "static",
      shot_size: "full",
      prompt: "",
    });
    expect(scene.active_shot_id).toBe(scene.shots?.[0].id);
    expect(scene.cameras?.[0].keyframes).toEqual([]);
    expect(createDirectorConfig().id).not.toBe(scene.id);
  });
  it("builds backend-valid primitives and articulated actors", () => {
    expect(createDirectorObject("box")).toMatchObject({
      kind: "primitive",
      primitive: "box",
      transform: {
        position: [0, 0.5, 0],
        rotation: [0, 0, 0],
        scale: [1, 1, 1],
      },
      keyframes: [],
    });
    expect(createDirectorObject("character")).toMatchObject({
      kind: "actor",
      builtin_actor: "mannequin",
      pose: "stand",
    });
  });
});

describe("director timeline", () => {
  const base = { position: [0, 0, 0], rotation: [0, 0, 0], scale: [1, 1, 1] };
  const end = { position: [10, 0, 0], rotation: [0, 2, 0], scale: [2, 2, 2] };
  const frames: API.DirectorKeyframe[] = [
    { id: "a", time: 0, transform: base, easing: "linear" },
    { id: "b", time: 10, transform: end, easing: "linear" },
  ];
  it("interpolates transforms without mutating the saved source", () => {
    expect(directorTransformAt(base, frames, 5)).toEqual({
      position: [5, 0, 0],
      rotation: [0, 1, 0],
      scale: [1.5, 1.5, 1.5],
    });
    expect(directorTransformAt(base, frames, -1)).toEqual(base);
    expect(directorTransformAt(base, frames, 20)).toEqual(end);
    expect(base.position).toEqual([0, 0, 0]);
  });
  it("holds step frames and supports smooth interpolation", () => {
    expect(
      directorTransformAt(
        base,
        [{ ...frames[0], easing: "step" }, frames[1]],
        5,
      ),
    ).toEqual(base);
    expect(
      directorTransformAt(
        base,
        [{ ...frames[0], easing: "smooth" }, frames[1]],
        2.5,
      ).position?.[0],
    ).toBeCloseTo(1.5625);
    expect(directorTransformAt(base, [], 5)).toEqual(base);
  });
  it("replaces a keyframe at the same frame time and keeps stable identities", () => {
    const added = putDirectorKeyframe([], 1, end, "smooth");
    const replaced = putDirectorKeyframe(added, 1, base, "step");
    expect(replaced).toHaveLength(1);
    expect(replaced[0]).toMatchObject({
      id: added[0].id,
      time: 1,
      transform: base,
      easing: "step",
    });
    expect(added[0].transform).toEqual(end);
    expect(
      putDirectorKeyframe(replaced, 0, end, "linear").map((f) => f.time),
    ).toEqual([0, 1]);
  });
  it("resolves shot boundaries and clamps the end of playback", () => {
    const shots = [
      { id: "a", duration: 2 },
      { id: "b", duration: 3 },
    ];
    expect(directorShotAt(shots, 2)).toEqual({
      shot: shots[1],
      localTime: 0,
      index: 1,
    });
    expect(directorShotAt(shots, 10)).toEqual({
      shot: shots[1],
      localTime: 3,
      index: 1,
    });
  });
});

describe("safe director removal", () => {
  it("clears stable camera object references when deleting an object", () => {
    const config = createDirectorConfig();
    const object = createDirectorObject("box");
    config.objects = [object];
    config.cameras![0] = {
      ...config.cameras![0],
      follow_object_id: object.id,
      look_at_object_id: object.id,
      look_at_mode: "object",
    };
    const removed = removeDirectorEntity(config, "object", object.id!);
    expect(removed.objects).toEqual([]);
    expect(removed.cameras![0].follow_object_id).toBeUndefined();
    expect(removed.cameras![0].look_at_object_id).toBeUndefined();
    expect(removed.cameras![0].look_at_mode).toBe("coordinates");
    expect(config.objects).toHaveLength(1);
  });
  it("protects the last camera and camera references used by shots", () => {
    const config = createDirectorConfig();
    expect(removeDirectorEntity(config, "camera", config.cameras![0].id!)).toBe(
      config,
    );
    config.cameras!.push({ ...config.cameras![0], id: crypto.randomUUID() });
    const removed = removeDirectorEntity(
      config,
      "camera",
      config.cameras![0].id!,
    );
    expect(removed.shots![0].camera_id).toBe(removed.cameras![0].id);
  });
});

describe("bone quaternion playback", () => {
  it("uses normalized spherical interpolation and retains step pose", () => {
    const frames = [
      { time: 0, rotation: [0, 0, 0, 1], easing: "linear" },
      { time: 2, rotation: [0, 1, 0, 0] },
    ];
    const interpolated = directorBoneAt(frames, 1, [0, 0, 0, 1]);
    expect(interpolated[1]).toBeCloseTo(Math.SQRT1_2);
    expect(interpolated[3]).toBeCloseTo(Math.SQRT1_2);
    expect(Math.hypot(...interpolated)).toBeCloseTo(1);
    expect(
      directorBoneAt(
        [{ ...frames[0], easing: "step" }, frames[1]],
        1,
        [0, 0, 0, 1],
      ),
    ).toEqual([0, 0, 0, 1]);
    expect(directorBoneAt([], 10, [0, 0, 0, 1])).toEqual([0, 0, 0, 1]);
  });
});

describe("articulated mannequin poses", () => {
  it("poses supported limbs and animates opposite legs with scene time", () => {
    expect(directorPose("sit", 0)).toMatchObject({
      height: 0.55,
      joints: {
        leftUpperLeg: [-Math.PI / 2, 0, 0],
        rightUpperLeg: [-Math.PI / 2, 0, 0],
      },
    });
    expect(directorPose("t_pose", 0).joints.leftUpperArm[2]).toBe(-Math.PI / 2);
    const walk = directorPose("walk", 0.25);
    expect(walk.joints.leftUpperLeg[0]).not.toBe(0);
    expect(walk.joints.leftUpperLeg[0]).toBe(-walk.joints.rightUpperLeg[0]);
    expect(directorPose("wave", 0).joints.rightUpperArm[2]).toBeGreaterThan(2);
    expect(directorPose("bow", 0).joints.spine[0]).toBeGreaterThan(0);
  });
});

describe("shot ordering", () => {
  it("moves shots while retaining camera and active shot identities", () => {
    const config = createDirectorConfig();
    config.shots!.push({
      ...config.shots![0],
      id: crypto.randomUUID(),
      name: "镜头 2",
    });
    const first = config.shots![0].id!;
    const moved = moveDirectorShot(config, first, 1);
    expect(moved.shots![1].id).toBe(first);
    expect(moved.active_shot_id).toBe(first);
    expect(moved.shots![1].camera_id).toBe(config.cameras![0].id);
    expect(config.shots![0].id).toBe(first);
    expect(moveDirectorShot(config, first, -1)).toBe(config);
  });
});
