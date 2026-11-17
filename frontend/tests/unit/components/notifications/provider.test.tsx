import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
  act,
} from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { SWRConfig, unstable_serialize } from "swr";
import {
  NotificationProvider,
  useNotificationCenter,
} from "@/components/notifications/notification-provider";
import * as api from "@/api/notifications";
import { RequestError, setCreationScope } from "@/lib/request";

const state = vi.hoisted(() => ({
  session: { actor: { id: "user-a" }, session_id: "session-a" } as {
    actor: { id: string };
    session_id: string;
  } | null,
}));
vi.mock("@/components/auth/session", () => ({
  useOptionalSession: () => state.session,
}));
vi.mock("@/api/notifications", () => ({
  getNotificationSummary: vi.fn(),
  getNotificationDisplay: vi.fn(),
  getPublicAnnouncements: vi.fn(),
  ackNotificationDisplay: vi.fn(),
  validateNotificationDisplay: vi.fn(),
  readAllNotifications: vi.fn(),
}));
const original = {
  data_epoch: "epoch-a",
  mailbox_version: "1",
  max_inbox_seq: "2",
  unread_count: 2,
};

function Probe() {
  const value = useNotificationCenter()!;
  return (
    <>
      <span data-testid="count">
        {value.summary?.unread_count ?? "unknown"}
      </span>
      <span data-testid="identity">{value.identity}</span>
      <button onClick={() => void value.readAll()}>全部已读</button>
      <button onClick={() => void value.refresh()}>刷新</button>
      <span>{value.readAllError ? "失败" : ""}</span>
    </>
  );
}
function tree() {
  return (
    <SWRConfig value={{ provider: () => new Map(), dedupingInterval: 0 }}>
      <NotificationProvider>
        <Probe />
      </NotificationProvider>
    </SWRConfig>
  );
}
beforeEach(() => {
  state.session = { actor: { id: "user-a" }, session_id: "session-a" };
  vi.mocked(api.getNotificationSummary).mockResolvedValue(original);
  vi.mocked(api.getNotificationDisplay).mockResolvedValue({});
  vi.mocked(api.getPublicAnnouncements).mockResolvedValue({});
  vi.mocked(api.ackNotificationDisplay).mockResolvedValue(undefined);
});
afterEach(() => {
  cleanup();
  vi.useRealTimers();
  vi.resetAllMocks();
});

it("首次请求失败后，切换创作范围会自动恢复摘要和公告", async () => {
  vi.mocked(api.getNotificationSummary)
    .mockRejectedValueOnce(new Error("scope cancelled"))
    .mockResolvedValue(original);
  vi.mocked(api.getNotificationDisplay)
    .mockRejectedValueOnce(new Error("scope cancelled"))
    .mockResolvedValue({});
  render(tree());
  await waitFor(() =>
    expect(api.getNotificationSummary).toHaveBeenCalledTimes(1),
  );
  act(() => setCreationScope("notification-recovery-test"));
  await waitFor(() =>
    expect(screen.getByTestId("count").textContent).toBe("2"),
  );
  expect(api.getNotificationDisplay).toHaveBeenCalledTimes(2);
});

it("暂时断网后的周期重试不被SWR错误缓存停止", async () => {
  vi.useFakeTimers();
  vi.mocked(api.getNotificationSummary)
    .mockRejectedValueOnce(new Error("network"))
    .mockResolvedValue(original);
  render(tree());
  await act(async () => vi.advanceTimersByTime(0));
  expect(screen.getByTestId("count").textContent).toBe("unknown");
  await act(async () => vi.advanceTimersByTime(30000));
  expect(screen.getByTestId("count").textContent).toBe("2");
});

it("一键已读使用点击时水位，迟到结果不覆盖新消息计数", async () => {
  let complete!: (value: API.NotificationSummary) => void;
  vi.mocked(api.readAllNotifications).mockImplementation(
    () =>
      new Promise((resolve) => {
        complete = resolve;
      }),
  );
  render(tree());
  await waitFor(() =>
    expect(screen.getByTestId("count").textContent).toBe("2"),
  );
  fireEvent.click(screen.getByRole("button", { name: "全部已读" }));
  expect(api.readAllNotifications).toHaveBeenCalledWith(
    expect.objectContaining({ data_epoch: "epoch-a", through_seq: "2" }),
  );
  vi.mocked(api.getNotificationSummary).mockResolvedValue({
    ...original,
    mailbox_version: "3",
    max_inbox_seq: "3",
    unread_count: 3,
  });
  fireEvent.click(screen.getByRole("button", { name: "刷新" }));
  await waitFor(() =>
    expect(screen.getByTestId("count").textContent).toBe("3"),
  );
  await act(async () =>
    complete({ ...original, mailbox_version: "2", unread_count: 0 }),
  );
  expect(screen.getByTestId("count").textContent).toBe("3");
});

