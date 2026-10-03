import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { ApiError } from "@/lib/request";
import { AdminFailure } from "./admin-ui";
import { SettingsCredentials } from "./settings-credentials";
import type { ProviderDetail } from "./admin-queries";

const api = vi.hoisted(() => ({
  setAdminCredential: vi.fn(),
  disableAdminCredential: vi.fn(),
  testAdminCredential: vi.fn(),
}));
vi.mock("@/api/settings", async (original) => ({
  ...(await original<typeof import("@/api/settings")>()),
  ...api,
}));
const id = "10000000-0000-4000-8000-000000000001";
const time = "2026-10-01T00:00:00Z";
const detail: ProviderDetail = {
  provider: {
    id,
    key: "fixture",
    name: "测试渠道",
    adapter_key: "mock",
    region: "domestic",
    status: "active",
    concurrency_limit: 1,
    rate_limit_per_min: 10,
    revision: 1,
    create_time: time,
    update_time: time,
  },
  credential: {
    id,
    provider_id: id,
    label: "原凭据",
    last4: "1234",
    status: "active",
    last_test_result: null,
    last_tested_at: null,
    create_time: time,
    update_time: time,
  },
  credential_schema: [{ name: "api_key", type: "string", required: true }],
};
beforeEach(() => vi.clearAllMocks());
afterEach(cleanup);
describe("设置权限与凭据状态", () => {
  it("403 保留实际权限状态", () => {
    render(<AdminFailure error={new ApiError(403, "forbidden")} />);
    expect(screen.getByRole("alert").textContent).toContain("需要管理员权限");
    expect(screen.queryByRole("button", { name: "授权" })).toBeNull();
  });
  it("202 只展示已受理，不冒充测试通过", async () => {
    api.testAdminCredential.mockResolvedValue({
      accepted: true,
      provider_id: id,
      credential_id: id,
      event_id: id,
      test_id: id,
    });
    render(<SettingsCredentials detail={detail} refresh={vi.fn()} />);
    fireEvent.click(screen.getByRole("button", { name: "测试连接" }));
    await waitFor(() =>
      expect(screen.getByRole("status").textContent).toContain(
        "尚未取得验证结果",
      ),
    );
    expect(screen.queryByText("验证通过")).toBeNull();
  });
  it("保存失败后清空一次性凭据，错误不包含输入", async () => {
    api.setAdminCredential.mockRejectedValue(
      new ApiError(503, "dependency_unavailable"),
    );
    render(<SettingsCredentials detail={detail} refresh={vi.fn()} />);
    fireEvent.change(screen.getByLabelText("api_key"), {
      target: { value: "synthetic-secret-for-ui-test" },
    });
    fireEvent.click(screen.getByRole("button", { name: "保存新凭据" }));
    await waitFor(() =>
      expect(screen.getByLabelText("api_key")).toHaveProperty("value", ""),
    );
    expect(screen.getByRole("alert").textContent).not.toContain(
      "synthetic-secret-for-ui-test",
    );
    expect(api.setAdminCredential.mock.calls[0][1].secret).toEqual({
      api_key: "synthetic-secret-for-ui-test",
    });
  });
});
