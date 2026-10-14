import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { UserDialog } from "./user-dialog";
import { RequestError } from "@/lib/request";
const api = vi.hoisted(() => ({
  update: vi.fn(),
  list: vi.fn(),
  disable: vi.fn(),
  enable: vi.fn(),
  create: vi.fn(),
  reset: vi.fn(),
  unlock: vi.fn(),
}));
vi.mock("@/api/identity", () => ({
  updateAdminUser: api.update,
  listAdminUsers: api.list,
  disableAdminUser: api.disable,
  enableAdminUser: api.enable,
  createAdminUser: api.create,
  resetAdminPassword: api.reset,
  unlockAdminUser: api.unlock,
}));
const user = {
  id: "11111111-1111-4111-8111-111111111111",
  login_name: "creator.one",
  display_name: "创作者",
  role: "creator" as const,
  status: "active" as const,
  revision: 8,
};
beforeEach(() => vi.clearAllMocks());
afterEach(cleanup);
it("停用请求携带读取版本，断网后先核对而不重复写入", async () => {
  api.disable.mockRejectedValue(new RequestError("request_unconfirmed"));
  api.list.mockResolvedValue({
    items: [{ ...user, status: "disabled", revision: 9 }],
  });
  const saved = vi.fn();
  render(
    <UserDialog
      action="disable"
      user={user}
      self={false}
      onClose={() => {}}
      onSaved={saved}
    />,
  );
  fireEvent.click(screen.getByRole("button", { name: "停用用户" }));
  await screen.findByRole("alert");
  expect(api.disable.mock.calls[0][1]).toEqual({ expected_revision: 8 });
  expect(
    screen.getByRole("button", { name: "停用用户" }).hasAttribute("disabled"),
  ).toBe(true);
  fireEvent.click(screen.getByRole("button", { name: "核对最新用户信息" }));
  await screen.findByText(/版本 9/);
  expect(api.disable).toHaveBeenCalledOnce();
  expect(saved).not.toHaveBeenCalled();
});
it("版本冲突保留编辑输入，错误回执不能报告成功", async () => {
  api.update.mockResolvedValue({
    id: user.id,
    revision: 10,
    display_name: "修改后",
    role: "creator",
  });
  const saved = vi.fn();
  render(
    <UserDialog
      action="edit"
      user={user}
      self={false}
      onClose={() => {}}
      onSaved={saved}
    />,
  );
  fireEvent.change(screen.getByLabelText("用户显示名"), {
    target: { value: "修改后" },
  });
  fireEvent.click(screen.getByRole("button", { name: "编辑用户" }));
  await screen.findByRole("alert");
  await waitFor(() => expect(saved).not.toHaveBeenCalled());
  expect((screen.getByLabelText("用户显示名") as HTMLInputElement).value).toBe(
    "修改后",
  );
});

it("独立解锁验证未启用停用账号，并保持版本条件", async () => {
  api.unlock.mockResolvedValue({
    id: user.id,
    revision: 9,
    status: "disabled",
    login_unlocked: true,
  });
  const saved = vi.fn();
  render(
    <UserDialog
      action="unlock"
      user={{ ...user, status: "disabled", login_locked: true }}
      self={false}
      onClose={() => {}}
      onSaved={saved}
    />,
  );
  fireEvent.click(screen.getByRole("button", { name: "解除登录锁定" }));
  await waitFor(() => expect(saved).toHaveBeenCalledOnce());
  expect(api.unlock.mock.calls[0][1]).toEqual({ expected_revision: 8 });
  expect(api.enable).not.toHaveBeenCalled();
  expect(api.reset).not.toHaveBeenCalled();
});
