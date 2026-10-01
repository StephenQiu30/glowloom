import { describe, expect, it } from "vitest";
import { createTimeline } from "./timeline";
import {
  applyTranscript,
  transcriptSchema,
  transcriptionResultSchema,
} from "./transcription-model";

const assetId = "a96e4ea7-42e8-4aeb-a99c-262bf45373d4";
const nodeId = "2ab60f74-d5c4-40a9-a6ed-5bc5554e9284";
const draft = {
  version: 1 as const,
  language: "chinese",
  duration_ms: 3000,
  segments: [
    { start_ms: 0, end_ms: 1500, text: "第一句" },
    { start_ms: 1500, end_ms: 3000, text: "第二句" },
  ],
};
function timeline() {
  const value = createTimeline();
  value.clips.push({
    id: crypto.randomUUID(),
    trackId: value.tracks[0].id,
    kind: "video",
    nodeId,
    assetId,
    title: "实际片段",
    startMs: 5000,
    durationMs: 2000,
    sourceStartMs: 500,
    sourceDurationMs: 3000,
    volume: 1,
    fadeInMs: 0,
    fadeOutMs: 0,
    text: "",
  });
  return value;
}
describe("真实转写稿边界", () => {
  it("拒绝缺失时间、重叠、空识别、越界与控制字节", () => {
    expect(transcriptSchema.safeParse(draft).success).toBe(true);
    for (const segments of [
      [],
      [{ end_ms: 1500, text: "没有起点" }],
      [{ start_ms: 0, end_ms: 3001, text: "越界" }],
      [{ start_ms: 0, end_ms: 1000, text: "\0" }],
      [
        { start_ms: 0, end_ms: 1500, text: "A" },
        { start_ms: 1499, end_ms: 3000, text: "B" },
      ],
    ])
      expect(transcriptSchema.safeParse({ ...draft, segments }).success).toBe(
        false,
      );
  });
  it("结果须包含冻结来源与精确字幕稿 SHA", () => {
    const result = {
      job_id: crypto.randomUUID(),
      revision: 5,
      sha256: "a".repeat(64),
      source_asset_id: assetId,
      source_asset_revision: 2,
      source_sha256: "b".repeat(64),
      draft,
      srt: "1\n00:00:00,000 --> 00:00:01,500\n第一句\n",
    };
    expect(transcriptionResultSchema.safeParse(result).success).toBe(true);
    expect(
      transcriptionResultSchema.safeParse({
        ...result,
        source_sha256: "missing",
      }).success,
    ).toBe(false);
  });
});
describe("复核后采用字幕", () => {
  it("按所选片段的裁剪与摆放换算时间，只替换明确的字幕轨道", () => {
    const value = timeline();
    const before = structuredClone(value);
    const next = applyTranscript(
      value,
      draft,
      value.clips[0].id,
      value.tracks[4].id,
      assetId,
      nodeId,
    );
    const subtitles = next.clips.filter((clip) => clip.kind === "subtitle");
    expect(
      subtitles.map((clip) => [clip.startMs, clip.durationMs, clip.text]),
    ).toEqual([
      [5000, 1000, "第一句"],
      [6000, 1000, "第二句"],
    ]);
    expect(next.clips[0]).toEqual(value.clips[0]);
    expect(next.tracks).toEqual(value.tracks);
    expect(value).toEqual(before);
  });
  it("拒绝错误来源、锁定字幕轨道及容量不足，保留原时间线", () => {
    const value = timeline();
    expect(() =>
      applyTranscript(
        value,
        draft,
        value.clips[0].id,
        value.tracks[4].id,
        crypto.randomUUID(),
        nodeId,
      ),
    ).toThrow();
    value.tracks[4].locked = true;
    expect(() =>
      applyTranscript(
        value,
        draft,
        value.clips[0].id,
        value.tracks[4].id,
        assetId,
        nodeId,
      ),
    ).toThrow();
    value.tracks[4].locked = false;
    const many = {
      ...draft,
      duration_ms: 300000,
      segments: Array.from({ length: 1000 }, (_, index) => ({
        start_ms: index * 300,
        end_ms: (index + 1) * 300,
        text: `字幕 ${index}`,
      })),
    };
    value.clips[0].startMs = 0;
    value.clips[0].sourceStartMs = 0;
    value.clips[0].sourceDurationMs = 300000;
    value.clips[0].durationMs = 300000;
    const before = structuredClone(value);
    expect(() =>
      applyTranscript(
        value,
        many,
        value.clips[0].id,
        value.tracks[4].id,
        assetId,
        nodeId,
      ),
    ).toThrow(/1000/);
    expect(value).toEqual(before);
  });
});
