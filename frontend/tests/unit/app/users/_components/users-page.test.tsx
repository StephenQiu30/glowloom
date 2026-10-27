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
  useSession: () => ({
    actor: { id: "self", platform_role: "platform_admin" },
  }),
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
  fireEvent.click(screen.getByRole("checkbox", { name: "选择当前页全部用户" }));
  expect(screen.getByText("已选 1 / 1 项")).toBeTruthy();
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
  expect(screen.queryByText("已选 1 / 1 项")).toBeNull();
  await waitFor(() =>
    expect(screen.getByRole("button", { name: "下一页" })).toHaveProperty(
      "disabled",
      false,
    ),
  );
  fireEvent.click(screen.getByRole("button", { name: "下一页" }));
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
  expect(await screen.findByText("没有匹配的用户")).toBeTruthy();
});

it("Select 角色和状态的选择进入查询，全部值不会发送给服务端", async () => {
  vi.mocked(request).mockResolvedValue({ items: [], next_cursor: null });
  render(<UserDirectory />);
  await screen.findByText("没有匹配的用户");
  fireEvent.keyDown(screen.getByRole("combobox", { name: "平台角色筛选" }), {
    key: "ArrowDown",
  });
  fireEvent.click(await screen.findByRole("option", { name: "创作者" }));
  fireEvent.keyDown(screen.getByRole("combobox", { name: "用户状态筛选" }), {
    key: "ArrowDown",
  });
  fireEvent.click(await screen.findByRole("option", { name: "禁用" }));
  fireEvent.click(screen.getByRole("button", { name: "查询" }));
  await waitFor(() =>
    expect(request).toHaveBeenLastCalledWith(
      "/api/platform/users",
      expect.objectContaining({
        params: expect.objectContaining({
          platform_role: "creator",
          status: "disabled",
        }),
      }),
    ),
  );
  for (const [label, option] of [
    ["平台角色筛选", "全部角色"],
    ["用户状态筛选", "全部状态"],
  ]) {
    fireEvent.keyDown(screen.getByRole("combobox", { name: label }), {
      key: "ArrowDown",
    });
    fireEvent.click(await screen.findByRole("option", { name: option }));
  }
  fireEvent.click(screen.getByRole("button", { name: "查询" }));
  await waitFor(() =>
    expect(request).toHaveBeenLastCalledWith(
      "/api/platform/users",
      expect.objectContaining({
        params: expect.objectContaining({ platform_role: "", status: "" }),
      }),
    ),
  );
});
