import {
  act,
  cleanup,
  fireEvent,
  render,
  screen,
} from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { TimedAnnouncementDialog } from "@/components/notifications/timed-announcement";

afterEach(() => {
  cleanup();
  vi.useRealTimers();
});
const message = {
  id: "notice",
  revision: "1",
  title: "新版本上线",
  body: "稍后可从通知中心查看。",
  placement: "modal",
  duration_seconds: 5,
  endless: false,
  manual_close: false,
  frequency: "once" as const,
  action_path: "",
  notification_id: "inbox",
  starts_at: new Date(Date.now() - 1000).toISOString(),
  ends_at: new Date(Date.now() + 3600000).toISOString(),
};

it("长期有效的手动关闭消息不启动停留或截止倒计时", () => {
  vi.useFakeTimers();
  const close = vi.fn();
  render(
    <TimedAnnouncementDialog
      announcement={{
        ...message,
        manual_close: true,
        endless: true,
        ends_at: "0001-01-01T00:00:00Z",
      }}
      onClose={close}
    />,
  );
  act(() => vi.advanceTimersByTime(120000));
  expect(close).not.toHaveBeenCalled();
  expect(screen.queryByRole("button", { name: "暂停自动关闭" })).toBeNull();
  fireEvent.click(screen.getByRole("button", { name: "关闭对话框" }));
  expect(close).toHaveBeenCalledTimes(1);
});

it("默认5秒自动关闭，关闭不会把消息标为已读", () => {
  vi.useFakeTimers();
  const close = vi.fn();
  render(<TimedAnnouncementDialog announcement={message} onClose={close} />);
  expect(screen.getByRole("dialog", { name: "新版本上线" })).toBeTruthy();
  act(() => vi.advanceTimersByTime(4999));
  expect(close).not.toHaveBeenCalled();
  act(() => vi.advanceTimersByTime(1));
  expect(close).toHaveBeenCalledTimes(1);
});

it("悬停暂停、离开继续剩余倒计时", () => {
  vi.useFakeTimers();
  const close = vi.fn();
  render(<TimedAnnouncementDialog announcement={message} onClose={close} />);
  const dialog = screen.getByRole("dialog");
  act(() => vi.advanceTimersByTime(2000));
  fireEvent.pointerEnter(dialog);
  act(() => vi.advanceTimersByTime(9000));
  expect(close).not.toHaveBeenCalled();
  fireEvent.pointerLeave(dialog);
  act(() => vi.advanceTimersByTime(3000));
  expect(close).toHaveBeenCalledTimes(1);
});

it("键盘交互暂停，提供显式继续入口", () => {
  vi.useFakeTimers();
  const close = vi.fn();
  render(<TimedAnnouncementDialog announcement={message} onClose={close} />);
  fireEvent.keyDown(screen.getByRole("dialog"), { key: "Tab" });
  act(() => vi.advanceTimersByTime(9000));
  expect(close).not.toHaveBeenCalled();
  fireEvent.click(screen.getByRole("button", { name: "继续自动关闭" }));
  act(() => vi.advanceTimersByTime(5000));
  expect(close).toHaveBeenCalledTimes(1);
});
