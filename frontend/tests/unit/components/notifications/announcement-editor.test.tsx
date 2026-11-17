import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { AnnouncementEditor } from "@/components/notifications/announcement-editor";
import * as api from "@/api/notifications";
import { requireAnnouncement } from "@/components/notifications/contracts";

const router = vi.hoisted(() => ({ push: vi.fn() }));
vi.mock("next/navigation", () => ({ useRouter: () => router }));
vi.mock("@/api/notifications", () => ({
  createPlatformNotification: vi.fn(),
  savePlatformNotification: vi.fn(),
  publishPlatformNotification: vi.fn(),
}));
vi.mock("@/api/identity", () => ({ platformTeams: vi.fn() }));
const saved = {
  id: "notice-a",
  version: "1",
  status: "draft",
  draft: { title: "维护通知", body: "今晚维护" },
};
beforeEach(() => {
  vi.mocked(api.savePlatformNotification).mockResolvedValue(saved);
  vi.mocked(api.createPlatformNotification).mockResolvedValue(saved);
  vi.mocked(api.publishPlatformNotification).mockResolvedValue({
    ...saved,
    version: "2",
    status: "active",
  });
});

it("定时编辑使用北京时间，长期有效和手动关闭独立提交", async () => {
  const announcement = requireAnnouncement({
    ...saved,
    draft: {
      title: "维护通知",
      body: "今晚维护",
      placement: "modal",
      audience: "authenticated",
      starts_at: "2099-11-17T01:00:00Z",
      ends_at: "0001-01-01T00:00:00Z",
      endless: true,
      manual_close: true,
      frequency: "daily",
      duration_seconds: 5,
      inbox: true,
    },
  });
  render(<AnnouncementEditor announcement={announcement} />);
  expect(
    (screen.getByLabelText("开始时间 · 北京时间") as HTMLInputElement).value,
  ).toBe("2099-11-17T09:00");
  expect(screen.queryByLabelText("截止时间 · 北京时间")).toBeNull();
  expect(screen.queryByLabelText("停留时长（秒）")).toBeNull();
  fireEvent.click(screen.getByRole("button", { name: "保存草稿" }));
  await waitFor(() => expect(api.savePlatformNotification).toHaveBeenCalled());
  expect(api.savePlatformNotification).toHaveBeenCalledWith(
    { id: "notice-a" },
    expect.objectContaining({
      content: expect.objectContaining({
        starts_at: "2099-11-17T01:00:00Z",
        ends_at: undefined,
        endless: true,
        manual_close: true,
        frequency: "daily",
      }),
    }),
  );
});
afterEach(() => {
  cleanup();
  vi.resetAllMocks();
});

it("新通知先保存再发布，默认5秒并跳转真实详情", async () => {
  render(<AnnouncementEditor />);
  fireEvent.change(screen.getByLabelText("标题"), {
    target: { value: "维护通知" },
  });
  fireEvent.change(screen.getByLabelText("正文"), {
    target: { value: "今晚维护" },
  });
  fireEvent.click(screen.getByRole("button", { name: "发布" }));
  await waitFor(() =>
    expect(api.publishPlatformNotification).toHaveBeenCalled(),
  );
  expect(api.createPlatformNotification).toHaveBeenCalledWith(
    expect.objectContaining({
      content: expect.objectContaining({
        title: "维护通知",
        body: "今晚维护",
        duration_seconds: 5,
      }),
    }),
  );
  expect(api.publishPlatformNotification).toHaveBeenCalledWith(
    { id: "notice-a" },
    expect.objectContaining({ expected_version: "1" }),
  );
  expect(router.push).toHaveBeenCalledWith("/platform/notifications/notice-a");
});

it("发布失败后重试复用保存和发布命令，不创建第二条公告", async () => {
  vi.mocked(api.publishPlatformNotification)
    .mockRejectedValueOnce(new Error("network"))
    .mockResolvedValueOnce({ ...saved, version: "2", status: "active" });
  render(<AnnouncementEditor />);
  fireEvent.change(screen.getByLabelText("标题"), {
    target: { value: "维护通知" },
  });
  fireEvent.change(screen.getByLabelText("正文"), {
    target: { value: "今晚维护" },
  });
  fireEvent.click(screen.getByRole("button", { name: "发布" }));
  await screen.findByRole("alert");
  fireEvent.click(screen.getByRole("button", { name: "发布" }));
  await waitFor(() => expect(router.push).toHaveBeenCalled());
  expect(api.createPlatformNotification).toHaveBeenCalledTimes(1);
  expect(vi.mocked(api.publishPlatformNotification).mock.calls[0]).toEqual(
    vi.mocked(api.publishPlatformNotification).mock.calls[1],
  );
});

it("保存结果未知时保留原内容和命令，先恢复原保存再发布", async () => {
  vi.mocked(api.createPlatformNotification)
    .mockRejectedValueOnce(new Error("network"))
    .mockResolvedValueOnce(saved);
  render(<AnnouncementEditor />);
  fireEvent.change(screen.getByLabelText("标题"), {
    target: { value: "维护通知" },
  });
  fireEvent.change(screen.getByLabelText("正文"), {
    target: { value: "今晚维护" },
  });
  fireEvent.click(screen.getByRole("button", { name: "发布" }));
  await screen.findByRole("alert");
  expect(
    (screen.getByLabelText("标题") as HTMLInputElement).matches(":disabled"),
  ).toBe(true);
  fireEvent.click(screen.getByRole("button", { name: "重试原操作" }));
  await waitFor(() => expect(router.push).toHaveBeenCalled());
  expect(vi.mocked(api.createPlatformNotification).mock.calls[0]).toEqual(
    vi.mocked(api.createPlatformNotification).mock.calls[1],
  );
});
