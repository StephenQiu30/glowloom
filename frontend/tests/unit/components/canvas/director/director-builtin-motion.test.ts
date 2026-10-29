import { DIRECTOR_BUILTIN_MOTION_PRESETS } from "@/components/canvas/director/director-motion-catalog";
import { readFileSync } from "node:fs";
import {
  AnimationClip,
  AnimationMixer,
  Bone,
  Group,
  Quaternion,
  Vector3,
} from "three";
import { GLTFLoader } from "three/addons/loaders/GLTFLoader.js";
import { reshapeDirectorQuaterniusActor } from "@/components/canvas/director/director-actor-body";
import { afterEach, beforeEach, expect, it, vi } from "vitest";

const sourceRest = new Quaternion().setFromAxisAngle(new Vector3(1, 0, 0), 0.4);
const motionDelta = new Quaternion().setFromAxisAngle(
  new Vector3(0, 0, 1),
  0.6,
);
const fixture = () => ({
  version: 1,
  rest_rotations: { arm: sourceRest.toArray() },
  motions: DIRECTOR_BUILTIN_MOTION_PRESETS.map(({ name }) => ({
    name,
    duration: 1,
    tracks: [
      {
        bone: "arm",
        times: [0, 1],
        values: [
          ...sourceRest.toArray(),
          ...sourceRest.clone().multiply(motionDelta).toArray(),
        ],
      },
    ],
  })),
});
function target() {
  const root = new Group(),
    arm = new Bone();
  arm.name = "arm";
  arm.quaternion.setFromAxisAngle(new Vector3(0, 1, 0), 0.7);
  arm.position.set(0.1, 0.45, 0.2);
  arm.scale.set(0.8, 1.1, 0.9);
  root.add(arm);
  return { root, arm };
}
beforeEach(() => {
  vi.resetModules();
  vi.stubGlobal(
    "fetch",
    vi.fn().mockResolvedValue(new Response(JSON.stringify(fixture()))),
  );
});
afterEach(() => {
  vi.unstubAllGlobals();
});

it("retargets source rotation deltas onto target rest while preserving positions and body scale", async () => {
  const { attachBuiltinDirectorMotions } =
    await import("@/components/canvas/director/director-builtin-motion");
  const { root, arm } = target();
  const rest = arm.quaternion.clone(),
    position = arm.position.clone(),
    scale = arm.scale.clone();
  await attachBuiltinDirectorMotions(root, new AbortController().signal);
  const clip = root.animations.find((item) => item.name === "Walk_Loop")!;
  expect(clip.tracks.map((track) => track.name)).toEqual([
    `${arm.uuid}.quaternion`,
  ]);
  const mixer = new AnimationMixer(root);
  mixer.clipAction(clip).play();
  mixer.setTime(0);
  expect(arm.quaternion.angleTo(rest)).toBeLessThan(0.001);
  mixer.setTime(0.5);
  const expected = rest
    .clone()
    .multiply(new Quaternion().slerp(motionDelta, 0.5));
  expect(arm.quaternion.angleTo(expected)).toBeLessThan(0.001);
  expect(arm.position.toArray()).toEqual(position.toArray());
  expect(arm.scale.toArray()).toEqual(scale.toArray());
});

it("preserves imported clips and does not append presets twice", async () => {
  const { attachBuiltinDirectorMotions } =
    await import("@/components/canvas/director/director-builtin-motion");
  const { root } = target();
  const imported = new AnimationClip("Custom", 1, []);
  root.animations = [imported];
  await attachBuiltinDirectorMotions(root, new AbortController().signal);
  await attachBuiltinDirectorMotions(root, new AbortController().signal);
  expect(root.animations.map((clip) => clip.name)).toEqual([
    "Custom",
    ...DIRECTOR_BUILTIN_MOTION_PRESETS.map((item) => item.name),
  ]);
  expect(root.animations[0]).toBe(imported);
  expect(fetch).toHaveBeenCalledOnce();
});

it("aborts a pending fetch and never adopts a late result", async () => {
  let resolve: (value: Response) => void = () => {};
  vi.mocked(fetch).mockImplementation(
    () =>
      new Promise((done) => {
        resolve = done;
      }),
  );
  const { attachBuiltinDirectorMotions } =
    await import("@/components/canvas/director/director-builtin-motion");
  const { root } = target();
  const controller = new AbortController();
  const pending = attachBuiltinDirectorMotions(root, controller.signal);
  const assertion = expect(pending).rejects.toMatchObject({
    name: "AbortError",
  });
  controller.abort();
  resolve(new Response(JSON.stringify(fixture())));
  await assertion;
  expect(root.animations).toEqual([]);
});

