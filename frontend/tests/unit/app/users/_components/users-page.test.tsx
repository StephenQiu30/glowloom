import { render } from "../../../../feedback";
import { cleanup, fireEvent, screen, waitFor } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { UserDirectory } from "@/app/users/_components/users-page";
import { request, RequestError } from "@/lib/request";
vi.mock("@/lib/request", async (original) => ({
  ...(await original<typeof import("@/lib/request")>()),
  request: vi.fn(),
}));
vi.mock("@/components/auth/session", () => ({
  useSession: () => ({ actor: { id: "self" } }),
}));
afterEach(() => {
  cleanup();
  vi.resetAllMocks();
});
it("server filters and pagination use platform contract without temporary-password creation", async () => {
  vi.mocked(request).mockResolvedValue({
    items: [
      {
        id: "user",
        email: "creator@example.com",
        display_name: "Creator",
        status: "active",
        platform_role: "creator",
        active_team_count: 1,
      },
    ],
    next_cursor: "next",
  });
  render(<UserDirectory />);
  await screen.findByText("creator@example.com");
  expect(screen.queryByText("创建用户")).toBeNull();
  fireEvent.change(screen.getByLabelText("搜索用户邮箱或显示名"), {
    target: { value: "creator" },
  });
  fireEvent.click(screen.getByText("查询"));
  await waitFor(() =>
    expect(request).toHaveBeenCalledWith(
      "/api/platform/users",
      expect.objectContaining({
        params: expect.objectContaining({ search: "creator" }),
      }),
    ),
  );
  await waitFor(() =>
    expect(screen.getByText("加载更多用户")).toHaveProperty("disabled", false),
  );
  fireEvent.click(screen.getByText("加载更多用户"));
  await waitFor(() =>
    expect(request).toHaveBeenLastCalledWith(
      "/api/platform/users",
      expect.objectContaining({
        params: expect.objectContaining({ cursor: "next" }),
      }),
    ),
  );
});
it("permission loss clears private directory while retaining retry", async () => {
  vi.mocked(request)
    .mockResolvedValueOnce({
      items: [
        {
          id: "user",
          email: "private@example.com",
          display_name: "Creator",
          status: "active",
          platform_role: "creator",
          active_team_count: 1,
        },
      ],
      next_cursor: null,
    })
    .mockRejectedValue(new RequestError("forbidden", 403));
  render(<UserDirectory />);
  await screen.findByText("private@example.com");
  fireEvent.click(screen.getByText("刷新"));
  await screen.findByRole("heading", { name: "暂无访问权限" });
  expect(screen.queryByText("private@example.com")).toBeNull();
  expect(screen.getByText("刷新")).toBeTruthy();
});
it("empty platform directory reports no invented users", async () => {
  vi.mocked(request).mockResolvedValue({ items: [], next_cursor: null });
  render(<UserDirectory />);
  expect(
    await screen.findByRole("heading", { name: "没有匹配的用户" }),
  ).toBeTruthy();
});
