import {
  act,
  cleanup,
  fireEvent,
  render,
  screen,
} from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { AutomaticAnnouncement } from "@/components/notifications/automatic-announcement";
import { validateNotificationDisplay } from "@/api/notifications";

vi.mock("@/api/notifications", () => ({
  validateNotificationDisplay: vi.fn(),
}));
beforeEach(() => {
  vi.mocked(validateNotificationDisplay).mockResolvedValue({ valid: true });
});

const notice = {
  id: "a",
  revision: "1",
  title: "公告",
  body: "正文",
  placement: "modal",
  duration_seconds: 5,
  action_path: "",
  notification_id: "",
  starts_at: new Date(Date.now() - 1000).toISOString(),
  ends_at: new Date(Date.now() + 60000).toISOString(),
};
afterEach(() => {
  cleanup();
  vi.useRealTimers();
  vi.resetAllMocks();
});

it("领取后延迟期间被停用，不在输入结束后展示旧版本", async () => {
  vi.useFakeTimers();
  let resolve!: (value: boolean) => void;
  const close = vi.fn();
  render(
    <>
      <input aria-label="创作内容" />
      <AutomaticAnnouncement
        announcement={notice}
        onShown={() =>
          new Promise<boolean>((done) => {
            resolve = done;
          })
        }
        onClose={close}
      />
    </>,
  );
  await act(async () => vi.advanceTimersByTime(0));
  screen.getByLabelText("创作内容").focus();
  await act(async () => resolve(true));
  vi.mocked(validateNotificationDisplay).mockResolvedValue({ valid: false });
  (screen.getByLabelText("创作内容") as HTMLInputElement).blur();
  await act(async () => vi.advanceTimersByTime(500));
  expect(screen.queryByRole("dialog")).toBeNull();
  expect(close).toHaveBeenCalled();
});

it("未进入收件箱的弹窗也复核有效性，停用后关闭", async () => {
  vi.useFakeTimers();
  const close = vi.fn();
  render(
    <AutomaticAnnouncement
      announcement={notice}
      onShown={async () => true}
      onClose={close}
    />,
  );
  await act(async () => vi.advanceTimersByTime(0));
  expect(screen.getByRole("dialog")).toBeTruthy();
  vi.mocked(validateNotificationDisplay).mockResolvedValue({ valid: false });
  await act(async () => vi.advanceTimersByTime(5000));
  expect(validateNotificationDisplay).toHaveBeenCalledWith({
    id: "a",
    revision: "1",
  });
  expect(close).toHaveBeenCalled();
});

it("领取遇到临时错误时保留候选并自动重试", async () => {
  vi.useFakeTimers();
  const claim = vi
    .fn()
    .mockRejectedValueOnce(new Error("network"))
    .mockResolvedValue(true);
  const close = vi.fn();
  render(
    <AutomaticAnnouncement
      announcement={notice}
      onShown={claim}
      onClose={close}
    />,
  );
  await act(async () => vi.advanceTimersByTime(0));
  expect(screen.queryByRole("dialog")).toBeNull();
  expect(close).not.toHaveBeenCalled();
  await act(async () => vi.advanceTimersByTime(5000));
  expect(screen.getByRole("dialog")).toBeTruthy();
  expect(claim).toHaveBeenCalledTimes(2);
});

it("正在输入时延后，闲置后才领取并显示", async () => {
  vi.useFakeTimers();
  const claim = vi.fn().mockResolvedValue(true);
  render(
    <>
      <input aria-label="创作内容" />
      <AutomaticAnnouncement
        announcement={notice}
        onShown={claim}
        onClose={vi.fn()}
      />
    </>,
  );
  screen.getByLabelText("创作内容").focus();
  await act(async () => vi.advanceTimersByTime(1000));
  expect(claim).not.toHaveBeenCalled();
  expect(screen.queryByRole("dialog")).toBeNull();
  fireEvent.blur(screen.getByLabelText("创作内容"));
  (screen.getByLabelText("创作内容") as HTMLInputElement).blur();
  await act(async () => vi.advanceTimersByTime(500));
  expect(claim).toHaveBeenCalledTimes(1);
  expect(screen.getByRole("dialog", { name: "公告" })).toBeTruthy();
});

it("回执等待期间开始输入时再次延后，不能迟到抢焦点", async () => {
  vi.useFakeTimers();
  let resolve!: (value: boolean) => void;
  const claim = vi.fn(
    () =>
      new Promise<boolean>((done) => {
        resolve = done;
      }),
  );
  render(
    <>
      <input aria-label="创作内容" />
      <AutomaticAnnouncement
        announcement={notice}
        onShown={claim}
        onClose={vi.fn()}
      />
    </>,
  );
  await act(async () => vi.advanceTimersByTime(0));
  screen.getByLabelText("创作内容").focus();
  await act(async () => resolve(true));
  expect(screen.queryByRole("dialog")).toBeNull();
  (screen.getByLabelText("创作内容") as HTMLInputElement).blur();
  await act(async () => vi.advanceTimersByTime(500));
  expect(screen.getByRole("dialog", { name: "公告" })).toBeTruthy();
  expect(claim).toHaveBeenCalledTimes(1);
});
