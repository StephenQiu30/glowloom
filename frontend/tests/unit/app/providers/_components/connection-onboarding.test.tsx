import {
  render as renderWithoutFeedback,
  cleanup,
  fireEvent,
  screen,
  waitFor,
} from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { SWRConfig } from "swr";
import { render } from "../../../../feedback";
import { pageBusinessMemory } from "@/lib/page-business-memory";
import { ConnectionOnboarding } from "@/app/providers/_components/connection-onboarding";
import { RequestError } from "@/lib/request";
const api = vi.hoisted(() => ({
  read: vi.fn(),
  test: vi.fn(),
  activate: vi.fn(),
  receipt: vi.fn(),
  copy: vi.fn(),
}));
vi.mock("@/components/auth/session", () => ({
  useSession: () => ({
    actor: { id: "admin-onboarding", platform_role: "platform_admin" },
  }),
}));
vi.mock("copy-to-clipboard", () => ({ default: api.copy }));
vi.mock("@/api/settings", () => ({
  getProviderOnboarding: api.read,
  testProviderOnboardingModel: api.test,
  activateProviderOnboarding: api.activate,
  getConfigReceipt: api.receipt,
  queryProviderModelCheck: vi.fn(),
  endProviderCheckTracking: vi.fn(),
}));
const saved = vi.fn();
let state: API.ProviderOnboardingState;
beforeEach(() => {
  vi.clearAllMocks();
  pageBusinessMemory.clear();
  state = {
    provider_id: "provider",
    revision: 1,
    status: "disabled",
    credential_id: "credential",
    models: [
      {
        id: "chat",
        version_id: "chat-v1",
        capability: "text.generate",
        display_name: "Doubao Seed",
        provider_model_id: "doubao-seed-2-1-pro-260915",
        status: "disabled",
      },
      {
        id: "image",
        version_id: "image-v1",
        capability: "image.generate",
        display_name: "Seedream",
        status: "disabled",
      },
    ],
  };
  api.read.mockImplementation(async () => structuredClone(state));
  api.test.mockImplementation(async () => {
    state.check = {
      id: "check",
      model_id: "chat",
      model_version_id: "chat-v1",
      status: "completed",
      model_result: "completed",
      auth_result: "ok",
    };
    return {
      accepted: true,
      provider_id: "provider",
      check_id: "check",
      model_id: "chat",
      model_version_id: "chat-v1",
    };
  });
  api.activate.mockImplementation(async () => ({
    ...state,
    status: "active",
    models: state.models!.map((m) => ({ ...m, status: "active" })),
  }));
});
afterEach(cleanup);
function setup() {
  render(
    <SWRConfig
      value={{
        provider: () => new Map(),
        dedupingInterval: 0,
        errorRetryCount: 0,
      }}
    >
      <ConnectionOnboarding id="provider" onSaved={saved} />
    </SWRConfig>,
  );
}
it("验证一次后启用所有所选模型，保存Key本身不能启用", async () => {
  setup();
  await screen.findByRole("button", { name: "准备验证" });
  expect(screen.queryByRole("button", { name: "启用 2 个模型" })).toBeNull();
  fireEvent.click(screen.getByRole("button", { name: "准备验证" }));
  expect(api.test).not.toHaveBeenCalled();
  fireEvent.click(await screen.findByRole("button", { name: "确认并验证" }));
  await screen.findByText("已通过");
  expect(api.test).toHaveBeenCalledOnce();
  expect(api.test).toHaveBeenCalledWith(
    { id: "provider", credential_id: "credential", model_id: "chat" },
    expect.objectContaining({
      headers: { "Idempotency-Key": expect.any(String) },
    }),
  );
  expect(saved).not.toHaveBeenCalled();
  fireEvent.click(screen.getByRole("button", { name: "启用 2 个模型" }));
  await waitFor(() => expect(saved).toHaveBeenCalledWith("provider"));
  expect(api.activate).toHaveBeenCalledWith(
    { id: "provider" },
    expect.objectContaining({ check_id: "check", expected_revision: 1 }),
  );
});
it("未知验证先查询原回执，不重新发起调用", async () => {
  api.test.mockRejectedValueOnce(new RequestError("request_unconfirmed"));
  api.receipt.mockImplementation(async () => {
    state.check = {
      id: "check",
      model_id: "chat",
      model_version_id: "chat-v1",
      status: "failed",
      auth_result: "auth_failed",
    };
    return {
      result: {
        accepted: true,
        provider_id: "provider",
        check_id: "check",
        model_id: "chat",
        model_version_id: "chat-v1",
      },
    };
  });
  setup();
  fireEvent.click(await screen.findByRole("button", { name: "准备验证" }));
  fireEvent.click(await screen.findByRole("button", { name: "确认并验证" }));
  const recover = await screen.findByRole("button", { name: "查询回执" });
  await waitFor(() => expect(recover).toHaveProperty("disabled", false));
  expect(screen.queryByRole("button", { name: "准备验证" })).toBeNull();
  expect(
    screen
      .getByRole("combobox", { name: "模型验证" })
      .getAttribute("data-disabled"),
  ).not.toBeNull();
  expect(screen.queryByText(/credential\.test/)).toBeNull();
  fireEvent.click(recover);
  await screen.findByText("凭据被拒，请核对当前 Key。");
  expect(api.test).toHaveBeenCalledOnce();
  expect(api.activate).not.toHaveBeenCalled();
});
it("视频受理不会被显示成生成完成", async () => {
  state.check = {
    id: "check",
    model_id: "image",
    model_version_id: "image-v1",
    status: "completed",
    auth_result: "ok",
    model_result: "accepted",
    task_id: "task-fixture",
  };
  setup();
  await screen.findByText(/仍在生成，可查询原任务/);
  expect(screen.getByRole("button", { name: "启用 2 个模型" })).toBeDefined();
});
it("检查读取失败有重试入口，不显示可启用", async () => {
  api.read.mockRejectedValueOnce(
    new RequestError("dependency_unavailable", 503),
  );
  setup();
  fireEvent.click(await screen.findByRole("button", { name: "重试" }));
  await screen.findByRole("button", { name: "准备验证" });
  expect(api.activate).not.toHaveBeenCalled();
});

