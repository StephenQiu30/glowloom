import { render, findFeedback } from "../../../../tests/feedback";
import { cleanup, fireEvent, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { Settings } from "./account-page";
import { request, RequestError } from "@/lib/request";
vi.mock("@/lib/request", async (original) => ({
  ...(await original<typeof import("@/lib/request")>()),
  request: vi.fn(),
}));
const mocks = vi.hoisted(() => ({
  replace: vi.fn(),
  update: vi.fn(),
  clear: vi.fn(),
  theme: vi.fn(),
}));
vi.mock("next/navigation", () => ({ useRouter: () => mocks }));
vi.mock("next-themes", () => ({ useTheme: () => ({ setTheme: mocks.theme }) }));
vi.mock("@/components/auth/session", () => ({
  useSession: () => session,
  useSessionUpdate: () => mocks.update,
  clearPrivateSession: mocks.clear,
}));
const profile = {
  id: "11111111-1111-4111-8111-111111111111",
  display_name: "Alice",
  email: "alice@example.com",
  revision: "1",
  platform_role: "creator",
};
const session = {
  actor: profile,
  session_id: "22222222-2222-4222-8222-222222222222",
};
let saved = false;
beforeEach(() => {
  saved = false;
  vi.clearAllMocks();
  vi.stubGlobal(
    "ResizeObserver",
    class {
      observe() {}
      unobserve() {}
      disconnect() {}
    },
  );
  vi.mocked(request).mockImplementation(async (url, options) => {
    if (url === "/api/me") {
      if (options?.method === "PATCH") saved = true;
      return saved
        ? { ...profile, display_name: "Updated", revision: "2" }
        : profile;
    }
    if (url === "/api/me/preferences")
      return { revision: "1", theme: "system", prompt_suffix: "initial" };
    if (url === "/api/me/sessions")
      return {
        items: [
          {
            id: session.session_id,
            current: true,
            device_summary: "当前浏览器",
            last_active_at: new Date().toISOString(),
          },
        ],
        as_of: new Date().toISOString(),
      };
    if (url === "/api/me/security-events")
      return { items: [], next_cursor: null, as_of: new Date().toISOString() };
    return undefined;
  });
});
afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});
it("profile saved state requires server readback and preserves explicit missing capabilities", async () => {
  render(<Settings />);
  const name = await screen.findByLabelText("显示名");
  fireEvent.change(name, { target: { value: "Updated" } });
  fireEvent.click(screen.getByText("保存资料"));
  expect(await screen.findByText("个人资料已保存并读回。")).toBeTruthy();
  expect(mocks.update).toHaveBeenCalledWith(
    expect.objectContaining({
      actor: expect.objectContaining({
        display_name: "Updated",
        revision: "2",
      }),
    }),
  );
  expect(screen.getByRole("region", { name: "编辑头像" })).toBeTruthy();
  expect(screen.getByRole("region", { name: "更换邮箱" })).toBeTruthy();
});
it("profile conflict preserves unsaved intent and offers reload", async () => {
  const original = vi.mocked(request).getMockImplementation()!;
  vi.mocked(request).mockImplementation(async (url, options) => {
    if (options?.method === "PATCH")
      throw new RequestError("revision_conflict", 409);
    return original(url, options);
  });
  render(<Settings />);
  const name = await screen.findByLabelText("显示名");
  fireEvent.change(name, { target: { value: "Unsaved" } });
  fireEvent.click(screen.getByText("保存资料"));
  expect(await findFeedback()).toBeTruthy();
  expect(name).toHaveProperty("value", "Unsaved");
  expect(mocks.update).not.toHaveBeenCalled();
});
it("last administrator duties prevent showing confirmation flow", async () => {
  const original = vi.mocked(request).getMockImplementation()!;
  vi.mocked(request).mockImplementation(async (url, options) =>
    url === "/api/me/deletion/preview"
      ? {
          revision: "1",
          team_ids: [],
          blocking_duties: ["last_platform_administrator"],
          policy_version: "soft-delete-v1",
        }
      : original(url, options),
  );
  render(<Settings />);
  await screen.findByLabelText("显示名");
  fireEvent.click(screen.getByText("注销账号"));
  fireEvent.click(screen.getByText("检查注销影响"));
  expect(await screen.findByText("last_platform_administrator")).toBeTruthy();
  expect(screen.queryByText("验证密码并发送验证码")).toBeNull();
  expect(mocks.clear).not.toHaveBeenCalled();
});
it("revocation refreshes directory and security activity contains no private payload", async () => {
  render(<Settings />);
  await screen.findByLabelText("显示名");
  fireEvent.click(screen.getByText("密码与会话"));
  fireEvent.click(screen.getByText("退出其他设备"));
  await waitFor(() =>
    expect(request).toHaveBeenCalledWith(
      "/api/me/sessions/revoke-others",
      expect.objectContaining({ method: "POST" }),
    ),
  );
  expect(await screen.findByText("暂无安全活动。")).toBeTruthy();
});

it("security event failure cannot block loaded profile or leave loading visible", async () => {
  const original = vi.mocked(request).getMockImplementation()!;
  vi.mocked(request).mockImplementation(async (url, options) => {
    if (url === "/api/me/security-events")
      throw new RequestError("temporarily_unavailable", 503);
    return original(url, options);
  });
  render(<Settings />);
  expect(await screen.findByLabelText("显示名")).toHaveProperty(
    "value",
    "Alice",
  );
  expect(screen.queryByText("正在加载账号资料…")).toBeNull();
  fireEvent.click(screen.getByText("密码与会话"));
  expect(await screen.findByText(/安全活动读取失败/)).toBeTruthy();
});

it("清除头像经生成客户端传送 HTTP null 并以本人读回结果更新绑定", async () => {
  let removed = false;
  const original = vi.mocked(request).getMockImplementation()!;
  vi.mocked(request).mockImplementation(async (url, options) => {
    if (url === "/api/me/avatar")
      return {
        url: "https://example.com/avatar",
        expires_at: new Date(Date.now() + 60000).toISOString(),
      };
    if (url === "/api/me") {
      if (options?.method === "PATCH") {
        expect(options.data).toMatchObject({
          avatar_asset_id: null,
          expected_revision: "1",
        });
        removed = true;
      }
      return {
        ...profile,
        avatar_asset_id: removed ? "" : "old-asset",
        revision: removed ? "2" : "1",
      };
    }
    return original(url, options);
  });
  render(<Settings />);
  fireEvent.click(await screen.findByText("恢复默认头像"));
  await screen.findByText("已恢复默认头像。");
  expect(screen.queryByText("恢复默认头像")).toBeNull();
  expect(mocks.update).toHaveBeenCalledWith(
    expect.objectContaining({
      actor: expect.objectContaining({ avatar_asset_id: "", revision: "2" }),
    }),
  );
});
