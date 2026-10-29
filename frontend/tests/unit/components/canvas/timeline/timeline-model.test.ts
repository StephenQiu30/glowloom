import { describe, expect, it } from "vitest";
import {
  emptyTimelineConfig,
  editTimeline,
  readTimeline,
  timelineDuration,
  timelineTime,
  timelineRulerTime,
} from "@/components/canvas/timeline/timeline-model";
const asset = (kind = "video"): API.AssetSummary => ({
  id: crypto.randomUUID(),
  project_id: crypto.randomUUID(),
  revision: 1,
  kind,
  file_name: "测试素材",
  duration_ms: 10000,
  width: 1920,
  height: 1080,
});
const populated = () =>
  editTimeline(emptyTimelineConfig(), {
    type: "add",
    assets: [{ asset: asset() }],
  });
describe("时间线正式编辑合同", () => {
  it("empty config is accepted with exact service defaults", () => {
    expect(readTimeline(emptyTimelineConfig())).toMatchObject({
      version: 1,
      aspect_ratio: "auto",
      fps: 30,
      clips: [],
      snapping: true,
    });
  });
  it("image shares the main visual lane while audio uses its own durable lane", () => {
    const image = asset("image"),
      audio = asset("audio");
    const result = editTimeline(emptyTimelineConfig(), {
      type: "add",
      assets: [{ asset: image }, { asset: audio }],
    });
    expect(result.clips.map((c) => c.asset_id)).toEqual([image.id, audio.id]);
    expect(result.tracks.map((t) => t.kind)).toEqual(["video", "audio"]);
    expect(result.clips[0].duration_ms).toBe(5000);
    expect(JSON.stringify(result)).not.toMatch(/blob:|https:/);
  });
  it("split and trim preserve the original source interval", () => {
    const base = populated(),
      id = base.clips[0].id;
    const split = editTimeline(base, { type: "split", id, at: 4000 });
    expect(
      split.clips.map((c) => [c.start_ms, c.duration_ms, c.source_start_ms]),
    ).toEqual([
      [0, 4000, 0],
      [4000, 6000, 4000],
    ]);
    const left = editTimeline(split, {
      type: "trim-left",
      id: split.clips[1].id,
      at: 5500,
    });
    const right = editTimeline(left, {
      type: "trim-right",
      id: split.clips[1].id,
      at: 8000,
    });
    expect(right.clips[1]).toMatchObject({
      start_ms: 4000,
      duration_ms: 4000,
      source_start_ms: 5500,
    });
    expect(base.clips[0].duration_ms).toBe(10000);
  });
  it("rejects clips shorter than 100ms and protects locked lanes", () => {
    const base = populated(),
      id = base.clips[0].id;
    expect(() => editTimeline(base, { type: "split", id, at: 50 })).toThrow();
    const locked = editTimeline(base, {
      type: "track",
      id: base.tracks[0].id,
      patch: { locked: true },
    });
    expect(() => editTimeline(locked, { type: "remove", id })).toThrow(
      "轨道已锁定",
    );
    expect(() =>
      editTimeline(locked, { type: "clip", id, patch: { volume: 0 } }),
    ).toThrow();
  });
  it("inserts visual moves into the continuous sequence rather than rejecting overlap", () => {
    const base = editTimeline(populated(), {
      type: "add",
      assets: [{ asset: asset() }],
    });
    const second = base.clips[1].id;
    expect(
      editTimeline(base, {
        type: "move",
        id: second,
        start: 10040,
        tolerance: 100,
      }).clips[1].start_ms,
    ).toBe(10000);
    expect(
      editTimeline(base, {
        type: "move",
        id: second,
        start: 9000,
        tolerance: 100,
      }).clips[1].start_ms,
    ).toBe(10000);
  });
  it("reorders adjacent clips without dropping source offsets or changing total duration", () => {
    const base = editTimeline(populated(), {
      type: "add",
      assets: [{ asset: asset() }],
    });
    const reordered = editTimeline(base, {
      type: "reorder",
      id: base.clips[1].id,
      direction: -1,
    });
    expect(
      reordered.clips.find((c) => c.id === base.clips[1].id)?.start_ms,
    ).toBe(0);
    expect(
      reordered.clips.find((c) => c.id === base.clips[0].id)?.start_ms,
    ).toBe(10000);
    expect(timelineDuration(reordered)).toBe(20000);
  });
  it("rejects malformed, nonfinite and out-of-source values", () => {
    const base = populated();
    expect(() => readTimeline({ ...base, fps: 17 })).toThrow();
    expect(() =>
      editTimeline(base, {
        type: "clip",
        id: base.clips[0].id,
        patch: { volume: Number.NaN },
      }),
    ).toThrow();
    expect(() =>
      editTimeline(base, {
        type: "clip",
        id: base.clips[0].id,
        patch: { duration_ms: 11000 },
      }),
    ).toThrow();
    expect(() =>
      editTimeline(base, {
        type: "add",
        assets: [{ asset: { ...asset(), id: "blob:fake" } }],
      }),
    ).toThrow();
  });
});