it("失败保留未读数，重试保留原命令和水位", async () => {
  vi.mocked(api.readAllNotifications)
    .mockRejectedValueOnce(new Error("network"))
    .mockResolvedValueOnce({
      ...original,
      mailbox_version: "2",
      unread_count: 0,
    });
  render(tree());
  await waitFor(() =>
    expect(screen.getByTestId("count").textContent).toBe("2"),
  );
  fireEvent.click(screen.getByRole("button", { name: "全部已读" }));
  await screen.findByText("失败");
  expect(screen.getByTestId("count").textContent).toBe("2");
  fireEvent.click(screen.getByRole("button", { name: "全部已读" }));
  await waitFor(() =>
    expect(screen.getByTestId("count").textContent).toBe("0"),
  );
  expect(vi.mocked(api.readAllNotifications).mock.calls[0][0]).toEqual(
    vi.mocked(api.readAllNotifications).mock.calls[1][0],
  );
});

it("切换账号丢弃旧请求和私有摘要", async () => {
  let complete!: (value: API.NotificationSummary) => void;
  vi.mocked(api.readAllNotifications).mockImplementation(
    () =>
      new Promise((resolve) => {
        complete = resolve;
      }),
  );
  const { rerender } = render(tree());
  await waitFor(() =>
    expect(screen.getByTestId("count").textContent).toBe("2"),
  );
  fireEvent.click(screen.getByRole("button", { name: "全部已读" }));
  state.session = { actor: { id: "user-b" }, session_id: "session-b" };
  vi.mocked(api.getNotificationSummary).mockResolvedValue({
    ...original,
    data_epoch: "epoch-b",
    unread_count: 7,
  });
  rerender(tree());
  await waitFor(() =>
    expect(screen.getByTestId("count").textContent).toBe("7"),
  );
  await act(async () =>
    complete({ ...original, mailbox_version: "9", unread_count: 0 }),
  );
  expect(screen.getByTestId("count").textContent).toBe("7");
});

it("自动弹窗先取得服务端展示回执，抢占失败不显示", async () => {
  const popup = {
    id: "notice",
    revision: "1",
    title: "版本更新",
    body: "正文",
    placement: "modal",
    duration_seconds: 5,
    starts_at: new Date(Date.now() - 1000).toISOString(),
    ends_at: new Date(Date.now() + 60000).toISOString(),
    action_path: "",
    notification_id: "inbox",
  };
  vi.mocked(api.getNotificationDisplay).mockResolvedValue({ popup });
  vi.mocked(api.ackNotificationDisplay).mockRejectedValue(new Error("claimed"));
  render(tree());
  await waitFor(() =>
    expect(api.ackNotificationDisplay).toHaveBeenCalledWith({
      announcement_id: "notice",
      revision: "1",
      kind: "shown",
    }),
  );
  expect(screen.queryByRole("dialog", { name: "版本更新" })).toBeNull();
});

it("空间切换中的本地409不会永久丢弃自动公告", async () => {
  vi.useFakeTimers();
  vi.mocked(api.getNotificationDisplay).mockResolvedValue({
    popup: {
      id: "notice",
      revision: "1",
      title: "版本更新",
      body: "正文",
      placement: "modal",
      duration_seconds: 5,
      starts_at: new Date(Date.now() - 1000).toISOString(),
      ends_at: new Date(Date.now() + 60000).toISOString(),
      action_path: "",
      notification_id: "",
    },
  });
  vi.mocked(api.ackNotificationDisplay)
    .mockRejectedValueOnce(new RequestError("scope_switch_pending", 409))
    .mockResolvedValue(undefined);
  vi.mocked(api.validateNotificationDisplay).mockResolvedValue({ valid: true });
  render(tree());
  await act(async () => vi.advanceTimersByTime(0));
  await act(async () => vi.advanceTimersByTime(0));
  expect(screen.queryByRole("dialog")).toBeNull();
  await act(async () => vi.advanceTimersByTime(5000));
  expect(screen.getByRole("dialog", { name: "版本更新" })).toBeTruthy();
  expect(api.ackNotificationDisplay).toHaveBeenCalledTimes(2);
});

