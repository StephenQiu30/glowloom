import { cleanup, fireEvent, screen, waitFor } from "@testing-library/react";
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
}));
vi.mock("@/components/auth/session", () => ({
  useSession: () => ({
    actor: { id: "admin-onboarding", platform_role: "platform_admin" },
  }),
}));
vi.mock("@/api/settings", () => ({
  getProviderOnboarding: api.read,
  testProviderOnboardingModel: api.test,
  activateProviderOnboarding: api.activate,
  getConfigReceipt: api.receipt,
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
        capability: "text.generate",
        display_name: "Doubao Seed",
        provider_model_id: "doubao-seed-2-1-pro-260915",
        status: "disabled",
      },
      {
        id: "image",
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
      status: "completed",
      model_result: "completed",
      auth_result: "ok",
    };
    return {
      accepted: true,
      provider_id: "provider",
      check_id: "check",
      model_id: "chat",
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
  await screen.findByRole("button", { name: "验证模型" });
  expect(screen.queryByRole("button", { name: "启用 2 个模型" })).toBeNull();
  fireEvent.click(screen.getByRole("button", { name: "验证模型" }));
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
      status: "failed",
      auth_result: "auth_failed",
    };
    return {
      result: {
        accepted: true,
        provider_id: "provider",
        check_id: "check",
        model_id: "chat",
      },
    };
  });
  setup();
  fireEvent.click(await screen.findByRole("button", { name: "验证模型" }));
  const recover = await screen.findByRole("button", { name: "查询回执" });
  await waitFor(() => expect(recover).toHaveProperty("disabled", false));
  expect(screen.queryByRole("button", { name: "验证模型" })).toBeNull();
  expect(
    screen
      .getByRole("combobox", { name: "模型验证" })
      .getAttribute("data-disabled"),
  ).not.toBeNull();
  expect(screen.queryByText(/credential\.test/)).toBeNull();
  fireEvent.click(recover);
  await screen.findByText("API Key 无效或模型未授权。");
  expect(api.test).toHaveBeenCalledOnce();
  expect(api.activate).not.toHaveBeenCalled();
});
it("视频受理不会被显示成生成完成", async () => {
  state.check = {
    id: "check",
    model_id: "image",
    status: "completed",
    auth_result: "ok",
    model_result: "accepted",
    task_id: "task-fixture",
  };
  setup();
  await screen.findByText(/服务商已受理视频验证任务，视频仍在生成/);
  expect(screen.getByRole("button", { name: "启用 2 个模型" })).toBeDefined();
});
it("检查读取失败有重试入口，不显示可启用", async () => {
  api.read.mockRejectedValueOnce(
    new RequestError("dependency_unavailable", 503),
  );
  setup();
  fireEvent.click(await screen.findByRole("button", { name: "重试" }));
  await screen.findByRole("button", { name: "验证模型" });
  expect(api.activate).not.toHaveBeenCalled();
});

it("已启用连接仍可手动验证，切换模型不会沿用其他模型的成功状态", async () => {
  state.status = "active";
  state.models = state.models!.map((m) => ({ ...m, status: "active" }));
  state.check = {
    id: "old-check",
    model_id: "chat",
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
  fireEvent.click(screen.getByRole("button", { name: "验证模型" }));
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
    status: "completed",
    auth_result: "ok",
    model_result: "completed",
  };
  api.test.mockRejectedValueOnce(new RequestError("request_unconfirmed"));
  setup();
  fireEvent.click(await screen.findByRole("button", { name: "重新验证" }));
  await screen.findByRole("button", { name: "查询回执" });
  expect(screen.getByRole("button", { name: "启用 2 个模型" })).toHaveProperty(
    "disabled",
    true,
  );
  expect(screen.queryByRole("button", { name: "重新验证" })).toBeNull();
  expect(api.test).toHaveBeenCalledOnce();
  expect(api.activate).not.toHaveBeenCalled();
});