it("persists lane mute/visibility independently from edit locking", () => {
  const base = populated();
  const edited = editTimeline(base, {
    type: "track",
    id: base.tracks[0].id,
    patch: { locked: true, muted: true, visible: false },
  });
  expect(
    readTimeline(JSON.parse(JSON.stringify(edited))).tracks[0],
  ).toMatchObject({ locked: true, muted: true, visible: false });
  expect(() =>
    editTimeline(edited, { type: "move", id: edited.clips[0].id, start: 1000 }),
  ).toThrow("锁定");
  const unlocked = editTimeline(edited, {
    type: "track",
    id: edited.tracks[0].id,
    patch: { locked: false },
  });
  expect(
    editTimeline(unlocked, { type: "remove", id: unlocked.clips[0].id }).clips,
  ).toEqual([]);
});
it("does not silently create a different lane when the selected import lane disappears", () => {
  expect(() =>
    editTimeline(emptyTimelineConfig(), {
      type: "add",
      assets: [{ asset: asset() }],
      trackID: crypto.randomUUID(),
    }),
  ).toThrow("目标轨道已不存在");
});
it("audio movement preserves absolute placement and cannot enter the visual lane", () => {
  const base = editTimeline(populated(), {
    type: "add",
    assets: [{ asset: asset() }, { asset: asset("audio") }],
  });
  const next = editTimeline(
    { ...base, snapping: false },
    { type: "move", id: base.clips[2].id, start: 10040, tolerance: 100 },
  );
  expect(next.clips[2].start_ms).toBe(10040);
  expect(() =>
    editTimeline(base, {
      type: "move",
      id: base.clips[0].id,
      start: 0,
      trackID: base.tracks[1].id,
    }),
  ).toThrow("目标轨道不可编辑");
});

it("imports without creating clips, deduplicates and retains the shelf after deletion and reopen", () => {
  const image = asset("image");
  const imported = editTimeline(emptyTimelineConfig(), {
    type: "import",
    assets: [{ asset: image }, { asset: image }],
  });
  expect(imported.clips).toEqual([]);
  expect(imported.imported_assets).toEqual([image.id]);
  const added = editTimeline(imported, {
    type: "add",
    assets: [{ asset: image }],
  });
  const removed = editTimeline(added, {
    type: "remove",
    id: added.clips[0].id,
  });
  expect(
    readTimeline(JSON.parse(JSON.stringify(removed))).imported_assets,
  ).toEqual([image.id]);
  const legacy = { ...added, imported_assets: undefined };
  expect(readTimeline(legacy).imported_assets).toEqual([image.id]);
});

it("keeps a four second video and five second image continuous through trim, resize, reorder and delete", () => {
  const base = editTimeline(emptyTimelineConfig(), {
    type: "add",
    assets: [
      { asset: { ...asset(), duration_ms: 4000 } },
      { asset: asset("image") },
      { asset: { ...asset("audio"), duration_ms: 7000 } },
    ],
  });
  const [video, image, audio] = base.clips;
  expect(video.track_id).toBe(image.track_id);
  expect(audio.track_id).not.toBe(video.track_id);
  expect(timelineDuration(base)).toBe(9000);
  const left = editTimeline(base, {
    type: "trim-left",
    id: video.id,
    at: 1000,
  });
  expect(left.clips.map((c) => [c.start_ms, c.duration_ms])).toEqual([
    [0, 3000],
    [3000, 5000],
    [0, 7000],
  ]);
  expect(left.clips[0].source_start_ms).toBe(1000);
  const right = editTimeline(base, {
    type: "trim-right",
    id: video.id,
    at: 1000,
  });
  expect(right.clips[1].start_ms).toBe(1000);
  expect(timelineDuration(right)).toBe(7000);
  const resized = editTimeline(base, {
    type: "clip",
    id: video.id,
    patch: { duration_ms: 2000 },
  });
  expect(resized.clips[1].start_ms).toBe(2000);
  const reordered = editTimeline(base, {
    type: "move",
    id: image.id,
    start: 0,
  });
  expect(reordered.clips[1].start_ms).toBe(0);
  expect(reordered.clips[0].start_ms).toBe(5000);
  expect(
    editTimeline(base, { type: "remove", id: video.id }).clips[0].start_ms,
  ).toBe(0);
});

