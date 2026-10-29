import { readFileSync } from "node:fs";
import { afterEach, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import {
  AnimationClip,
  Euler,
  Bone,
  Group,
  NumberKeyframeTrack,
  Quaternion,
  QuaternionKeyframeTrack,
  Vector3,
} from "three";
import { createDirectorActor } from "@/components/canvas/director/director-assets";
import {
  DirectorModelMotion,
  directorModelInfo,
} from "@/components/canvas/director/director-motion";
import {
  directorTransform,
  emptyDirectorConfig,
  readDirector,
} from "@/components/canvas/director/director-model";
import {
  directorLibraryClips,
  directorPoseSource,
} from "@/components/canvas/director/director-motion-library-runtime";
import {
  addDirectorMotionSource,
  directorMotionSourceName,
  validateDirectorMotionLibrary,
  type DirectorMotionSource,
} from "@/components/canvas/director/director-motion-library-model";
import { placeDirectorMotionClip } from "@/components/canvas/director/director-motion-model";
import { DirectorMotionLibrary } from "@/components/canvas/director/director-motion-library";
import { DirectorPoseEditor } from "@/components/canvas/director/director-pose-editor";
import { importDirectorMotion } from "@/components/canvas/director/director-motion-import";
afterEach(cleanup);
function fixture() {
  const object = createDirectorActor("male_standard"),
    root = new Group(),
    bone = new Bone();
  bone.name = "pelvis";
  root.add(bone);
  const source: DirectorMotionSource = {
    id: crypto.randomUUID(),
    name: "我的转身",
    kind: "import",
    duration: 1,
    rest: { pelvis: directorTransform() },
    tracks: [
      {
        bone: "pelvis",
        property: "quaternion",
        times: [0, 1],
        values: [
          0,
          0,
          0,
          1,
          ...new Quaternion()
            .setFromAxisAngle(new Vector3(0, 1, 0), Math.PI / 2)
            .toArray(),
        ],
      },
    ],
  };
  return { object, root, bone, source };
}
it("applies independent compatible sources and rejects unmapped or malformed skeleton tracks", () => {
  const { object, root, bone, source } = fixture();
  Object.assign(object, addDirectorMotionSource(object, source, ["pelvis"]));
  object.motion_clips = [
    {
      id: crypto.randomUUID(),
      name: source.name,
      source_animation: directorMotionSourceName(source.id),
      start: 0,
      duration: 1,
      playback_rate: 1,
      loop: false,
    },
  ];
  const motion = new DirectorModelMotion(root, object);
  motion.sample(0.5, true);
  expect(bone.quaternion.angleTo(new Quaternion())).toBeCloseTo(Math.PI / 4);
  motion.dispose();
  expect(() =>
    addDirectorMotionSource(object, { ...source, id: crypto.randomUUID() }, [
      "head",
    ]),
  ).toThrow("不兼容");
  expect(() =>
    validateDirectorMotionLibrary([
      { ...source, tracks: [{ ...source.tracks[0], values: [0] }] },
    ]),
  ).toThrow();
  expect(() => directorLibraryClips(new Group(), [source])).toThrow("兼容骨骼");
});
it("saves actual pose as a constant reusable clip and survives JSON scene roundtrip", () => {
  const { object, root, bone } = fixture(),
    rest = { pelvis: directorTransform() };
  const pose = {
    pelvis: { position: [0, 0.25, 0], rotation: [0, 0.7, 0], scale: [1, 1, 1] },
  };
  const source = directorPoseSource("姿势", pose, rest);
  Object.assign(object, addDirectorMotionSource(object, source, ["pelvis"]));
  object.motion_clips = [
    {
      id: crypto.randomUUID(),
      name: source.name,
      source_animation: directorMotionSourceName(source.id),
      start: 0,
      duration: 2,
      playback_rate: 1,
      loop: false,
    },
  ];
  const scene = emptyDirectorConfig();
  scene.objects = [object];
  const restored = readDirector(JSON.parse(JSON.stringify(scene)));
  const motion = new DirectorModelMotion(root, restored.objects[0]);
  motion.sample(0.2, true);
  expect(bone.position.y).toBeCloseTo(0.25);
  expect(bone.rotation.y).toBeCloseTo(0.7);
  motion.sample(1.5, true);
  expect(bone.rotation.y).toBeCloseTo(0.7);
  motion.dispose();
});
it("blends only adjacent clips at the boundary and cancellation restores the incoming action", () => {
  const { object, root, bone } = fixture();
  root.animations = [
    new AnimationClip("A", 2, [
      new NumberKeyframeTrack("pelvis.position[x]", [0, 2], [0, 4]),
    ]),
    new AnimationClip("B", 2, [
      new NumberKeyframeTrack("pelvis.position[x]", [0, 2], [10, 12]),
    ]),
  ];
  const a = {
      id: crypto.randomUUID(),
      name: "A",
      source_animation: "A",
      start: 0,
      duration: 2,
      playback_rate: 1,
      loop: false,
    },
    b = {
      id: crypto.randomUUID(),
      name: "B",
      source_animation: "B",
      start: 2,
      duration: 2,
      playback_rate: 1,
      loop: false,
      blend_in: 1,
    };
  object.motion_clips = placeDirectorMotionClip([a], b, 10);
  const motion = new DirectorModelMotion(root, object);
  motion.sample(2, true);
  expect(bone.position.x).toBeCloseTo(4);
  motion.sample(2.5, true);
  expect(bone.position.x).toBeCloseTo(7.25);
  motion.sample(3, true);
  expect(bone.position.x).toBeCloseTo(11);
  motion.dispose();
  expect(() =>
    placeDirectorMotionClip(object.motion_clips!, { ...b, start: 3 }, 10),
  ).toThrow("相邻");
  object.motion_clips = placeDirectorMotionClip(
    object.motion_clips!,
    { ...b, blend_in: 0 },
    10,
  );
  const unblended = new DirectorModelMotion(root, object);
  unblended.sample(2, true);
  expect(bone.position.x).toBeCloseTo(10);
  unblended.dispose();
});
it("imports a self-contained action JSON without changing source identity or accepting incompatible bones", async () => {
  const { source } = fixture();
  const bytes = new TextEncoder().encode(JSON.stringify(source));
  const file = new File([bytes], "motion.json");
  Object.defineProperty(file, "arrayBuffer", {
    value: async () => bytes.buffer,
  });
  const loaded = await importDirectorMotion(
    file,
    ["pelvis"],
    new AbortController().signal,
  );
  expect(loaded.source.id).not.toBe(source.id);
  expect(loaded.source.tracks).toEqual(source.tracks);
  await expect(
    importDirectorMotion(file, ["head"], new AbortController().signal),
  ).rejects.toThrow("不兼容");
  const controller = new AbortController();
  controller.abort();
  await expect(
    importDirectorMotion(file, ["pelvis"], controller.signal),
  ).rejects.toThrow();
});
it("searches library, previews without persistence, applies at playhead and cancels preview", () => {
  const { object, root, source } = fixture();
  object.motion_library = [source];
  root.animations = [new AnimationClip("Walk_Loop", 1, [])];
  const onChange = vi.fn(),
    onPreview = vi.fn();
  render(
    <DirectorMotionLibrary
      object={object}
      info={directorModelInfo(root, object)}
      dynamic
      time={3}
      duration={10}
      disabled={false}
      onChange={onChange}
      onPreview={onPreview}
      onCapturePose={() => ({ pelvis: directorTransform() })}
    />,
  );
  fireEvent.change(screen.getByRole("textbox", { name: "搜索动作" }), {
    target: { value: "行走" },
  });
  fireEvent.click(screen.getByRole("button", { name: "预览行走" }));
  expect(onPreview).toHaveBeenCalled();
  expect(onChange).not.toHaveBeenCalled();
  const previewObject = onPreview.mock.calls.at(-1)![0].object;
  fireEvent.change(screen.getByRole("spinbutton", { name: "动作预览时间" }), {
    target: { value: "0.5" },
  });
  expect(onPreview.mock.calls.at(-1)![0].object).toBe(previewObject);
  expect(onPreview.mock.calls.at(-1)![0].time).toBe(0.5);
  fireEvent.click(screen.getByRole("button", { name: "取消预览" }));
  expect(onPreview).toHaveBeenLastCalledWith(null);
  fireEvent.click(screen.getByRole("button", { name: "应用行走" }));
  expect(onChange.mock.calls[0][0].motion_clips[0].start).toBe(3);
});
it("edits left pose angles independently and retains right limb overrides", () => {
  const object = createDirectorActor("male_standard");
  object.bone_overrides = { rightUpperArm: [0, 0, 0, 1] };
  const onChange = vi.fn();
  render(
    <DirectorPoseEditor object={object} disabled={false} onChange={onChange} />,
  );
  const input = screen.getByRole("spinbutton", { name: "左臂 上臂 前后倾" });
  fireEvent.change(input, { target: { value: "30" } });
  fireEvent.blur(input);
  expect(onChange.mock.calls[0][0].bone_overrides.rightUpperArm).toEqual([
    0, 0, 0, 1,
  ]);
  expect(onChange.mock.calls[0][0].bone_overrides.leftUpperArm).toBeDefined();
  expect(onChange.mock.calls[0][0].frozen_nodes).toBeUndefined();
});

it("imports actual self-contained GLB bone animation and rejects external references", async () => {
  const values = new Float32Array([
    0,
    0,
    0,
    1,
    0,
    0,
    0,
    1,
    0,
    0,
    1,
    0,
    0,
    0,
    1,
    0,
    Math.SQRT1_2,
    0,
    Math.SQRT1_2,
  ]);
  const document = {
    asset: { version: "2.0" },
    scene: 0,
    scenes: [{ nodes: [0, 1] }],
    nodes: [{ name: "pelvis" }, { mesh: 0, skin: 0 }],
    skins: [{ joints: [0] }],
    meshes: [{ primitives: [{ attributes: { POSITION: 0 } }] }],
    buffers: [{ byteLength: values.byteLength }],
    bufferViews: [
      { buffer: 0, byteOffset: 0, byteLength: 36 },
      { buffer: 0, byteOffset: 36, byteLength: 8 },
      { buffer: 0, byteOffset: 44, byteLength: 32 },
    ],
    accessors: [
      {
        bufferView: 0,
        componentType: 5126,
        count: 3,
        type: "VEC3",
        min: [0, 0, 0],
        max: [1, 1, 0],
      },
      {
        bufferView: 1,
        componentType: 5126,
        count: 2,
        type: "SCALAR",
        min: [0],
        max: [1],
      },
      { bufferView: 2, componentType: 5126, count: 2, type: "VEC4" },
    ],
    animations: [
      {
        name: "Turn",
        samplers: [{ input: 1, output: 2, interpolation: "LINEAR" }],
        channels: [{ sampler: 0, target: { node: 0, path: "rotation" } }],
      },
    ],
  };
  const raw = JSON.stringify(document),
    json = new TextEncoder().encode(
      raw + " ".repeat((4 - (raw.length % 4)) % 4),
    );
  const bytes = new ArrayBuffer(12 + 8 + json.length + 8 + values.byteLength),
    view = new DataView(bytes);
  [0x46546c67, 2, bytes.byteLength, json.length, 0x4e4f534a].forEach((n, i) =>
    view.setUint32(i * 4, n, true),
  );
  new Uint8Array(bytes, 20, json.length).set(json);
  const pos = 20 + json.length;
  view.setUint32(pos, values.byteLength, true);
  view.setUint32(pos + 4, 0x004e4942, true);
  new Uint8Array(bytes, pos + 8).set(new Uint8Array(values.buffer));
  const file = new File([bytes], "turn.glb");
  Object.defineProperty(file, "arrayBuffer", { value: async () => bytes });
  const imported = await importDirectorMotion(
    file,
    ["pelvis"],
    new AbortController().signal,
  );
  expect(imported.names).toEqual(["Turn"]);
  expect(imported.source.tracks[0].bone).toBe("pelvis");
  expect(imported.source.tracks[0].values.at(-1)).toBeCloseTo(Math.SQRT1_2);
  const external = new TextEncoder().encode(
    JSON.stringify({
      ...document,
      buffers: [{ uri: "https://invalid.example/a.bin", byteLength: 76 }],
    }),
  );
  const bad = new File([external], "bad.gltf");
  Object.defineProperty(bad, "arrayBuffer", {
    value: async () => external.buffer,
  });
  await expect(
    importDirectorMotion(bad, ["pelvis"], new AbortController().signal),
  ).rejects.toThrow("自包含");
});
it("imports compatible BVH motion as a reusable independent source", async () => {
  const text =
    "HIERARCHY\nROOT pelvis\n{\n OFFSET 0 0 0\n CHANNELS 6 Xposition Yposition Zposition Zrotation Xrotation Yrotation\n End Site\n {\n OFFSET 0 10 0\n }\n}\nMOTION\nFrames: 2\nFrame Time: 1\n0 0 0 0 0 0\n0 0 0 0 0 90\n";
  const bytes = new TextEncoder().encode(text),
    file = new File([bytes], "turn.bvh");
  Object.defineProperty(file, "arrayBuffer", {
    value: async () => bytes.buffer,
  });
  const result = await importDirectorMotion(
    file,
    ["pelvis"],
    new AbortController().signal,
  );
  expect(
    result.source.tracks.some((track) => track.property === "quaternion"),
  ).toBe(true);
  expect(result.source.duration).toBe(1);
});

it("shows zero relative angles for nontrivial rest and retains other frozen bones on editing and reset", () => {
  const object = createDirectorActor("male_standard");
  const bone = object.rig!.bone_map!.leftUpperLeg!;
  const transform = { ...directorTransform(), rotation: [2.86, 0.2, -0.1] };
  const rest = { [bone]: transform };
  object.frozen_bones = { [bone]: transform, untouched: directorTransform() };
  const onChange = vi.fn();
  render(
    <DirectorPoseEditor
      object={object}
      rest={rest}
      disabled={false}
      onChange={onChange}
    />,
  );
  const input = screen.getByRole("spinbutton", { name: "左腿 大腿 前后倾" });
  expect((input as HTMLInputElement).value).toBe("0");
  fireEvent.change(input, { target: { value: "30" } });
  fireEvent.blur(input);
  const expected = new Quaternion()
    .setFromEuler(
      new Euler(...(transform.rotation as [number, number, number])),
    )
    .multiply(new Quaternion().setFromEuler(new Euler(Math.PI / 6, 0, 0)));
  expect(
    new Quaternion()
      .fromArray(onChange.mock.calls[0][0].bone_overrides.leftUpperLeg)
      .angleTo(expected),
  ).toBeCloseTo(0);
  expect(Object.keys(onChange.mock.calls[0][0])).toEqual(["bone_overrides"]);
  fireEvent.click(screen.getByRole("button", { name: "重置左腿" }));
  expect(Object.keys(onChange.mock.calls.at(-1)![0])).toEqual([
    "bone_overrides",
  ]);
});

it("imports the real CC0 Quaternius walk fixture for the builtin skeleton within scene size limits", async () => {
  const bytes = readFileSync(
    "tests/fixtures/director/quaternius-walk-import.glb",
  );
  const motions = JSON.parse(
    readFileSync(
      "public/director/motions/quaternius-ual1-standard.json",
      "utf8",
    ),
  );
  const file = new File([bytes], "quaternius-walk-import.glb");
  Object.defineProperty(file, "arrayBuffer", {
    value: async () => {
      const buffer = new ArrayBuffer(bytes.byteLength);
      new Uint8Array(buffer).set(bytes);
      return buffer;
    },
  });
  const imported = await importDirectorMotion(
    file,
    Object.keys(motions.rest_rotations),
    new AbortController().signal,
  );
  expect(imported.names).toEqual(["Walk_Loop_CC0_Import"]);
  expect(imported.source.tracks).toHaveLength(65);
  expect(
    new TextEncoder().encode(JSON.stringify(imported.source)).length,
  ).toBeLessThan(512 * 1024);
});

it("reads frozen node pose before bone legacy pose and edits one relative axis without losing the others", () => {
  const object = createDirectorActor("male_standard");
  object.pose = "neutral";
  const bone = object.rig!.bone_map!.leftUpperLeg!;
  const rest = { [bone]: directorTransform() };
  object.frozen_bones = { [bone]: directorTransform() };
  object.frozen_nodes = {
    "root/0": {
      transform: {
        ...directorTransform(),
        rotation: [Math.PI / 6, Math.PI / 9, 0],
      },
    },
  };
  const onChange = vi.fn();
  render(
    <DirectorPoseEditor
      object={object}
      rest={rest}
      nodePaths={{ [bone]: "root/0" }}
      disabled={false}
      onChange={onChange}
    />,
  );
  const pitch = screen.getByRole("spinbutton", { name: "左腿 大腿 前后倾" });
  expect((pitch as HTMLInputElement).value).toBe("30");
  fireEvent.change(pitch, { target: { value: "45" } });
  fireEvent.blur(pitch);
  const angles = new Euler().setFromQuaternion(
    new Quaternion().fromArray(
      onChange.mock.calls[0][0].bone_overrides.leftUpperLeg,
    ),
  );
  expect(angles.x).toBeCloseTo(Math.PI / 4);
  expect(angles.y).toBeCloseTo(Math.PI / 9);
  expect(Object.keys(onChange.mock.calls[0][0])).toEqual(["bone_overrides"]);
});

it("captures reusable poses against immutable rest rather than the sampled animation pose", () => {
  const { object, root, bone } = fixture();
  const start = new Quaternion().setFromEuler(new Euler(0, 0.4, 0));
  const end = new Quaternion().setFromEuler(new Euler(0, 1.4, 0));
  bone.quaternion.copy(start);
  root.animations = [
    new AnimationClip("Turn", 1, [
      new QuaternionKeyframeTrack(
        "pelvis.quaternion",
        [0, 1],
        [...start.toArray(), ...end.toArray()],
      ),
    ]),
  ];
  object.motion_clips = [
    {
      id: crypto.randomUUID(),
      name: "Turn",
      source_animation: "Turn",
      start: 0,
      duration: 1,
      playback_rate: 1,
      loop: false,
    },
  ];
  const info = directorModelInfo(root, object);
  const motion = new DirectorModelMotion(root, object);
  motion.sample(0.5, true);
  expect(info.rest!.pelvis.rotation[1]).toBeCloseTo(0.4);
  const expected = bone.quaternion.clone();
  const saved = directorPoseSource("Captured", motion.snapshot(), info.rest!);
  motion.dispose();
  const rebuilt = new Group(),
    restoredBone = new Bone();
  restoredBone.name = "pelvis";
  restoredBone.quaternion.copy(start);
  rebuilt.add(restoredBone);
  const restoredObject = {
    ...object,
    motion_library: [saved],
    motion_clips: [
      {
        ...object.motion_clips[0],
        source_animation: directorMotionSourceName(saved.id),
      },
    ],
  };
  const restored = new DirectorModelMotion(rebuilt, restoredObject);
  restored.sample(0.5, true);
  expect(restoredBone.quaternion.angleTo(expected)).toBeCloseTo(0);
  restored.dispose();
});

it("lists all file actions before validating the selected action so a broken first action cannot hide a valid later one", async () => {
  const original = readFileSync(
    "tests/fixtures/director/quaternius-walk-import.glb",
  );
  const jsonLength = original.readUInt32LE(12);
  const document = JSON.parse(
    original.subarray(20, 20 + jsonLength).toString("utf8"),
  );
  const valid = document.animations[0];
  const invalid = structuredClone(valid);
  invalid.name = "Incompatible_First";
  invalid.channels[0].target.node = document.nodes.length - 1;
  document.animations = [invalid, valid];
  const raw = JSON.stringify(document);
  const json = new TextEncoder().encode(
    raw + " ".repeat((4 - (raw.length % 4)) % 4),
  );
  const binary = original.subarray(28 + jsonLength);
  const bytes = new ArrayBuffer(28 + json.length + binary.byteLength);
  const view = new DataView(bytes);
  [0x46546c67, 2, bytes.byteLength, json.length, 0x4e4f534a].forEach((n, i) =>
    view.setUint32(i * 4, n, true),
  );
  new Uint8Array(bytes, 20, json.length).set(json);
  view.setUint32(20 + json.length, binary.byteLength, true);
  view.setUint32(24 + json.length, 0x004e4942, true);
  new Uint8Array(bytes, 28 + json.length).set(binary);
  const file = new File([bytes], "multiple.glb");
  Object.defineProperty(file, "arrayBuffer", { value: async () => bytes });
  const bones = Object.keys(
    JSON.parse(
      readFileSync(
        "public/director/motions/quaternius-ual1-standard.json",
        "utf8",
      ),
    ).rest_rotations,
  );
  const onNames = vi.fn();
  await expect(
    importDirectorMotion(
      file,
      bones,
      new AbortController().signal,
      undefined,
      onNames,
    ),
  ).rejects.toThrow("匹配");
  expect(onNames).toHaveBeenCalledWith([
    "Incompatible_First",
    "Walk_Loop_CC0_Import",
  ]);
  const imported = await importDirectorMotion(
    file,
    bones,
    new AbortController().signal,
    "Walk_Loop_CC0_Import",
    onNames,
  );
  expect(imported.source.tracks).toHaveLength(65);
});
