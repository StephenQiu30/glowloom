import { expect, it } from "vitest";
import {
  AnimationClip,
  Bone,
  Group,
  Mesh,
  NumberKeyframeTrack,
  Quaternion,
  Vector3,
} from "three";
import {
  DirectorModelMotion,
  directorModelInfo,
} from "@/components/canvas/director/director-motion";
import {
  editDirector,
  emptyDirectorConfig,
  readDirector,
  type DirectorObject,
} from "@/components/canvas/director/director-model";
function fixture() {
  const object = editDirector(emptyDirectorConfig(), {
    type: "add-object",
    primitive: "box",
  }).objects[0];
  object.kind = "model";
  delete object.primitive;
  object.asset_id = crypto.randomUUID();
  const root = new Group(),
    bone = new Bone();
  bone.name = "Hip";
  root.add(bone);
  root.animations = [
    new AnimationClip("Walk", 2, [
      new NumberKeyframeTrack("Hip.position[x]", [0, 2], [0, 4]),
    ]),
  ];
  const clip = {
    id: crypto.randomUUID(),
    name: "Walk",
    source_animation: "Walk",
    start: 1,
    duration: 6,
    playback_rate: 1,
    loop: true,
  };
  object.motion_clips = [clip];
  return { object, root, bone, clip };
}
it("samples real imported clip tracks deterministically when seeking, looping, changing speed and switching static", () => {
  const { object, root, bone, clip } = fixture();
  clip.playback_rate = 2;
  const motion = new DirectorModelMotion(root, object);
  motion.sample(1.5, true);
  expect(bone.position.x).toBeCloseTo(2);
  motion.sample(2.5, true);
  expect(bone.position.x).toBeCloseTo(2);
  motion.sample(1.25, true);
  expect(bone.position.x).toBeCloseTo(1);
  motion.sample(0.5, true);
  expect(bone.position.x).toBe(0);
  motion.sample(1.5, false);
  expect(bone.position.x).toBe(0);
  motion.dispose();
});
it("clamps non-looping source animation and restores pose outside scheduled duration", () => {
  const { object, root, bone, clip } = fixture();
  clip.loop = false;
  const motion = new DirectorModelMotion(root, object);
  motion.sample(4, true);
  expect(bone.position.x).toBeCloseTo(4);
  motion.sample(7, true);
  expect(bone.position.x).toBe(0);
  motion.dispose();
});
it("blends overlapping repeated source clips independently and rejects missing/ambiguous sources", () => {
  const { object, root, bone, clip } = fixture();
  object.motion_clips!.push({ ...clip, id: crypto.randomUUID(), start: 2 });
  const motion = new DirectorModelMotion(root, object);
  motion.sample(2.5, true);
  expect(bone.position.x).toBeCloseTo(2);
  motion.dispose();
  root.animations.push(root.animations[0].clone());
  expect(() => new DirectorModelMotion(root, object)).toThrow("唯一模型动作");
  root.animations = [];
  expect(() => new DirectorModelMotion(root, object)).toThrow("唯一模型动作");
});
it("applies mapped local bone overrides and quaternion keyframes after animations, then restores bind pose", () => {
  const { object, root, bone } = fixture();
  object.rig = { bone_map: { hips: "Hip" }, animation_names: ["Walk"] };
  const quarterTurn = new Quaternion()
    .setFromAxisAngle(new Vector3(0, 1, 0), Math.PI / 2)
    .toArray();
  object.bone_overrides = { hips: quarterTurn };
  object.bone_tracks = [
    {
      bone: "hips",
      keyframes: [
        {
          id: crypto.randomUUID(),
          time: 0,
          rotation: [0, 0, 0, 1],
          easing: "linear",
        },
        {
          id: crypto.randomUUID(),
          time: 2,
          rotation: quarterTurn,
          easing: "linear",
        },
      ],
    },
  ];
  const motion = new DirectorModelMotion(root, object);
  motion.sample(1, false);
  expect(
    bone.quaternion.angleTo(new Quaternion().fromArray(quarterTurn)),
  ).toBeCloseTo(0);
  motion.sample(1, true);
  expect(bone.quaternion.angleTo(new Quaternion())).toBeCloseTo(Math.PI / 4);
  motion.sample(0, true);
  expect(bone.quaternion.angleTo(new Quaternion())).toBeCloseTo(0);
  motion.dispose();
});
it("publishes only unambiguous animation metadata and real bone names", () => {
  const { object, root } = fixture();
  expect(directorModelInfo(root, object)).toEqual({
    bonePaths: { Hip: "root/0" },
    rest: {
      Hip: { position: [0, 0, 0], rotation: [0, 0, 0], scale: [1, 1, 1] },
    },
    objectID: object.id,
    assetID: object.asset_id,
    animations: [{ name: "Walk", duration: 2 }],
    bones: ["Hip"],
  });
  root.animations.push(root.animations[0].clone());
  expect(directorModelInfo(root, object).animations).toEqual([]);
});
it("rejects invalid persisted motion and bone contracts before enqueuing", () => {
  const { object } = fixture(),
    scene = emptyDirectorConfig();
  scene.objects = [object];
  expect(readDirector(scene).objects[0].motion_clips).toEqual(
    object.motion_clips,
  );
  for (const patch of [
    { motion_clips: [{ ...object.motion_clips![0], playback_rate: 0 }] },
    { motion_clips: [{ ...object.motion_clips![0], start: 3600 }] },
    { active_motion_clip_id: crypto.randomUUID() },
    { bone_overrides: { hips: [0, 0, 0, 0] } },
    { rig: { bone_map: { bogus: "Hip" }, animation_names: [] } },
  ]) {
    expect(() =>
      readDirector({
        ...scene,
        objects: [{ ...object, ...patch } as DirectorObject],
      }),
    ).toThrow();
  }
});

