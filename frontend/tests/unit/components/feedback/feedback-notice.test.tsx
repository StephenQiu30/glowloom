import { cleanup, fireEvent, screen, waitFor } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { render } from "../../../feedback";
import { FeedbackNotice } from "@/components/feedback/feedback-notice";

afterEach(cleanup);

it("首次只显示通知，关闭后在原位置保留恢复入口，查询不会触发重试发送", async () => {
  const read = vi.fn();
  render(
    <FeedbackNotice
      message="发送结果待确认"
      description="草稿已保留"
      action={{ label: "查看状态", onClick: read }}
    />,
  );
  await screen.findByRole("button", { name: "关闭提示" });
  expect(document.querySelector('[data-slot="alert"]')).toBeNull();
  fireEvent.click(screen.getByRole("button", { name: "关闭提示" }));
  await waitFor(() =>
    expect(document.querySelector('[data-slot="alert"]')).not.toBeNull(),
  );
  const alert = document.querySelector('[data-slot="alert"]')!;
  expect(alert.textContent).toContain("草稿已保留");
  fireEvent.click(alert.querySelector("button")!);
  expect(read).toHaveBeenCalledOnce();
  expect(screen.queryByRole("button", { name: "重试发送" })).toBeNull();
});

it("Escape关闭当前通知后仍有恢复操作", async () => {
  render(
    <FeedbackNotice
      message="发送结果待确认"
      action={{ label: "查看状态", onClick: vi.fn() }}
    />,
  );
  await screen.findByRole("button", { name: "关闭提示" });
  fireEvent.keyDown(document, { key: "Escape" });
  await waitFor(() =>
    expect(document.querySelector('[data-slot="alert"]')).not.toBeNull(),
  );
});
