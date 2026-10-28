import { expect, it } from "vitest";
import {
  defaultAssetFilters,
  historyFilterParams,
  requirePersonalHistory,
} from "@/app/assets/_components/asset-query";
import {
  canBatchDownload,
  historyCardTargets,
  historyCommandTarget,
  historyPublicationTargets,
} from "@/app/assets/_components/history-results";
const id = "11111111-1111-4111-8111-111111111111";
const pid = "22222222-2222-4222-8222-222222222222";
const output = "33333333-3333-4333-8333-333333333333";
const second = "44444444-4444-4444-8444-444444444444";
const task = {
  id,
  project_id: pid,
  capability: "video.generate",
  mode: "lip_sync",
  status: "completed",
  create_time: "2026-10-28T00:00:00Z",
  prompt_preview: "测试结果",
  history_revision: 9,
  outputs: [output, second].map((id) => ({
    id,
    kind: "video",
    media_asset_id: pid,
    moderation_status: "passed",
    favorite: true,
    metadata_revision: 4,
    resolution: "1080P",
    ratio: "16:9",
    generation_mode: "lip_sync",
    process_types: [],
    can_download: true,
    can_publish: true,
  })),
};
const validated = () =>
  requirePersonalHistory({
    items: [task as API.PersonalHistoryItem],
    next_cursor: null,
  } as unknown as API.PersonalHistoryPageResponse).items[0];
it("视频按记录计数但发布保留全部输出，图片按结果CAS", () => {
  const record = historyCardTargets([validated()], "video");
  expect(record).toHaveLength(1);
  expect(record[0].outputs).toHaveLength(2);
  expect(historyCommandTarget(record[0])).toEqual({
    operation_id: id,
    output_id: undefined,
    expected_revision: 9,
  });
  expect(historyPublicationTargets(record)).toEqual([
    { operation_id: id, output_id: output },
    { operation_id: id, output_id: second },
  ]);
  const results = historyCardTargets([validated()], "image");
  expect(results).toHaveLength(2);
  expect(historyCommandTarget(results[1])).toEqual({
    operation_id: id,
    output_id: second,
    expected_revision: 4,
  });
});
it("对口型可发布，配音不可发布；最终能力不足不依据成功状态猜测", () => {
  expect(historyCardTargets([validated()], "video")[0].canPublish).toBe(true);
  for (const mode of ["video_dubbing", "dubbing"])
    expect(
      historyCardTargets([{ ...validated(), mode }], "video")[0].canPublish,
    ).toBe(false);
  expect(
    historyCardTargets(
      [{ ...validated(), capability: "video.dubbing" }],
      "video",
    )[0].canPublish,
  ).toBe(false);
  const unfinished = {
    ...validated(),
    status: "succeeded" as const,
    outputs: validated().outputs.map((item) => ({
      ...item,
      can_download: false,
      can_publish: false,
    })),
  };
  expect(historyCardTargets([unfinished], "video")[0]).toMatchObject({
    canDownload: false,
    canPublish: false,
  });
  for (const kind of ["audio", "document"] as const)
    expect(
      historyCardTargets([validated()], kind).every((card) => !card.canPublish),
    ).toBe(true);
});
it("视频配乐合成单选可下载，多选混入即禁用", () => {
  const ordinary = historyCardTargets([validated()], "video")[0];
  for (const mode of [
    "VideoBGM",
    "AudioVideoMix",
    "video_bgm",
    "audio_video_mix",
  ]) {
    const special = historyCardTargets([{ ...validated(), mode }], "video")[0];
    expect(canBatchDownload([special], "video")).toBe(true);
    expect(canBatchDownload([ordinary, special], "video")).toBe(false);
  }
});
it("无输出记录可按记录修订删除，不制造收藏或下载资格", () => {
  const card = historyCardTargets(
    [{ ...validated(), outputs: [], status: "failed" }],
    "image",
  )[0];
  expect(card).toMatchObject({
    canFavorite: false,
    canDownload: false,
    canPublish: false,
  });
  expect(historyCommandTarget(card).expected_revision).toBe(9);
});
it("多组选项使用不同CSV参数，时间为半开区间", () => {
  expect(
    historyFilterParams({
      ...defaultAssetFilters,
      favorite: true,
      processTypes: ["super_resolution", "frame_interpolation"],
      resolutions: ["1080P", "4K"],
      ratios: ["16:9"],
      generationModes: ["first_last_frame", "multi_frame"],
    }),
  ).toMatchObject({
    favorite_only: true,
    process_types: "super_resolution,frame_interpolation",
    resolutions: "1080P,4K",
    ratios: "16:9",
    generation_modes: "first_last_frame,multi_frame",
  });
});
it("拒绝缺失修订与资格的旧摘要，避免旧后端上误执行新动作", () => {
  for (const key of [
    "can_download",
    "can_publish",
    "metadata_revision",
  ] as const) {
    const broken = {
      ...task,
      outputs: [{ ...task.outputs[0], [key]: undefined }],
    };
    expect(() =>
      requirePersonalHistory({
        items: [broken],
        next_cursor: null,
      } as unknown as API.PersonalHistoryPageResponse),
    ).toThrow();
  }
});
