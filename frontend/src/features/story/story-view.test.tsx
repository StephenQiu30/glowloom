import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";

import { StoryView } from "@/features/story/story-view";

const versions = {
  sources: [{ id: "source-2", version: 2, status: "draft" as const, resourceVersion: 3 }],
  scripts: [{ id: "script-1", version: 1, status: "draft" as const, resourceVersion: 1 }],
  storyboards: [
    { id: "board-1", version: 1, status: "confirmed" as const, resourceVersion: 2 },
  ],
};

describe("StoryView", () => {
  it("creates a rights-declared source and exposes version confirmation actions", async () => {
    const user = userEvent.setup();
    const createSource = vi.fn();
    const confirmSource = vi.fn();
    const confirmScript = vi.fn();
    render(
      <StoryView
        {...versions}
        onConfirmScript={confirmScript}
        onConfirmSource={confirmSource}
        onConfirmStoryboard={vi.fn()}
        onCreateSource={createSource}
        onGenerateScript={vi.fn()}
        onGenerateStoryboard={vi.fn()}
      />,
    );

    await user.type(screen.getByRole("textbox", { name: "获权故事正文" }), "山".repeat(300));
    screen.getByRole("combobox", { name: "权利依据" }).focus();
    await user.keyboard("{Enter}{ArrowUp}{Enter}");
    await user.click(screen.getByRole("button", { name: "保存来源草稿" }));

    expect(createSource).toHaveBeenCalledWith({ content: "山".repeat(300), rights: "original" });
    await user.click(screen.getByRole("button", { name: "确认来源 v2" }));
    await user.click(screen.getByRole("button", { name: "确认剧本 v1" }));
    expect(confirmSource).toHaveBeenCalledWith("source-2", 3);
    expect(confirmScript).toHaveBeenCalledWith("script-1", 1);
    expect(screen.getByText("AI 提案")).toBeInTheDocument();
    expect(screen.getByText("分镜 v1 · 已确认")).toBeInTheDocument();
  });

  it("shows a 412 conflict without offering automatic overwrite", () => {
    render(
      <StoryView
        {...versions}
        conflictStage="剧本"
        onConfirmScript={vi.fn()}
        onConfirmSource={vi.fn()}
        onConfirmStoryboard={vi.fn()}
        onCreateSource={vi.fn()}
        onGenerateScript={vi.fn()}
        onGenerateStoryboard={vi.fn()}
      />,
    );

    expect(screen.getByRole("alert")).toHaveTextContent("剧本的服务端版本已更新");
    expect(screen.getByText("请重新读取后再次确认")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /覆盖/ })).not.toBeInTheDocument();
  });
});
