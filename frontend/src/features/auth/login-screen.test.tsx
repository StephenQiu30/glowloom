import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
const mocks = vi.hoisted(() => ({
  replace: vi.fn(),
  getCurrentSession: vi.fn(),
  login: vi.fn(),
  changePassword: vi.fn(),
}));
vi.mock("next/navigation", () => ({
  useRouter: () => ({ replace: mocks.replace }),
  useSearchParams: () => new URLSearchParams("returnTo=/canvas"),
}));
vi.mock("@/components/theme-toggle", () => ({ ThemeToggle: () => null }));
vi.mock("./queries", () => ({
  CURRENT_SESSION_KEY: ["identity", "current-user"],
  getCurrentSession: mocks.getCurrentSession,
  login: mocks.login,
  changePassword: mocks.changePassword,
}));
import { ApiError } from "@/lib/request";
import { LoginScreen } from "./login-screen";
const session = {
  user: {
    id: "bdad64d0-6982-474e-a268-208889020cf0",
    org_id: "fdad64d0-6982-474e-a268-208889020cf0",
    login_name: "synthetic-user",
    display_name: "合成测试用户",
    role: "producer",
    must_change_password: true,
  },
  must_change_password: true,
};
beforeEach(() => {
  vi.clearAllMocks();
  mocks.getCurrentSession.mockRejectedValue(
    new ApiError(401, "unauthenticated"),
  );
});
afterEach(cleanup);
function open(cached?: unknown) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  if (cached) client.setQueryData(["identity", "current-user"], cached);
  render(
    <QueryClientProvider client={client}>
      <LoginScreen />
    </QueryClientProvider>,
  );
}
describe("正式登录入口", () => {
  it("登录后必须先修改初始密码，不能跳过服务约束", async () => {
    mocks.login.mockResolvedValue(session);
    open();
    await screen.findByLabelText("账号");
    fireEvent.change(screen.getByLabelText("账号"), {
      target: { value: "synthetic-user" },
    });
    fireEvent.change(screen.getByLabelText("密码"), {
      target: { value: "Synthetic123!" },
    });
    fireEvent.click(screen.getByRole("button", { name: "登录" }));
    await screen.findByRole("heading", { name: "修改初始密码" });
    expect(mocks.replace).not.toHaveBeenCalled();
    fireEvent.change(screen.getByLabelText("当前密码"), {
      target: { value: "Synthetic123!" },
    });
    fireEvent.change(screen.getByLabelText("新密码"), {
      target: { value: "Synthetic456!" },
    });
    fireEvent.change(screen.getByLabelText("再次输入新密码"), {
      target: { value: "Mismatch123!" },
    });
    fireEvent.click(screen.getByRole("button", { name: "更新密码" }));
    expect(screen.getByRole("alert").textContent).toContain("不一致");
    expect(mocks.changePassword).not.toHaveBeenCalled();
  });
  it("正常登录使用服务返回会话进入画布", async () => {
    mocks.login.mockResolvedValue({
      ...session,
      must_change_password: false,
      user: { ...session.user, must_change_password: false },
    });
    open();
    await screen.findByLabelText("账号");
    fireEvent.change(screen.getByLabelText("账号"), {
      target: { value: "synthetic-user" },
    });
    fireEvent.change(screen.getByLabelText("密码"), {
      target: { value: "Synthetic123!" },
    });
    fireEvent.click(screen.getByRole("button", { name: "登录" }));
    await waitFor(() => expect(mocks.replace).toHaveBeenCalledWith("/canvas"));
  });
  it("身份服务不可达时保留失败，不降级为演示账号", async () => {
    mocks.getCurrentSession.mockRejectedValue(
      new ApiError(503, "dependency_unavailable"),
    );
    open();
    await screen.findByRole("alert");
    expect(screen.queryByLabelText("账号")).toBeNull();
    expect(mocks.replace).not.toHaveBeenCalled();
    expect(screen.getByRole("button", { name: "重试连接" })).toBeTruthy();
  });
  it("失效会话的旧缓存不能把登录页重定向回画布", async () => {
    open({
      ...session,
      must_change_password: false,
      user: { ...session.user, must_change_password: false },
    });
    await screen.findByLabelText("账号");
    expect(mocks.replace).not.toHaveBeenCalled();
  });
  it("改密已确认但会话检查失败时，可以重新检查而不重复改密", async () => {
    mocks.getCurrentSession
      .mockResolvedValueOnce(session)
      .mockRejectedValueOnce(new ApiError(503, "dependency_unavailable"))
      .mockResolvedValueOnce({
        ...session,
        must_change_password: false,
        user: { ...session.user, must_change_password: false },
      });
    mocks.changePassword.mockResolvedValue({
      revision: 2,
      must_change_password: false,
    });
    open();
    await screen.findByRole("heading", { name: "修改初始密码" });
    for (const [label, value] of [
      ["当前密码", "Synthetic123!"],
      ["新密码", "Synthetic456!"],
      ["再次输入新密码", "Synthetic456!"],
    ]) {
      fireEvent.change(screen.getByLabelText(label), { target: { value } });
    }
    fireEvent.click(screen.getByRole("button", { name: "更新密码" }));
    await screen.findByRole("alert");
    fireEvent.click(screen.getByRole("button", { name: "重新检查会话" }));
    await waitFor(() => expect(mocks.replace).toHaveBeenCalledWith("/canvas"));
    expect(mocks.changePassword).toHaveBeenCalledTimes(1);
  });
});