it("freezes root and non-bone transforms while retaining the exact bone pose", async () => {
  const { freezeDirectorScene } =
    await import("@/components/canvas/director/director-workspaces");
  const { object, root, bone, clip } = fixture();
  clip.start = 0;
  const meshNode = new Group();
  meshNode.name = "Duplicate";
  root.add(meshNode);
  root.animations[0].tracks.push(
    new NumberKeyframeTrack(".position[x]", [0, 2], [0, 10]),
  );
  root.animations[0].tracks.push(
    new NumberKeyframeTrack("Duplicate.scale[y]", [0, 2], [1, 3]),
  );
  const motion = new DirectorModelMotion(root, object);
  motion.sample(1, true);
  expect(root.position.x).toBe(5);
  const bones = motion.snapshot(),
    nodes = motion.snapshotNodes();
  const scene = emptyDirectorConfig();
  scene.mode = "dynamic";
  scene.objects = [object];
  const frozen = freezeDirectorScene(
    scene,
    1,
    { [object.id]: bones },
    { [object.id]: nodes },
  );
  const restored = root.clone(true);
  restored.position.set(0, 0, 0);
  restored.children[0].position.set(0, 0, 0);
  restored.children[1].scale.set(1, 1, 1);
  const staticMotion = new DirectorModelMotion(restored, frozen.objects[0]);
  staticMotion.sample(0, false);
  expect(restored.position.x).toBe(5);
  expect(restored.children[1].scale.y).toBe(2);
  expect(staticMotion.snapshot()).toEqual(bones);
  expect(bone.position.x).toBe(2);
  staticMotion.sample(0, true);
  expect(restored.position.x).toBe(0);
  expect(restored.children[1].scale.y).toBe(1);
  motion.dispose();
  staticMotion.dispose();
});
it("uses stable child paths for duplicate names and rejects missing frozen nodes", () => {
  const { object, root } = fixture();
  const a = new Group(),
    b = new Group();
  a.name = b.name = "same";
  a.position.x = 3;
  b.position.x = 8;
  root.add(a, b);
  const motion = new DirectorModelMotion(root, object);
  const nodes = motion.snapshotNodes();
  expect(nodes["root/1"].transform.position[0]).toBe(3);
  expect(nodes["root/2"].transform.position[0]).toBe(8);
  object.frozen_nodes = { "root/99": nodes.root };
  expect(() => motion.sample(0, false)).toThrow("冻结节点");
  motion.dispose();
});

