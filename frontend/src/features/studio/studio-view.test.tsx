import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";

import { StudioView, type StudioSlotItem } from "@/features/studio/studio-view";

const slots: StudioSlotItem[] = [
  {
    group: "asset",
    usageType: "asset_image",
    usageId: "asset-1",
    inputVersionId: "asset-version-1",
    inputHash: "a".repeat(64),
    label: "角色：阿岚",
    task: { id: "task-asset", status: "succeeded" },
    candidate: { id: "candidate-asset", mediaVersionId: "media-asset", status: "ready" },
    adoptionStatus: "active",
  },
  {
    group: "shot",
    usageType: "shot_video",
    usageId: "shot-3",
    inputVersionId: "board-1",
    inputHash: "b".repeat(64),
    label: "镜头 03",
    task: { id: "task-shot-3", status: "failed", errorCode: "PROVIDER_TIMEOUT" },
    adoptionStatus: null,
  },
  {
    group: "speech",
    usageType: "speech_audio",
    usageId: "speech-1",
    inputVersionId: "script-1",
    inputHash: "c".repeat(64),
    label: "语音 01",
    task: { id: "task-speech", status: "succeeded" },
    candidate: { id: "candidate-speech", mediaVersionId: "media-speech", status: "ready" },
    adoptionStatus: null,
  },
];

describe("StudioView", () => {
  it("separates task, candidate, and adoption states by production slot", () => {
    render(<StudioView episodeId="episode-1" slots={slots} />);

    expect(screen.getByRole("heading", { name: "资产" })).toBeInTheDocument();
    expect(screen.getByRole("heading", { name: "镜头" })).toBeInTheDocument();
    expect(screen.getByRole("heading", { name: "语音" })).toBeInTheDocument();
    expect(screen.getByText("任务已成功", { exact: true })).toBeInTheDocument();
    expect(screen.getAllByText("候选可用", { exact: true })).toHaveLength(2);
    expect(screen.getByText("已采用", { exact: true })).toBeInTheDocument();
    expect(screen.getByText("尚未采用", { exact: true })).toBeInTheDocument();
  });

  it("retries only the failed slot and preserves the owning task identity", async () => {
    const retry = vi.fn();
    const user = userEvent.setup();
    render(<StudioView episodeId="episode-1" onRetry={retry} slots={slots} />);

    await user.click(screen.getByRole("button", { name: "仅重试 镜头 03" }));

    expect(retry).toHaveBeenCalledWith("task-shot-3", 0);
    expect(screen.getByText("PROVIDER_TIMEOUT")).toBeInTheDocument();
  });
});
