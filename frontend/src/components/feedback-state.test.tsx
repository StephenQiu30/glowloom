import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";

import { FeedbackState } from "@/components/feedback-state";

describe("FeedbackState", () => {
  it("announces non-blocking loading and empty states", () => {
    const { rerender } = render(
      <FeedbackState description="正在从服务端恢复任务事实。" state="loading" title="正在加载" />,
    );

    expect(screen.getByRole("status")).toHaveAttribute("aria-live", "polite");
    expect(screen.getByText("正在从服务端恢复任务事实。")).toBeInTheDocument();

    rerender(<FeedbackState description="创建项目后即可开始。" state="empty" title="暂无项目" />);

    expect(screen.getByRole("status")).toHaveTextContent("暂无项目");
  });

  it("opens error details by keyboard and restores trigger focus", async () => {
    const user = userEvent.setup();
    render(
      <FeedbackState
        description="请求失败，服务端事实未改变。"
        details="NETWORK_UNAVAILABLE"
        state="error"
        title="暂时无法连接"
      />,
    );
    const trigger = screen.getByRole("button", { name: "查看错误详情" });
    trigger.focus();

    await user.keyboard("{Enter}");

    expect(screen.getByRole("dialog", { name: "错误详情" })).toBeInTheDocument();
    expect(screen.getByText("NETWORK_UNAVAILABLE")).toBeInTheDocument();

    await user.keyboard("{Escape}");

    expect(trigger).toHaveFocus();
  });
});