it("formats preview centiseconds separately from the minute/second ruler", () => {
  expect(timelineTime(136)).toBe("00:00:13");
  expect(timelineTime(61567)).toBe("01:01:56");
  expect(timelineRulerTime(61567)).toBe("01:01");
  expect(timelineTime(3600000)).toBe("60:00:00");
});

it("rejects malformed imported identities and caps the combined shelf and legacy clip identities", () => {
  const base = populated();
  expect(() =>
    readTimeline({ ...base, imported_assets: ["blob:fake"] }),
  ).toThrow();
  expect(() =>
    readTimeline({
      ...base,
      imported_assets: [base.clips[0].asset_id!, base.clips[0].asset_id!],
    }),
  ).toThrow();
  expect(() =>
    readTimeline({
      ...base,
      imported_assets: Array.from({ length: 1000 }, () => crypto.randomUUID()),
    }),
  ).toThrow();
});

it("resizes both visual edges within source bounds and keeps the main lane closed", () => {
  const base = editTimeline(populated(), {
    type: "add",
    assets: [{ asset: asset("image") }],
  });
  const id = base.clips[0].id;
  const trimmed = editTimeline(base, {
    type: "resize",
    id,
    edge: "left",
    delta: 2000,
  });
  expect(trimmed.clips[0]).toMatchObject({
    start_ms: 0,
    duration_ms: 8000,
    source_start_ms: 2000,
  });
  expect(trimmed.clips[1].start_ms).toBe(8000);
  const extended = editTimeline(trimmed, {
    type: "resize",
    id,
    edge: "left",
    delta: -5000,
  });
  expect(extended.clips[0]).toMatchObject({
    start_ms: 0,
    duration_ms: 10000,
    source_start_ms: 0,
  });
  const right = editTimeline(base, {
    type: "resize",
    id,
    edge: "right",
    delta: -2000,
  });
  expect(right.clips[1].start_ms).toBe(8000);
  expect(
    editTimeline(right, { type: "resize", id, edge: "right", delta: 5000 })
      .clips[0].duration_ms,
  ).toBe(10000);
  expect(
    editTimeline(base, { type: "resize", id, edge: "right", delta: -20000 })
      .clips[0].duration_ms,
  ).toBe(100);
});

it("resizes audio using absolute placement and prevents collision with adjacent clips", () => {
  const base = editTimeline(emptyTimelineConfig(), {
    type: "add",
    assets: [{ asset: asset("audio") }, { asset: asset("audio") }],
  });
  const id = base.clips[1].id;
  const shortened = editTimeline(base, {
    type: "resize",
    id,
    edge: "left",
    delta: 2000,
  });
  expect(shortened.clips[1]).toMatchObject({
    start_ms: 12000,
    source_start_ms: 2000,
    duration_ms: 8000,
  });
  expect(
    editTimeline(shortened, { type: "resize", id, edge: "left", delta: -5000 })
      .clips[1],
  ).toMatchObject({ start_ms: 10000, source_start_ms: 0, duration_ms: 10000 });
  expect(
    editTimeline(base, {
      type: "resize",
      id: base.clips[0].id,
      edge: "right",
      delta: 2000,
    }).clips[0].duration_ms,
  ).toBe(10000);
});

