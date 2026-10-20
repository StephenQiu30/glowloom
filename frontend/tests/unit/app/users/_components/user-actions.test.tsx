import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { UserActions } from "@/app/users/_components/user-actions";
const detail = {
  user: {
    id: "target",
    status: "active",
    revision: "9",
    email: "target@example.com",
    display_name: "Target",
    platform_role: "creator",
    active_team_count: 0,
    avatar_asset_id: "",
    create_time: "2026-10-15T00:00:00Z",
    email_verified_at: "2026-10-15T00:00:00Z",
    last_login_at: "2026-10-15T00:00:00Z",
  },
  allowed_actions: ["password_reset", "disable"],
  teams: [],
} satisfies API.PlatformUserDetailView;
afterEach(cleanup);
it("只显示服务端当前允许的工作流，密码恢复不设置临时密码", () => {
  const choose = vi.fn();
  render(<UserActions detail={detail} onAction={choose} />);
  expect(screen.getByText("禁用账号")).toBeTruthy();
  expect(screen.queryByText("启用账号")).toBeNull();
  expect(screen.queryByLabelText("临时密码")).toBeNull();
  fireEvent.click(screen.getByText("发送密码恢复通知"));
  expect(choose).toHaveBeenCalledWith(
    expect.objectContaining({
      reason: true,
      title: "发送密码恢复通知",
      effect: expect.stringContaining("邮件队列"),
    }),
  );
});
it("注销账号即使残留动作字段也不显示恢复或启用", () => {
  render(
    <UserActions
      detail={{
        ...detail,
        user: { ...detail.user, status: "deleted" },
        allowed_actions: ["enable", "password_reset"],
      }}
      onAction={vi.fn()}
    />,
  );
  expect(screen.queryByText("启用账号")).toBeNull();
  expect(screen.getByText("当前账号没有可用管理动作。")).toBeTruthy();
});
