import { describe, expect, it } from "vitest";
import { createDirectorConfig, decodeDirector, encodeDirector } from "./model";
import {
  applyDirectorUniformScale,
  createDirectorObject,
  directorFocalLengthToFov,
  directorFovToFocalLength,
  directorTransformPathLength,
  interpolateDirectorTransform,
  removeDirectorBoneKeyframe,
  resolveDirectorKeyframeProgress,
  upsertDirectorKeyframe,
} from "./scene";

describe("导演台迁移算法", () => {
  it("严格导演场景以snake字段往返，骨骼映射名称保持枚举", () => {
    const scene = createDirectorConfig();
    scene.objects[0].rig!.boneMap.leftUpperArm = "LeftArm";
    expect(decodeDirector(encodeDirector(scene))).toEqual(scene);
    expect(() =>
      decodeDirector({
        ...encodeDirector(scene),
        url: "https://example.invalid/model.glb",
      }),
    ).toThrow();
  });
  it("35mm全画幅焦距与视角双向一致", () => {
    expect(directorFovToFocalLength(directorFocalLengthToFov(85))).toBeCloseTo(
      85,
      8,
    );
  });
  it("关键帧相同时点更新身份并保持顺序，缓动取前帧", () => {
    const first = upsertDirectorKeyframe([], 2, {
      position: [0, 0, 0],
      rotation: [0, 0, 0],
      scale: [1, 1, 1],
    });
    const replaced = upsertDirectorKeyframe(first, 2.0005, {
      position: [1, 0, 0],
      rotation: [0, 0, 0],
      scale: [1, 1, 1],
    });
    expect(replaced).toHaveLength(1);
    expect(replaced[0].id).toBe(first[0].id);
    expect(resolveDirectorKeyframeProgress(0.5, "smooth")).toBe(0.5);
    expect(resolveDirectorKeyframeProgress(0.9, "step")).toBe(0);
  });
  it("缩放倍率同时更新基础和关键帧并保留各轴比例", () => {
    const object = createDirectorObject();
    object.transform.scale = [1, 2, 3];
    object.keyframes = upsertDirectorKeyframe([], 0, object.transform);
    const next = applyDirectorUniformScale(object, 2);
    expect(next.transform.scale).toEqual([2, 4, 6]);
    expect(next.keyframes[0].transform.scale).toEqual([2, 4, 6]);
    expect(object.transform.scale).toEqual([1, 2, 3]);
  });
  it("移除最后骨骼关键帧清除空轨道", () => {
    const tracks = [
      {
        bone: "head" as const,
        keyframes: [
          {
            id: "key",
            time: 0,
            rotation: [0, 0, 0, 1] as [number, number, number, number],
          },
        ],
      },
    ];
    expect(removeDirectorBoneKeyframe(tracks, "head", "key")).toEqual([]);
    expect(removeDirectorBoneKeyframe(tracks, "head", "unknown")).toBe(tracks);
  });
  it("按时间插值transform和路径长度，忽略非法统计位置", () => {
    const object = createDirectorObject();
    const frames = upsertDirectorKeyframe(
      upsertDirectorKeyframe([], 0, object.transform),
      2,
      { ...object.transform, position: [3, 4.5, 0] },
    );
    expect(
      interpolateDirectorTransform(object.transform, frames, 1).position,
    ).toEqual([1.5, 2.5, 0]);
    expect(directorTransformPathLength(frames)).toBe(5);
  });
});