it("moves audio between unlocked lanes and pushes colliding clips without changing source offsets", () => {
  let base = editTimeline(emptyTimelineConfig(), {
    type: "add",
    assets: [{ asset: asset("audio") }],
  });
  base = editTimeline(base, { type: "add-track", kind: "audio" });
  const id = base.clips[0].id,
    trackID = base.tracks.at(-1)!.id;
  const moved = editTimeline(
    { ...base, snapping: false },
    { type: "move", id, trackID, start: 2500 },
  );
  expect(moved.clips[0]).toMatchObject({
    track_id: trackID,
    start_ms: 2500,
    source_start_ms: 0,
    duration_ms: 10000,
  });
  const occupied = editTimeline(base, {
    type: "add",
    trackID,
    assets: [{ asset: asset("audio") }],
  });
  const inserted = editTimeline(occupied, {
    type: "move",
    id,
    trackID,
    start: 0,
  });
  expect(inserted.clips[0].start_ms).toBe(0);
  expect(inserted.clips[1].start_ms).toBe(10000);
  expect(base.clips[0].track_id).not.toBe(trackID);
  const locked = editTimeline(base, {
    type: "track",
    id: trackID,
    patch: { locked: true },
  });
  expect(() =>
    editTimeline(locked, { type: "move", id, trackID, start: 0 }),
  ).toThrow("不可编辑");
});

it("matches source audio insertion: a six-second tail moves before a one-second head, while a later placement preserves a gap", () => {
  const imported = editTimeline(emptyTimelineConfig(), {
    type: "add",
    assets: [{ asset: { ...asset("audio"), duration_ms: 7000 } }],
  });
  const base = editTimeline(imported, {
    type: "split",
    id: imported.clips[0].id,
    at: 1000,
  });
  const [first, second] = base.clips;
  const reordered = editTimeline(base, {
    type: "move",
    id: second.id,
    start: 0,
  });
  expect(reordered.clips).toEqual(
    expect.arrayContaining([
      expect.objectContaining({
        id: second.id,
        start_ms: 0,
        duration_ms: 6000,
        source_start_ms: 1000,
      }),
      expect.objectContaining({
        id: first.id,
        start_ms: 6000,
        duration_ms: 1000,
        source_start_ms: 0,
      }),
    ]),
  );
  const gap = editTimeline(base, { type: "move", id: second.id, start: 3000 });
  expect(gap.clips[0].start_ms).toBe(0);
  expect(gap.clips[1].start_ms).toBe(3000);
  expect(timelineDuration(gap)).toBe(9000);
});

it("does not bypass the locked primary visual lane by silently creating another visual track", () => {
  const base = emptyTimelineConfig();
  const locked = editTimeline(base, {
    type: "track",
    id: base.tracks[0].id,
    patch: { locked: true },
  });
  expect(() =>
    editTimeline(locked, { type: "add", assets: [{ asset: asset("image") }] }),
  ).toThrow("锁定");
});

it("rejects a missing track even when a malformed clip also omits its kind", () => {
  const base = populated();
  expect(() =>
    readTimeline({
      ...base,
      clips: [
        { ...base.clips[0], track_id: crypto.randomUUID(), kind: undefined },
      ],
    }),
  ).toThrow();
});

