import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";

import { TasksView, type TaskItem } from "@/features/tasks/tasks-view";

const tasks: TaskItem[] = [
  {
    id: "task-unknown",
    episodeId: "episode-1",
    locationLabel: "镜头 02",
    studioHref: "/episodes/episode-1/studio?slot=shot_video%3Ashot-2",
    status: "unknown",
    phase: "provider_reconcile",
    resourceVersion: 4,
  },
  {
    id: "task-cancelling",
    episodeId: "episode-1",
    locationLabel: "语音 01",
    studioHref: "/episodes/episode-1/studio?slot=speech_audio%3Aspeech-1",
    status: "cancelling",
    phase: "provider_cancel",
    resourceVersion: 2,
  },
  {
    id: "task-failed",
    episodeId: "episode-1",
    locationLabel: "镜头 03",
    studioHref: "/episodes/episode-1/studio?slot=shot_video%3Ashot-3",
    status: "failed",
    phase: "provider",
    errorCode: "PROVIDER_TIMEOUT",
    retryable: true,
    nextAction: "重试当前镜头",
    resourceVersion: 6,
  },
];

describe("TasksView", () => {
  it("keeps unknown and cancelling distinct from terminal states", () => {
    render(<TasksView tasks={tasks} />);

    expect(screen.getByText("等待对账或人工确认")).toBeInTheDocument();
    expect(screen.getByText("取消已请求，等待服务端收敛")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "重试 镜头 02" })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "取消 语音 01" })).not.toBeInTheDocument();
  });

  it("retries and jumps back to the exact failed slot", async () => {
    const retry = vi.fn();
    const user = userEvent.setup();
    render(<TasksView onRetry={retry} tasks={tasks} />);

    await user.click(screen.getByRole("button", { name: "重试 镜头 03" }));

    expect(retry).toHaveBeenCalledWith("task-failed", 6);
    expect(screen.getByRole("link", { name: "回到 镜头 03" })).toHaveAttribute(
      "href",
      "/episodes/episode-1/studio?slot=shot_video%3Ashot-3",
    );
  });
});