it("已启用连接仍可手动验证，切换模型不会沿用其他模型的成功状态", async () => {
  state.status = "active";
  state.models = state.models!.map((m) => ({ ...m, status: "active" }));
  state.check = {
    id: "old-check",
    model_id: "chat",
    model_version_id: "chat-v1",
    status: "completed",
    auth_result: "ok",
    model_result: "completed",
  };
  setup();
  await screen.findByText("已通过");
  expect(screen.queryByRole("button", { name: "启用 2 个模型" })).toBeNull();
  fireEvent.keyDown(screen.getByRole("combobox", { name: "模型验证" }), {
    key: "ArrowDown",
  });
  fireEvent.click(await screen.findByRole("option", { name: "Seedream" }));
  expect(screen.queryByText("已通过")).toBeNull();
  fireEvent.click(screen.getByRole("button", { name: "准备验证" }));
  expect(api.test).not.toHaveBeenCalled();
  fireEvent.click(await screen.findByRole("button", { name: "确认并验证" }));
  await waitFor(() =>
    expect(api.test).toHaveBeenCalledWith(
      { id: "provider", credential_id: "credential", model_id: "image" },
      expect.anything(),
    ),
  );
  expect(api.activate).not.toHaveBeenCalled();
});

it("再次验证结果未知时冻结模型和启用，只查询原回执", async () => {
  state.check = {
    id: "old",
    model_id: "chat",
    model_version_id: "chat-v1",
    status: "completed",
    auth_result: "ok",
    model_result: "completed",
  };
  api.test.mockRejectedValueOnce(new RequestError("request_unconfirmed"));
  setup();
  fireEvent.click(await screen.findByRole("button", { name: "准备验证" }));
  fireEvent.click(await screen.findByRole("button", { name: "确认并验证" }));
  await screen.findByRole("button", { name: "查询回执" });
  expect(screen.getByRole("button", { name: "启用 2 个模型" })).toHaveProperty(
    "disabled",
    true,
  );
  expect(screen.queryByRole("button", { name: "准备验证" })).toBeNull();
  expect(api.test).toHaveBeenCalledOnce();
  expect(api.activate).not.toHaveBeenCalled();
});

