import { act, cleanup, renderHook } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { useAnnouncementCountdown } from "@/components/notifications/use-announcement-countdown";

afterEach(() => {
  cleanup();
  vi.useRealTimers();
});

it("5000ms后只关闭一次，回调更新不会重置倒计时", () => {
  vi.useFakeTimers();
  const first = vi.fn();
  const latest = vi.fn();
  const { rerender } = renderHook(
    ({ close }) => useAnnouncementCountdown(5, false, close),
    { initialProps: { close: first } },
  );
  act(() => vi.advanceTimersByTime(3000));
  rerender({ close: latest });
  act(() => vi.advanceTimersByTime(1999));
  expect(latest).not.toHaveBeenCalled();
  act(() => vi.advanceTimersByTime(1));
  expect(latest).toHaveBeenCalledTimes(1);
  expect(first).not.toHaveBeenCalled();
  act(() => vi.advanceTimersByTime(10000));
  expect(latest).toHaveBeenCalledTimes(1);
});

it("交互暂停保留剩余时间，恢复后不重新开始5秒", () => {
  vi.useFakeTimers();
  const close = vi.fn();
  const { rerender, result } = renderHook(
    ({ paused }) => useAnnouncementCountdown(5, paused, close),
    { initialProps: { paused: false } },
  );
  act(() => vi.advanceTimersByTime(2000));
  rerender({ paused: true });
  act(() => vi.advanceTimersByTime(15000));
  expect(close).not.toHaveBeenCalled();
  expect(result.current).toBe(3);
  rerender({ paused: false });
  act(() => vi.advanceTimersByTime(2999));
  expect(close).not.toHaveBeenCalled();
  act(() => vi.advanceTimersByTime(1));
  expect(close).toHaveBeenCalledTimes(1);
});

it("卸载清理计时器，不关闭已经离开的消息", () => {
  vi.useFakeTimers();
  const close = vi.fn();
  const { unmount } = renderHook(() =>
    useAnnouncementCountdown(5, false, close),
  );
  act(() => vi.advanceTimersByTime(1000));
  unmount();
  act(() => vi.advanceTimersByTime(10000));
  expect(close).not.toHaveBeenCalled();
  expect(vi.getTimerCount()).toBe(0);
});
