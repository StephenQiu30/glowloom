import { beforeEach, describe, expect, it, vi } from "vitest";
vi.mock("@/gen/api/auth", () => ({
  currentUser: vi.fn(),
  login: vi.fn(),
  logout: vi.fn(),
  changePassword: vi.fn(),
}));
import * as api from "@/gen/api/auth";
import { ApiError } from "@/lib/request";
import { changePassword, getCurrentSession, login } from "./queries";
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
beforeEach(() => vi.clearAllMocks());
describe("身份服务边界", () => {
  it("登录保留强制改密状态且会话仅取自服务", async () => {
    vi.mocked(api.login).mockResolvedValue(session);
    expect(await login("synthetic-user", "synthetic-password")).toEqual(
      session,
    );
    expect(api.login).toHaveBeenCalledWith({
      login_name: "synthetic-user",
      password: "synthetic-password",
    });
  });
  it("拒绝畸形响应且不把失效会话转成演示账号", async () => {
    vi.mocked(api.currentUser).mockResolvedValue({ user: {} });
    await expect(getCurrentSession()).rejects.toMatchObject({
      status: 502,
      code: "invalid_response",
    });
    vi.mocked(api.currentUser).mockRejectedValue(
      new ApiError(401, "unauthenticated"),
    );
    await expect(getCurrentSession()).rejects.toMatchObject({ status: 401 });
  });
  it("改密使用独立幂等键并校验服务确认", async () => {
    vi.mocked(api.changePassword).mockResolvedValue({
      must_change_password: false,
      revision: 2,
    });
    await expect(
      changePassword("old-test", "new-test", "key"),
    ).resolves.toMatchObject({ revision: 2 });
    expect(api.changePassword).toHaveBeenCalledWith(
      { current_password: "old-test", new_password: "new-test" },
      { headers: { "Idempotency-Key": "key" } },
    );
  });
});