it("creates an audio lane at the dragged boundary and removes its empty lane when moved back", () => {
  let base = editTimeline(emptyTimelineConfig(), {
    type: "add",
    assets: [{ asset: { ...asset("audio"), duration_ms: 7000 } }],
  });
  base = editTimeline(base, { type: "split", id: base.clips[0].id, at: 1000 });
  const clipID = base.clips[1].id,
    sourceTrack = base.clips[1].track_id;
  base = editTimeline(base, { type: "move", id: clipID, start: 3000 });
  const moved = editTimeline(base, {
    type: "move",
    id: clipID,
    start: 3000,
    newAudioTrack: { afterTrackID: sourceTrack },
  });
  const clip = moved.clips.find((c) => c.id === clipID)!;
  expect(moved.tracks.map((t) => t.kind)).toEqual(["video", "audio", "audio"]);
  expect(clip.track_id).not.toBe(sourceTrack);
  expect(clip).toMatchObject({
    start_ms: 3000,
    duration_ms: 6000,
    source_start_ms: 1000,
  });
  const restored = editTimeline(moved, {
    type: "move",
    id: clipID,
    start: 3000,
    trackID: sourceTrack,
  });
  expect(restored.tracks.map((t) => t.id)).toEqual(
    base.tracks.map((t) => t.id),
  );
  expect(restored.clips).toEqual(base.clips);
  expect(moved.tracks).toHaveLength(3);
});
it("permits the primary visual lane as an audio insertion anchor while preserving locked empty lanes", () => {
  let base = editTimeline(emptyTimelineConfig(), {
    type: "add",
    assets: [{ asset: asset("audio") }],
  });
  base = editTimeline(base, { type: "add-track", kind: "audio" });
  const lockedID = base.tracks.at(-1)!.id;
  base = editTimeline(base, {
    type: "track",
    id: lockedID,
    patch: { locked: true },
  });
  const moved = editTimeline(base, {
    type: "move",
    id: base.clips[0].id,
    start: 0,
    newAudioTrack: { afterTrackID: base.tracks[0].id },
  });
  expect(moved.tracks[0]).toEqual(base.tracks[0]);
  expect(moved.tracks[1].id).toBe(moved.clips[0].track_id);
  expect(moved.clips[0].track_id).not.toBe(base.clips[0].track_id);
  expect(moved.tracks[2]).toEqual(base.tracks.at(-1));
  expect(moved.tracks).toHaveLength(3);
});
it("refuses locked, non-audio, stale-anchor and ambiguous new-lane movements without mutating input", () => {
  const visual = populated();
  expect(() =>
    editTimeline(visual, {
      type: "move",
      id: visual.clips[0].id,
      start: 0,
      newAudioTrack: { afterTrackID: visual.tracks[0].id },
    }),
  ).toThrow("音频");
  const audio = editTimeline(emptyTimelineConfig(), {
    type: "add",
    assets: [{ asset: asset("audio") }],
  });
  const original = structuredClone(audio);
  expect(() =>
    editTimeline(audio, {
      type: "move",
      id: audio.clips[0].id,
      start: 0,
      newAudioTrack: { afterTrackID: crypto.randomUUID() },
    }),
  ).toThrow("轨道");
  expect(() =>
    editTimeline(audio, {
      type: "move",
      id: audio.clips[0].id,
      start: 0,
      trackID: audio.clips[0].track_id,
      newAudioTrack: { afterTrackID: audio.tracks[0].id },
    }),
  ).toThrow("目标");
  const locked = editTimeline(audio, {
    type: "track",
    id: audio.clips[0].track_id,
    patch: { locked: true },
  });
  expect(() =>
    editTimeline(locked, {
      type: "move",
      id: locked.clips[0].id,
      start: 0,
      newAudioTrack: { afterTrackID: locked.tracks[0].id },
    }),
  ).toThrow("锁定");
  expect(audio).toEqual(original);
  expect(locked.tracks).toHaveLength(2);
});

it("deletes an empty audio lane with its final clip while retaining the primary lane and imported asset", () => {
  const base = editTimeline(emptyTimelineConfig(), {
    type: "add",
    assets: [{ asset: asset("audio") }],
  });
  const next = editTimeline(base, { type: "remove", id: base.clips[0].id });
  expect(next.clips).toEqual([]);
  expect(next.tracks).toEqual([base.tracks[0]]);
  expect(next.imported_assets).toEqual(base.imported_assets);
});

it("snaps a moved audio boundary to the playhead only while snapping is enabled", () => {
  let base = editTimeline(emptyTimelineConfig(), {
    type: "add",
    assets: [{ asset: { ...asset("audio"), duration_ms: 7000 } }],
  });
  base = editTimeline(base, { type: "split", id: base.clips[0].id, at: 1000 });
  const id = base.clips[1].id;
  base = editTimeline(base, { type: "move", id, start: 3000 });
  const input = {
    type: "move" as const,
    id,
    start: 2112,
    playhead: 2000,
    tolerance: 6000 / 21.4,
  };
  expect(
    editTimeline(base, input).clips.find((c) => c.id === id)?.start_ms,
  ).toBe(2000);
  expect(
    editTimeline({ ...base, snapping: false }, input).clips.find(
      (c) => c.id === id,
    )?.start_ms,
  ).toBe(2112);
  expect(
    editTimeline(base, { ...input, playhead: 2600 }).clips.find(
      (c) => c.id === id,
    )?.start_ms,
  ).toBe(2112);
});