it("打开与取消费用确认不调用供应商", async () => {
  setup();
  fireEvent.click(await screen.findByRole("button", { name: "准备验证" }));
  expect(api.test).not.toHaveBeenCalled();
  fireEvent.click(screen.getByRole("button", { name: "取消" }));
  expect(api.test).not.toHaveBeenCalled();
  expect(screen.queryByRole("button", { name: "确认并验证" })).toBeNull();
  await waitFor(() =>
    expect(document.activeElement).toBe(
      screen.getByRole("button", { name: "准备验证" }),
    ),
  );
});
it("同步模型结果未知时只允许核查，不能直接重发", async () => {
  state.check = {
    id: "unknown",
    model_id: "chat",
    model_version_id: "chat-v1",
    status: "failed",
    model_result: "unknown",
    auth_result: "unknown",
  };
  setup();
  await screen.findByText("待确认");
  expect(screen.queryByRole("button", { name: "准备验证" })).toBeNull();
  expect(screen.getByRole("button", { name: "查看核查说明" })).toBeDefined();
  expect(api.test).not.toHaveBeenCalled();
});

it("同一模型升级版本后不沿用旧通过", async () => {
  state.models![0].version_id = "chat-v2";
  state.check = {
    id: "old-check",
    model_id: "chat",
    model_version_id: "chat-v1",
    status: "completed",
    auth_result: "ok",
    model_result: "completed",
  };
  setup();
  await screen.findByRole("button", { name: "准备验证" });
  expect(screen.queryByText("已通过")).toBeNull();
  expect(screen.queryByRole("button", { name: "启用 2 个模型" })).toBeNull();
});
it("结束追踪后仍显示结果未知，新验证仍须再次确认费用", async () => {
  state.check = {
    id: "unknown-ended",
    model_id: "chat",
    model_version_id: "chat-v1",
    status: "failed",
    model_result: "unknown",
    auth_result: "unknown",
    tracking_ended_at: "2026-11-01T00:00:00Z",
  };
  setup();
  await screen.findByText("待确认");
  fireEvent.click(screen.getByRole("button", { name: "准备验证" }));
  await screen.findByText(/旧请求费用尚未确认/);
  expect(api.test).not.toHaveBeenCalled();
});

it("未知请求可复制脱敏标识并查看原凭据引用", async () => {
  state.check = {
    id: "unknown",
    request_id: "request-original",
    credential_id: "credential-original",
    model_id: "chat",
    model_version_id: "chat-v1",
    status: "failed",
    model_result: "unknown",
    auth_result: "unknown",
  };
  setup();
  fireEvent.click(await screen.findByRole("button", { name: "查看核查说明" }));
  fireEvent.click(await screen.findByRole("button", { name: "复制请求标识" }));
  expect(api.copy).toHaveBeenCalledWith("request-original");
  expect(screen.getByText(/credential-original/)).toBeDefined();
});

it("管理页验证不依赖画布专用反馈上下文", async () => {
  renderWithoutFeedback(
    <SWRConfig value={{ provider: () => new Map(), dedupingInterval: 0 }}>
      <ConnectionOnboarding id="provider" onSaved={saved} />
    </SWRConfig>,
  );
  await screen.findByRole("button", { name: "准备验证" });
});
