import { describe, expect, it } from "vitest";
import {
  createDirectorConfig,
  decodeDirector,
  directorSchema,
  encodeDirector,
} from "./model";
import {
  appendDirectorScreenshot,
  createDirectorCaptureContext,
  groupDirectorScreenshots,
  reconcileDirectorCover,
  removeDirectorScreenshot,
} from "./outputs";

const assetId = "10000000-0000-4000-8000-000000000001";
function fixture() {
  const scene = createDirectorConfig("截图场景");
  return {
    ...scene,
    cover: { assetId, shotId: scene.activeShotId },
    shots: scene.shots.map((shot) => ({
      ...shot,
      screenshots: [
        {
          id: crypto.randomUUID(),
          assetId,
          name: "机位一",
          createdAt: "2026-10-02T00:00:00+08:00",
        },
      ],
    })),
  };
}
describe("导演台正式截图与封面", () => {
  it("再次捕获已有图片恢复最新封面并保留原图库身份", () => {
    const scene = fixture();
    const context = createDirectorCaptureContext(scene);
    const oldEntry = scene.shots[0].screenshots[0];
    const { cover: _cover, ...withoutCover } = scene;
    void _cover;
    for (const input of [
      withoutCover,
      {
        ...scene,
        cover: { assetId: crypto.randomUUID(), shotId: scene.activeShotId },
      },
    ]) {
      const adopted = appendDirectorScreenshot(input, context, {
        ...oldEntry,
        id: crypto.randomUUID(),
      });
      expect(adopted.cover).toEqual(scene.cover);
      expect(adopted.shots[0].screenshots).toEqual([oldEntry]);
    }
    expect(appendDirectorScreenshot(scene, context, oldEntry)).toBe(scene);
  });
  it("闭合字段完整往返，拒绝链接、重复身份、失效镜头及超额图库", () => {
    const scene = fixture();
    const wire = encodeDirector(scene);
    expect(wire.cover).toEqual({
      asset_id: assetId,
      shot_id: scene.activeShotId,
    });
    expect(wire.shots[0].screenshots?.[0].created_at).toBe(
      "2026-10-02T00:00:00+08:00",
    );
    expect(decodeDirector(wire)).toEqual(scene);
    expect(
      directorSchema.safeParse({
        ...scene,
        cover: { ...scene.cover, url: "https://example.com/a.png" },
      }).success,
    ).toBe(false);
    expect(
      directorSchema.safeParse({
        ...scene,
        cover: { ...scene.cover, shotId: crypto.randomUUID() },
      }).success,
    ).toBe(false);
    const original = scene.shots[0];
    const entry = original.screenshots[0];
    for (const invalid of [
      { ...entry, createdAt: "2026-02-30T00:00:00Z" },
      { ...entry, name: "\0" },
      { ...entry, name: " " },
      { ...entry, storageKey: "private/object" },
    ])
      expect(
        directorSchema.safeParse({
          ...scene,
          shots: [{ ...original, screenshots: [invalid] }],
        }).success,
      ).toBe(false);
    expect(
      directorSchema.safeParse({
        ...scene,
        shots: [original, { ...original, id: crypto.randomUUID() }],
      }).success,
    ).toBe(false);
    expect(
      directorSchema.safeParse({
        ...scene,
        shots: [
          {
            ...original,
            screenshots: Array.from({ length: 65 }, () => ({
              ...entry,
              id: crypto.randomUUID(),
            })),
          },
        ],
      }).success,
    ).toBe(false);
    expect(
      directorSchema.safeParse({
        ...scene,
        shots: Array.from({ length: 9 }, () => ({
          ...original,
          id: crypto.randomUUID(),
          screenshots: Array.from({ length: 64 }, () => ({
            ...entry,
            id: crypto.randomUUID(),
          })),
        })),
      }).success,
    ).toBe(false);
  });
  it("图集变化保留封面，渲染内容或封面镜头变化清除封面并保留图集", () => {
    const scene = fixture();
    const extra = { ...scene.shots[0].screenshots[0], id: crypto.randomUUID() };
    const gallery = {
      ...scene,
      shots: [
        {
          ...scene.shots[0],
          screenshots: [...scene.shots[0].screenshots, extra],
        },
      ],
    };
    expect(reconcileDirectorCover(scene, gallery).cover).toEqual(scene.cover);
    const changed = reconcileDirectorCover(scene, {
      ...scene,
      background: "#123456",
    });
    expect(changed.cover).toBeUndefined();
    expect(changed.shots[0].screenshots).toEqual(scene.shots[0].screenshots);
    const replacement = { ...scene.shots[0], id: crypto.randomUUID() };
    expect(
      reconcileDirectorCover(scene, {
        ...scene,
        shots: [replacement],
        activeShotId: replacement.id,
      }).cover,
    ).toBeUndefined();
  });
  it("迟到截图不会回填变更场景，同素材回读重试不会重复，分镜删除不重建", () => {
    const scene = createDirectorConfig();
    const context = createDirectorCaptureContext(scene);
    const screenshot = {
      id: crypto.randomUUID(),
      assetId,
      name: "场景截图",
      createdAt: "2026-10-02T00:00:00Z",
    };
    const saved = appendDirectorScreenshot(scene, context, screenshot);
    expect(saved.shots[0].screenshots).toEqual([screenshot]);
    expect(saved.cover).toEqual({ assetId, shotId: scene.activeShotId });
    expect(
      appendDirectorScreenshot(saved, context, {
        ...screenshot,
        id: crypto.randomUUID(),
      }),
    ).toEqual(saved);
    expect(() =>
      appendDirectorScreenshot(
        { ...scene, background: "#123456" },
        context,
        screenshot,
      ),
    ).toThrow("场景已改变");
    expect(() =>
      appendDirectorScreenshot(
        { ...scene, id: crypto.randomUUID() },
        context,
        screenshot,
      ),
    ).toThrow("场景已改变");
  });
  it("按摄影机聚合同机位多个镜头，移除截图只解除引用和相关封面", () => {
    const scene = fixture();
    const second = {
      ...scene.shots[0],
      id: crypto.randomUUID(),
      screenshots: [
        {
          ...scene.shots[0].screenshots[0],
          id: crypto.randomUUID(),
          assetId: crypto.randomUUID(),
        },
      ],
    };
    scene.shots.push(second);
    expect(
      groupDirectorScreenshots(scene)[0].screenshots.map((item) => item.shotId),
    ).toEqual([scene.shots[0].id, second.id]);
    const removed = removeDirectorScreenshot(
      scene,
      scene.shots[0].id,
      scene.shots[0].screenshots[0].id,
    );
    expect(removed.shots[0].screenshots).toEqual([]);
    expect(removed.shots[1].screenshots).toEqual(second.screenshots);
    expect(removed.cover).toBeUndefined();
    expect(
      removeDirectorScreenshot(scene, second.id, second.screenshots[0].id)
        .cover,
    ).toEqual(scene.cover);
  });
  it("服务端省略空可选动作及骨骼集合不误判截图迟到", () => {
    const scene = createDirectorConfig();
    const context = createDirectorCaptureContext(scene);
    const persisted = structuredClone(scene);
    delete persisted.objects[0].motionClips;
    delete persisted.objects[0].boneTracks;
    delete persisted.objects[0].boneOverrides;
    expect(
      appendDirectorScreenshot(persisted, context, {
        id: crypto.randomUUID(),
        assetId,
        name: "已保存场景",
        createdAt: "2026-10-02T00:00:00Z",
      }).cover?.assetId,
    ).toBe(assetId);
  });
});
