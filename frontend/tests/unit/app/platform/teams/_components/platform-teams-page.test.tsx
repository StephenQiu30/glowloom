import { render } from "../../../../../feedback";
import { cleanup, fireEvent, screen, act } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { PlatformTeamsPage } from "@/app/platform/teams/_components/platform-teams-page";
import { request } from "@/lib/request";
vi.mock("@/components/auth/session", () => ({
  useSession: () => ({
    actor: { id: "admin", platform_role: "platform_admin" },
  }),
}));
vi.mock("@/lib/request", async (original) => ({
  ...(await original<typeof import("@/lib/request")>()),
  request: vi.fn(),
}));
afterEach(() => {
  cleanup();
  vi.resetAllMocks();
});
it("A头像上传迟到时已选B，不再弹出旧团队确认或修改A", async () => {
  URL.createObjectURL = vi.fn(() => "blob:file");
  URL.revokeObjectURL = vi.fn();
  let finish!: (value: unknown) => void;
  const upload = new Promise((resolve) => (finish = resolve));
  vi.mocked(request).mockImplementation(async (url) => {
    if (url === "/api/platform/teams")
      return {
        items: [
          { id: "a", name: "Alpha", status: "active" },
          { id: "b", name: "Beta", status: "active" },
        ],
      };
    if (url === "/api/platform/teams/a")
      return { id: "a", name: "Alpha", status: "active", revision: "1" };
    if (url === "/api/platform/teams/b")
      return { id: "b", name: "Beta", status: "active", revision: "1" };
    if (url === "/api/me/avatar/uploads") return upload;
    return { items: [] };
  });
  render(<PlatformTeamsPage />);
  fireEvent.click(await screen.findByRole("button", { name: "查看 Alpha" }));
  await screen.findByRole("heading", { name: "Alpha" });
  fireEvent.click(screen.getByRole("button", { name: "编辑团队资料" }));
  fireEvent.change(screen.getByLabelText(/选择头像/), {
    target: {
      files: [new File(["image"], "avatar.png", { type: "image/png" })],
    },
  });
  fireEvent.click(screen.getByText("上传并保存头像"));
  fireEvent.click(screen.getByRole("button", { name: "关闭团队详情" }));
  fireEvent.click(screen.getByRole("button", { name: "查看 Beta" }));
  await screen.findByRole("heading", { name: "Beta" });
  const uploadOptions = vi
    .mocked(request)
    .mock.calls.find(([url]) => url === "/api/me/avatar/uploads")?.[1];
  expect(uploadOptions?.signal?.aborted).toBe(true);
  expect(URL.revokeObjectURL).toHaveBeenCalledWith("blob:file");
  await act(async () => finish({ id: "uploaded-for-a" }));
  expect(
    screen.queryByRole("dialog", { name: "修改 Alpha 的头像" }),
  ).toBeNull();
  expect(
    vi
      .mocked(request)
      .mock.calls.some(
        ([url, options]) =>
          url === "/api/platform/teams/a" && options?.method === "PATCH",
      ),
  ).toBe(false);
  expect(screen.getByRole("heading", { name: "Beta" })).toBeTruthy();
});

it("关闭详情同时取消已上传头像的待确认动作", async () => {
  URL.createObjectURL = vi.fn(() => "blob:file");
  URL.revokeObjectURL = vi.fn();
  vi.mocked(request).mockImplementation(async (url) => {
    if (url === "/api/platform/teams")
      return { items: [{ id: "a", name: "Alpha", status: "active" }] };
    if (url === "/api/platform/teams/a")
      return { id: "a", name: "Alpha", status: "active", revision: "1" };
    if (url === "/api/me/avatar/uploads") return { id: "uploaded" };
    return { items: [] };
  });
  render(<PlatformTeamsPage />);
  fireEvent.click(await screen.findByRole("button", { name: "查看 Alpha" }));
  await screen.findByRole("heading", { name: "Alpha" });
  fireEvent.click(screen.getByRole("button", { name: "编辑团队资料" }));
  fireEvent.change(screen.getByLabelText(/选择头像/), {
    target: {
      files: [new File(["image"], "avatar.png", { type: "image/png" })],
    },
  });
  fireEvent.click(screen.getByText("上传并保存头像"));
  await screen.findByRole("dialog", { name: "修改 Alpha 的头像" });
  await act(async () => fireEvent.click(screen.getByText("关闭团队详情")));
  expect(screen.queryByRole("dialog")).toBeNull();
  expect(
    vi
      .mocked(request)
      .mock.calls.some(
        ([url, options]) =>
          url === "/api/platform/teams/a" && options?.method === "PATCH",
      ),
  ).toBe(false);
});
