import { render } from "../../../../feedback";
import {
  cleanup,
  fireEvent,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
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

it("高级筛选收起后仍可查询，重置清除条件并返回第一页", async () => {
  vi.mocked(request).mockResolvedValue({ items: [], next_cursor: "next" });
  render(<UserDirectory />);
  await screen.findByText("没有匹配的用户");

  fireEvent.click(screen.getByRole("button", { name: "更多筛选" }));
  const from = "2026-10-01T09:30";
  fireEvent.change(screen.getByLabelText("注册时间起"), {
    target: { value: from },
  });
  fireEvent.click(screen.getByRole("button", { name: "收起筛选" }));
  expect(screen.queryByLabelText("注册时间起")).toBeNull();
  fireEvent.click(screen.getByRole("button", { name: "查询" }));
  await screen.findByRole("button", { name: "更多筛选（1）" });
  expect(request).toHaveBeenLastCalledWith(
    "/api/platform/users",
    expect.objectContaining({
      params: expect.objectContaining({ from: new Date(from).toISOString() }),
    }),
  );

  await screen.findByText("没有匹配的用户");
  fireEvent.click(screen.getByRole("button", { name: "下一页" }));
  await waitFor(() =>
    expect(request).toHaveBeenLastCalledWith(
      "/api/platform/users",
      expect.objectContaining({
        params: expect.objectContaining({ cursor: "next" }),
      }),
    ),
  );
  await waitFor(() =>
    expect(screen.getByRole("button", { name: "重置" })).toHaveProperty(
      "disabled",
      false,
    ),
  );
  fireEvent.click(screen.getByRole("button", { name: "重置" }));
  await waitFor(() => {
    const [, options] = vi.mocked(request).mock.lastCall!;
    expect(options?.params).toMatchObject({ cursor: undefined, search: "" });
    expect(options?.params).not.toHaveProperty("from");
  });
  fireEvent.click(screen.getByRole("button", { name: "更多筛选" }));
  expect(screen.getByLabelText("注册时间起")).toHaveProperty("value", "");
});

it("批量全选排除已注销和当前账号，确认前不发送删除", async () => {
  const users = [
    { id: "active", display_name: "Active", status: "active" },
    { id: "disabled", display_name: "Disabled", status: "disabled" },
    { id: "deleted", display_name: "Deleted", status: "deleted" },
    { id: "self", display_name: "Admin", status: "active" },
  ].map((user) => ({
    email: `${user.id}@example.invalid`,
    platform_role: user.id === "self" ? "platform_admin" : "creator",
    revision: "1",
    ...user,
  }));
  const deleted = new Set<string>();
  vi.mocked(request).mockImplementation(async (path, options) => {
    if (path === "/api/platform/users") {
      return {
        items: users.map((user) =>
          deleted.has(user.id) ? { ...user, status: "deleted" } : user,
        ),
      };
    }
    const user = users.find((user) => path.split("/")[4] === user.id);
    if (options?.method === "POST" && user) {
      deleted.add(user.id);
      return;
    }
    return {
      user,
      allowed_actions:
        user && ["active", "disabled"].includes(user.id) ? ["delete"] : [],
      blocked_actions: { delete: "account_deleted" },
    };
  });
  const writes = () =>
    vi
      .mocked(request)
      .mock.calls.filter(([, options]) => options?.method === "POST");

  render(<UserDirectory />);
  await screen.findByText("active@example.invalid");
  expect(screen.getByRole("checkbox", { name: /选择 Deleted/ })).toHaveProperty(
    "disabled",
    true,
  );
  expect(screen.getByRole("checkbox", { name: /选择 Admin/ })).toHaveProperty(
    "disabled",
    true,
  );
  fireEvent.click(screen.getByRole("checkbox", { name: "选择当前页全部用户" }));
  expect(screen.getByText("已选 2 / 4 项")).toBeTruthy();

  const open = () =>
    fireEvent.click(screen.getByRole("button", { name: "删除账号" }));
  open();
  await screen.findByRole("alertdialog", { name: "删除 2 个账号？" });
  fireEvent.click(screen.getByRole("button", { name: "取消" }));
  expect(writes()).toHaveLength(0);

  open();
  await screen.findByRole("alertdialog", { name: "删除 2 个账号？" });
  expect(writes()).toHaveLength(0);
  fireEvent.click(screen.getByRole("button", { name: "删除" }));
  await waitFor(() => expect(screen.queryByRole("alertdialog")).toBeNull());
  expect(writes().map(([path]) => path)).toEqual([
    "/api/platform/users/active/delete",
    "/api/platform/users/disabled/delete",
  ]);
  await waitFor(() =>
    expect(
      screen.getByRole("checkbox", { name: /选择 Active/ }),
    ).toHaveProperty("disabled", true),
  );
  await waitFor(() => {
    const directory = within(screen.getByRole("table", { name: "用户目录" }));
    expect(directory.getAllByText("已注销")).toHaveLength(3);
    expect(
      screen.getByText(
        "本页没有可批量操作的账号。已注销账号和当前登录账号不可选。",
      ),
    ).toBeTruthy();
    expect(
      directory.getByRole("checkbox", { name: "选择当前页全部用户" }),
    ).toHaveProperty("disabled", true);
  });
});

it("确认单项删除成功后关闭详情并刷新目录", async () => {
  const user = {
    id: "target",
    display_name: "Target",
    email: "target@example.invalid",
    status: "active",
    revision: "1",
  };
  let deleted = false;
  vi.mocked(request).mockImplementation(async (path, options) => {
    if (
      path === "/api/platform/users/target/delete" &&
      options?.method === "POST"
    ) {
      deleted = true;
      return;
    }
    const current = { ...user, status: deleted ? "deleted" : "active" };
    return path === "/api/platform/users"
      ? { items: [current] }
      : { user: current, allowed_actions: deleted ? [] : ["delete"] };
  });
  render(<UserDirectory />);
  fireEvent.click(await screen.findByRole("button", { name: "查看 Target" }));
  fireEvent.click(await screen.findByRole("button", { name: "删除账号" }));
  expect(deleted).toBe(false);
  fireEvent.click(screen.getByRole("button", { name: "删除" }));
  const directory = await screen.findByRole("table", { name: "用户目录" });
  expect(screen.queryByRole("heading", { name: "账号详情" })).toBeNull();
  expect(deleted).toBe(true);
  await within(directory).findByText("已注销");
});
