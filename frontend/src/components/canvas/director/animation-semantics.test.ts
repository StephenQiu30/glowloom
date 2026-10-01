import { describe, expect, it } from "vitest";
import { createDirectorConfig } from "./model";
import { upsertDirectorKeyframe } from "./scene";
import {
  resolveDirectorBoneRotation,
  resolveDirectorKeyframeRecord,
  resolveDirectorObjectTransformEdit,
  resolveDirectorCameraMoveKeyframes,
} from "./animation-semantics";
import {
  directorModeCapabilities,
  resolveDirectorModeTransition,
} from "./modes";
import { inferDirectorRig } from "./rig";
import {
  directorStageLocalCamera,
  directorStagePoint,
  directorStageTransform,
} from "./stage-transform";
import { createDirectorCameraFromPreset } from "./camera-presets";
describe("导演动画与模式语义", () => {
  it("关闭自动帧时对所有帧应用渲染值的增量，已有动画不会覆盖用户编辑", () => {
    const base = createDirectorConfig().objects[0].transform;
    const frames = upsertDirectorKeyframe(
      upsertDirectorKeyframe([], 0, base),
      2,
      { ...base, position: [2, 0, 0] },
    );
    const edit = resolveDirectorObjectTransformEdit({
      base,
      keyframes: frames,
      rendered: { ...base, position: [1, 0, 0] },
      edited: { ...base, position: [3, 0, 0] },
      autoKey: false,
      time: 1,
    });
    expect(edit.transform.position).toEqual([2, 0, 0]);
    expect(edit.keyframes.map((frame) => frame.transform.position[0])).toEqual([
      2, 4,
    ]);
    expect(edit.keyframes[0].id).toBe(frames[0].id);
  });
  it("开启自动帧只改变当前帧；记录取真实渲染时刻再写入吸附帧", () => {
    const base = createDirectorConfig().objects[0].transform;
    const frames = upsertDirectorKeyframe(
      upsertDirectorKeyframe([], 0, base),
      2,
      { ...base, position: [2, 0, 0] },
    );
    const edit = resolveDirectorObjectTransformEdit({
      base,
      keyframes: frames,
      rendered: base,
      edited: { ...base, position: [10, 0, 0] },
      autoKey: true,
      time: 1,
    });
    expect(edit.transform).toBe(base);
    expect(edit.keyframes[0]).toEqual(frames[0]);
    expect(edit.keyframes).toHaveLength(3);
    const record = resolveDirectorKeyframeRecord({
      base,
      keyframes: frames,
      rawTime: 0.99,
      snappedTime: 1,
    });
    expect(record.time).toBe(1);
    expect(record.transform.position[0]).toBeCloseTo(0.99);
  });
  it("骨骼静态覆盖高于动作，关键帧高于静态覆盖，手指共用规则", () => {
    const rotation = resolveDirectorBoneRotation({
      motion: [0, 0, 1, 0],
      override: [0, 0, 0, 1],
      keyframes: [{ id: crypto.randomUUID(), time: 0, rotation: [1, 0, 0, 0] }],
      time: 0,
    });
    expect(rotation).toEqual([1, 0, 0, 0]);
    const rig = inferDirectorRig(
      ["mixamorig:LeftArm", "mixamorig:LeftHandIndex1", "mixamorig:Hips"],
      ["Walk"],
    );
    expect(rig.boneMap.leftUpperArm).toBe("mixamorig:LeftArm");
    expect(rig.boneMap.leftIndex1).toBe("mixamorig:LeftHandIndex1");
    expect(rig.animationNames).toEqual(["Walk"]);
  });
  it("离开动画停播并关闭自动帧，深度模式回到摆场时恢复场景视图", () => {
    expect(
      resolveDirectorModeTransition({
        mode: "layout",
        playing: true,
        autoKey: true,
        renderMode: "depth",
      }),
    ).toEqual({
      mode: "layout",
      playing: false,
      autoKey: false,
      renderMode: "beauty",
    });
    expect(directorModeCapabilities("pose").bones).toBe(true);
  });
  it("舞台旋转按角度表达，视角回写从世界坐标还原舞台坐标", () => {
    const scene = createDirectorConfig();
    scene.stageTransform = {
      position: [2, 0, 0],
      scale: 2,
      rotation: [0, 90, 0],
    };
    const stage = directorStageTransform(scene);
    const world = directorStagePoint(stage, [1, 0, 0]);
    expect(world[0]).toBeCloseTo(2);
    expect(world[2]).toBeCloseTo(-2);
    expect(
      directorStageLocalCamera(stage, {
        position: world,
        rotation: [0, Math.PI / 2, 0],
        scale: [1, 1, 1],
      }).position[0],
    ).toBeCloseTo(1);
  });
  it("机位预设与首尾运镜保留手工中间帧身份", () => {
    const camera = createDirectorCameraFromPreset({
      presetId: "front-close",
      name: "近景",
      target: [0, 1, 0],
    });
    expect(camera.focalLength).toBe(70);
    expect(camera.transform.position).toEqual([0, 1.25, 2.1]);
    const frames = upsertDirectorKeyframe([], 1, camera.transform);
    const next = resolveDirectorCameraMoveKeyframes(
      frames,
      camera.transform,
      { ...camera.transform, position: [0, 1, 4] },
      2,
    );
    expect(next).toHaveLength(3);
    expect(next[1].id).toBe(frames[0].id);
  });
});