it("从SWR缓存恢复的公告也会排队并重新验证，不依赖首次请求回调", async () => {
  const popup = {
    id: "cached",
    revision: "1",
    title: "缓存中的公告",
    body: "正文",
    placement: "modal",
    duration_seconds: 5,
    starts_at: new Date(Date.now() - 1000).toISOString(),
    ends_at: new Date(Date.now() + 60000).toISOString(),
    action_path: "",
    notification_id: "",
  };
  vi.mocked(api.getNotificationDisplay).mockImplementation(
    () => new Promise(() => {}),
  );
  vi.mocked(api.validateNotificationDisplay).mockResolvedValue({ valid: true });
  render(
    <SWRConfig
      value={{
        provider: () => new Map(),
        fallback: {
          [unstable_serialize(["notification-display", "user-a:session-a"])]: {
            popup,
          },
        },
      }}
    >
      <NotificationProvider>
        <Probe />
      </NotificationProvider>
    </SWRConfig>,
  );
  await screen.findByRole("dialog", { name: "缓存中的公告" });
  expect(api.validateNotificationDisplay).toHaveBeenCalledWith({
    id: "cached",
    revision: "1",
  });
});

it("当前弹窗关闭后不连续补弹期间到达的下一条候选", async () => {
  const popup = {
    id: "a",
    revision: "1",
    title: "公告A",
    body: "正文",
    placement: "modal",
    duration_seconds: 5,
    starts_at: new Date(Date.now() - 1000).toISOString(),
    ends_at: new Date(Date.now() + 60000).toISOString(),
    action_path: "",
    notification_id: "",
  };
  vi.mocked(api.getNotificationDisplay).mockResolvedValue({ popup });
  vi.mocked(api.validateNotificationDisplay).mockResolvedValue({ valid: true });
  render(tree());
  await screen.findByRole("dialog", { name: "公告A" });
  vi.mocked(api.getNotificationDisplay).mockResolvedValue({
    popup: { ...popup, id: "b", title: "公告B" },
  });
  fireEvent.click(screen.getByRole("button", { name: "刷新", hidden: true }));
  await waitFor(() =>
    expect(api.getNotificationDisplay).toHaveBeenCalledTimes(2),
  );
  fireEvent.click(screen.getByRole("button", { name: "关闭对话框" }));
  await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
  expect(screen.queryByRole("dialog", { name: "公告B" })).toBeNull();
  expect(api.ackNotificationDisplay).toHaveBeenCalledTimes(1);
});

it("领取回执延迟期间刷新候选，仍展示原公告且不重复领取", async () => {
  const popup = {
    id: "a",
    revision: "1",
    title: "公告A",
    body: "正文",
    placement: "modal",
    duration_seconds: 5,
    starts_at: new Date(Date.now() - 1000).toISOString(),
    ends_at: new Date(Date.now() + 60000).toISOString(),
    action_path: "",
    notification_id: "",
  };
  let release!: () => void;
  const pending = new Promise<void>((resolve) => {
    release = resolve;
  });
  vi.mocked(api.ackNotificationDisplay)
    .mockImplementationOnce(() => pending)
    .mockRejectedValueOnce(new RequestError("notification_conflict", 409));
  vi.mocked(api.getNotificationDisplay).mockResolvedValue({ popup });
  vi.mocked(api.validateNotificationDisplay).mockResolvedValue({ valid: true });
  render(tree());
  await waitFor(() =>
    expect(api.ackNotificationDisplay).toHaveBeenCalledTimes(1),
  );
  vi.mocked(api.getNotificationDisplay).mockResolvedValue({
    popup: { ...popup, id: "b", title: "公告B" },
  });
  fireEvent.click(screen.getByRole("button", { name: "刷新" }));
  await waitFor(() =>
    expect(api.getNotificationDisplay).toHaveBeenCalledTimes(2),
  );
  await act(async () => release());
  await screen.findByRole("dialog", { name: "公告A" });
  expect(api.ackNotificationDisplay).toHaveBeenCalledTimes(1);
});
