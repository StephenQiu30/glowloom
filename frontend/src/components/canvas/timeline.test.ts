import { describe, expect, it } from "vitest";
import {
  canPlaceAt,
  computeSnap,
  findNearestAvailablePlacement,
  splitTimelineClip,
  createTimeline,
  encodeTimeline,
  decodeTimeline,
  timelineSchema,
  type TimelineClip,
} from "./timeline";

const id = (value: number) =>
  `00000000-0000-4000-8000-${String(value).padStart(12, "0")}`;
const clip: TimelineClip = {
  id: id(1),
  trackId: id(2),
  kind: "video",
  nodeId: id(3),
  assetId: null,
  title: "第一镜",
  startMs: 1000,
  durationMs: 2000,
  sourceStartMs: 500,
  sourceDurationMs: 6000,
  volume: 1,
  fadeInMs: 0,
  fadeOutMs: 0,
  text: "",
};
describe("迁移时间线编辑算法", () => {
  it("同轨采用半开区间，其他轨道不会碰撞", () => {
    expect(
      canPlaceAt({
        trackId: id(2),
        startMs: 0,
        durationMs: 1000,
        clips: [clip],
      }).ok,
    ).toBe(true);
    expect(
      canPlaceAt({
        trackId: id(2),
        startMs: 900,
        durationMs: 200,
        clips: [clip],
      }).ok,
    ).toBe(false);
    expect(
      canPlaceAt({
        trackId: id(8),
        startMs: 1000,
        durationMs: 200,
        clips: [clip],
      }).ok,
    ).toBe(true);
  });
  it("遇到碰撞选择最近完整间隙并保留非负时间", () => {
    expect(
      findNearestAvailablePlacement({
        trackId: id(2),
        targetStartMs: 1100,
        durationMs: 500,
        clips: [clip],
      }),
    ).toEqual({ startMs: 500, fits: true });
    expect(
      findNearestAvailablePlacement({
        trackId: id(2),
        targetStartMs: -100,
        durationMs: 500,
        clips: [],
      }).startMs,
    ).toBe(0);
  });
  it("屏幕像素阈值吸附到最近片段边界，排除自身", () => {
    const args = {
      candidateMs: 2990,
      playheadMs: 2980,
      clips: [clip],
      pxPerMs: 0.1,
      thresholdPx: 8,
      enabled: true,
    };
    expect(computeSnap(args).snappedMs).toBe(2980);
    expect(
      computeSnap({ ...args, excludeClipId: clip.id, playheadMs: 0 }).snappedMs,
    ).toBe(2990);
  });
  it("分割保留源裁剪偏移且拒绝边缘极小片段", () => {
    const [left, right] = splitTimelineClip(clip, 1800, id(9))!;
    expect(left.durationMs).toBe(800);
    expect(right).toMatchObject({
      id: id(9),
      startMs: 1800,
      sourceStartMs: 1300,
      durationMs: 1200,
    });
    expect(splitTimelineClip(clip, 1050, id(9))).toBeNull();
  });
  it("像素裁切编码往返并仅允许合法图片/视频边界", () => {
    const value = createTimeline();
    const track = value.tracks.find((item) => item.kind === "video")!;
    value.clips = [
      {
        ...clip,
        trackId: track.id,
        crop: { x: 4, y: 6, width: 100, height: 60 },
      },
    ];
    expect(decodeTimeline(encodeTimeline(value))).toEqual(value);
    expect(
      timelineSchema.safeParse({
        ...value,
        clips: [
          { ...value.clips[0], crop: { x: 0, y: 0, width: 99, height: 60 } },
        ],
      }).success,
    ).toBe(false);
    expect(
      timelineSchema.safeParse({
        ...value,
        clips: [
          {
            ...value.clips[0],
            crop: { x: 32700, y: 0, width: 100, height: 60 },
          },
        ],
      }).success,
    ).toBe(false);
    const audioTrack = value.tracks.find((item) => item.kind === "audio")!;
    expect(
      timelineSchema.safeParse({
        ...value,
        clips: [{ ...value.clips[0], kind: "audio", trackId: audioTrack.id }],
      }).success,
    ).toBe(false);
  });
});