it("validates frozen node paths and values while retaining legacy bone snapshots", () => {
  const { object } = fixture(),
    scene = emptyDirectorConfig();
  const transform = {
    position: [5, 0, 0],
    rotation: [0, 0.2, 0],
    scale: [-1, 1, 1],
  };
  object.frozen_nodes = { root: { transform, morph_weights: [0.25, 0.75] } };
  object.frozen_bones = { Hip: { ...transform, scale: [1, 1, 1] } };
  scene.objects = [object];
  const parsed = readDirector(JSON.parse(JSON.stringify(scene)));
  expect(parsed.objects[0].frozen_nodes).toEqual(object.frozen_nodes);
  expect(parsed.objects[0].frozen_bones).toEqual(object.frozen_bones);
  for (const frozen_nodes of [
    { "root/01": { transform } },
    { "root/../1": { transform } },
    { root: { transform: { ...transform, position: [NaN, 0, 0] } } },
    { root: { transform, morph_weights: Array(257).fill(0) } },
  ]) {
    expect(() =>
      readDirector({ ...scene, objects: [{ ...object, frozen_nodes }] }),
    ).toThrow("冻结节点");
  }
});

it("freezes morph weights and samples again without replacing bound influence arrays", () => {
  const { object, root, clip } = fixture();
  clip.start = 0;
  const mesh = new Mesh();
  mesh.name = "Face";
  mesh.morphTargetInfluences = [0];
  mesh.morphTargetDictionary = { smile: 0 };
  root.add(mesh);
  root.animations[0].tracks.push(
    new NumberKeyframeTrack("Face.morphTargetInfluences[0]", [0, 2], [0, 1]),
  );
  const motion = new DirectorModelMotion(root, object);
  motion.sample(1, true);
  expect(mesh.morphTargetInfluences[0]).toBeCloseTo(0.5);
  const nodes = motion.snapshotNodes();
  motion.sample(0.5, true);
  expect(mesh.morphTargetInfluences[0]).toBeCloseTo(0.25);
  object.frozen_nodes = nodes;
  motion.sample(0, false);
  expect(mesh.morphTargetInfluences[0]).toBeCloseTo(0.5);
  motion.dispose();
});

it("consumes source offsets in source seconds with speed, loop and non-loop end clamping", () => {
  const { object, root, bone, clip } = fixture();
  object.motion_clips = [
    {
      ...clip,
      start: 0,
      duration: 5,
      playback_rate: 2,
      source_offset: 0.5,
      loop: true,
    },
  ];
  const motion = new DirectorModelMotion(root, object);
  motion.sample(0, true);
  expect(bone.position.x).toBeCloseTo(1);
  motion.sample(0.5, true);
  expect(bone.position.x).toBeCloseTo(3);
  motion.sample(1, true);
  expect(bone.position.x).toBeCloseTo(1);
  motion.dispose();
  object.motion_clips[0].loop = false;
  const clamped = new DirectorModelMotion(root, object);
  clamped.sample(1, true);
  expect(bone.position.x).toBeCloseTo(4);
  clamped.dispose();
  const scene = emptyDirectorConfig();
  scene.objects = [object];
  for (const source_offset of [-1, NaN, 3601]) {
    object.motion_clips[0].source_offset = source_offset;
    expect(() => readDirector(scene)).toThrow();
  }
});
