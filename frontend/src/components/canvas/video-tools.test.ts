import { describe, expect, it } from "vitest";
import { createVideoClipTimeline } from "./video-tools";
import { CanvasNodeType } from "./model";
import { createNode } from "./document";
import type { MediaAsset } from "./queries";
const project = "10000000-0000-4000-8000-000000000000";
const asset: MediaAsset = {
  id: "20000000-0000-4000-8000-000000000000",
  project_id: project,
  kind: "video",
  mime_type: "video/mp4",
  byte_size: 100,
  revision: 1,
  file_name: "video.mp4",
  width: 101,
  height: 61,
  duration_ms: 5000,
};
const source = {
  ...createNode(CanvasNodeType.Video, { x: 0, y: 0 }),
  assetId: asset.id,
};
describe("视频剪辑草稿", () => {
  it("冻结源身份、真实时长、剪辑偏移与编码裁切", () => {
    const value = createVideoClipTimeline(source, asset, project, {
      startMs: 1000,
      endMs: 3000,
      crop: { x: 3, y: 5, width: 99, height: 57 },
      aspectRatio: "16:9",
    });
    expect(value.clips[0]).toMatchObject({
      nodeId: source.id,
      assetId: asset.id,
      startMs: 0,
      durationMs: 2000,
      sourceStartMs: 1000,
      sourceDurationMs: 5000,
      crop: { x: 2, y: 4, width: 98, height: 56 },
    });
    expect(value.tracks).toHaveLength(1);
  });
  it("拒绝跨项目/身份错配、未检测时长与越界剪辑", () => {
    const selection = {
      startMs: 0,
      endMs: 4000,
      crop: null,
      aspectRatio: "16:9" as const,
    };
    expect(() =>
      createVideoClipTimeline(
        source,
        { ...asset, project_id: asset.id },
        project,
        selection,
      ),
    ).toThrow();
    expect(() =>
      createVideoClipTimeline(
        source,
        { ...asset, duration_ms: undefined },
        project,
        selection,
      ),
    ).toThrow();
    expect(() =>
      createVideoClipTimeline(source, asset, project, {
        ...selection,
        endMs: 6000,
      }),
    ).toThrow();
    expect(() =>
      createVideoClipTimeline(source, asset, project, {
        ...selection,
        startMs: 3950,
      }),
    ).toThrow();
  });
});