it("does not stop another actor's shared download when one actor is cancelled", async () => {
  let resolve: (value: Response) => void = () => {};
  vi.mocked(fetch).mockImplementation(
    () =>
      new Promise((done) => {
        resolve = done;
      }),
  );
  const { attachBuiltinDirectorMotions } =
    await import("@/components/canvas/director/director-builtin-motion");
  const first = target(),
    second = target(),
    controller = new AbortController();
  const one = attachBuiltinDirectorMotions(first.root, controller.signal);
  const rejected = expect(one).rejects.toMatchObject({ name: "AbortError" });
  const two = attachBuiltinDirectorMotions(
    second.root,
    new AbortController().signal,
  );
  controller.abort();
  resolve(new Response(JSON.stringify(fixture())));
  await rejected;
  await two;
  expect(first.root.animations).toEqual([]);
  expect(second.root.animations).toHaveLength(43);
  expect(fetch).toHaveBeenCalledOnce();
});

it("refuses missing bones without partially attaching", async () => {
  const { attachBuiltinDirectorMotions } =
    await import("@/components/canvas/director/director-builtin-motion");
  const root = new Group();
  await expect(
    attachBuiltinDirectorMotions(root, new AbortController().signal),
  ).rejects.toThrow("骨骼");
  expect(root.animations).toEqual([]);
});

it("rejects invalid quaternion values instead of adding a broken preset", async () => {
  const malformed = fixture();
  malformed.motions[0].tracks[0].values[0] = 20;
  vi.mocked(fetch).mockResolvedValue(new Response(JSON.stringify(malformed)));
  const { attachBuiltinDirectorMotions } =
    await import("@/components/canvas/director/director-builtin-motion");
  const { root } = target();
  await expect(
    attachBuiltinDirectorMotions(root, new AbortController().signal),
  ).rejects.toThrow("格式无效");
  expect(root.animations).toEqual([]);
});

it("binds real CC0 clips to all 12 actual actor shapes without changing their bone lengths or scale", async () => {
  vi.mocked(fetch).mockResolvedValue(
    new Response(
      readFileSync(
        "public/director/motions/quaternius-ual1-standard.json",
        "utf8",
      ),
    ),
  );
  const { attachBuiltinDirectorMotions } =
    await import("@/components/canvas/director/director-builtin-motion");
  for (const sex of ["male", "female"])
    for (const shape of [
      "standard",
      "teen",
      "child",
      "slim",
      "athletic",
      "broad",
    ]) {
      const data = readFileSync(
        `public/director/models/quaternius-standard-${sex}.glb`,
      );
      const buffer = new ArrayBuffer(data.byteLength);
      new Uint8Array(buffer).set(data);
      const root = (await new GLTFLoader().parseAsync(buffer, "")).scene;
      reshapeDirectorQuaterniusActor(root, shape);
      root.position.set(7, 0, 3);
      const bones: Bone[] = [];
      root.traverse((node) => {
        if (node instanceof Bone) bones.push(node);
      });
      const rest = bones.map((bone) => ({
        position: bone.position.toArray(),
        scale: bone.scale.toArray(),
      }));
      await attachBuiltinDirectorMotions(root, new AbortController().signal);
      expect(root.animations).toHaveLength(43);
      expect(bones).toHaveLength(65);
      const mixer = new AnimationMixer(root);
      for (const clip of root.animations) {
        expect(clip.tracks).toHaveLength(65);
        expect(
          clip.tracks.every((track) => track.name.endsWith(".quaternion")),
        ).toBe(true);
        mixer.clipAction(clip).play();
        mixer.setTime(clip.duration * 0.37);
        bones.forEach((bone, index) => {
          expect(bone.position.toArray()).toEqual(rest[index].position);
          expect(bone.scale.toArray()).toEqual(rest[index].scale);
          expect(bone.quaternion.toArray().every(Number.isFinite)).toBe(true);
        });
        expect(root.position.toArray()).toEqual([7, 0, 3]);
        mixer.stopAllAction();
      }
      mixer.uncacheRoot(root);
    }
});

it("ships all 43 real CC0 Standard clips with finite rotation data for the 65-joint rig", () => {
  const resource = JSON.parse(
    readFileSync(
      "public/director/motions/quaternius-ual1-standard.json",
      "utf8",
    ),
  );
  expect(resource.source.license).toBe("CC0-1.0");
  expect(
    resource.motions.map((motion: { name: string }) => motion.name).sort(),
  ).toEqual(DIRECTOR_BUILTIN_MOTION_PRESETS.map((item) => item.name).sort());
  expect(Object.keys(resource.rest_rotations)).toHaveLength(65);
  for (const motion of resource.motions) {
    expect(motion.tracks).toHaveLength(65);
    for (const track of motion.tracks) {
      expect(track.values.length).toBe(track.times.length * 4);
      expect(track.values.every(Number.isFinite)).toBe(true);
      expect(track.times[0]).toBe(0);
      expect(track.times.at(-1)).toBeCloseTo(motion.duration);
    }
  }
});
