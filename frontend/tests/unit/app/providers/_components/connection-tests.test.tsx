import { render } from "../../../../feedback";
import { cleanup, fireEvent, screen, waitFor } from "@testing-library/react";
import { beforeEach, afterEach, it, expect, vi } from "vitest";
import { SWRConfig } from "swr";
import { ConnectionTests } from "@/app/providers/_components/connection-tests";
const api = vi.hoisted(() => ({
  list: vi.fn(),
  network: vi.fn(),
  auth: vi.fn(),
  query: vi.fn(),
  models: vi.fn(),
}));
vi.mock("@/components/auth/session", () => ({
  useSession: () => ({
    actor: { id: "tester", platform_role: "platform_admin" },
  }),
}));
vi.mock("@/api/settings", () => ({
  listProviderChecks: api.list,
  testProviderReachability: api.network,
  testAdminCredential: api.auth,
  queryProviderModelCheck: api.query,
  listProviderTestModels: api.models,
  getProviderOnboarding: vi.fn().mockResolvedValue({ models: [] }),
  getConfigReceipt: vi.fn(),
}));
beforeEach(() => {
  vi.clearAllMocks();
  api.models.mockResolvedValue([]);
  api.list.mockResolvedValue({
    items: [],
    current_checks: [],
    has_more: false,
  });
});
afterEach(cleanup);
function setup(
  adapter = "openai",
  credential = true,
  onConfigure?: () => void,
) {
  return render(
    <SWRConfig
      value={{
        provider: () => new Map(),
        dedupingInterval: 0,
        errorRetryCount: 0,
      }}
    >
      <ConnectionTests
        id="provider"
        adapter={adapter}
        credentialID={credential ? "credential" : undefined}
        revision={0}
        onConfigure={onConfigure}
        onSaved={() => {}}
      />
    </SWRConfig>,
  );
}
it("打开不调用上游；无凭据仍可检测网络，401仅显示网络可达", async () => {
  api.network.mockImplementation(async () => {
    api.list.mockResolvedValue({
      items: [
        {
          id: "check",
          kind: "reachability",
          current: true,
          status: "completed",
          network_result: "reachable",
          http_status: 401,
          elapsed_ms: 92,
          attempts: 1,
        },
      ],
      current_checks: [
        {
          id: "check",
          kind: "reachability",
          current: true,
          status: "completed",
          network_result: "reachable",
          http_status: 401,
          elapsed_ms: 92,
          attempts: 1,
        },
      ],
    });
    return { accepted: true, provider_id: "provider", check_id: "check" };
  });
  const configure = vi.fn();
  setup("openai", false, configure);
  const network = await screen.findByRole("button", { name: "检测连通" });
  expect(api.network).not.toHaveBeenCalled();
  expect(api.auth).not.toHaveBeenCalled();
  expect(screen.queryByRole("button", { name: "检查凭据" })).toBeNull();
  fireEvent.click(screen.getByRole("button", { name: "配置凭据" }));
  expect(configure).toHaveBeenCalledOnce();
  fireEvent.click(network);
  await screen.findByText("网络可达");
  expect(screen.queryByText(/HTTP 401/)).toBeNull();
  fireEvent.click(screen.getByRole("button", { name: "检测详情" }));
  expect(screen.getByText(/HTTP 401/)).toBeDefined();
  expect(screen.queryByText("鉴权通过")).toBeNull();
});
it("旧凭据的通过只能作为历史记录，不冒充当前状态", async () => {
  api.list.mockResolvedValue({
    items: [
      {
        id: "old",
        kind: "credential_auth",
        status: "completed",
        auth_result: "ok",
        current: false,
        created_at: "2026-10-30T00:00:00Z",
      },
    ],
    current_checks: [],
  });
  setup();
  await screen.findByRole("button", { name: "检查凭据" });
  expect(screen.queryByText("鉴权通过")).toBeNull();
  fireEvent.click(screen.getByRole("button", { name: "检测详情" }));
  await screen.findByText("历史版本");
  expect(api.auth).not.toHaveBeenCalled();
});
it("语音与音乐不把不支持只读鉴权显示为失败或发起假验证", async () => {
  setup("volcengine_music");
  await screen.findByText("需模型验证");
  expect(screen.queryByRole("button", { name: "检查凭据" })).toBeNull();
  expect(api.auth).not.toHaveBeenCalled();
});

it("历史异步任务只提供原任务查询，不重新生成", async () => {
  api.list.mockResolvedValue({
    items: [
      {
        id: "old-task",
        kind: "model_access",
        current: false,
        status: "completed",
        model_result: "accepted",
        task_id: "task-original",
      },
    ],
    current_checks: [],
  });
  api.query.mockResolvedValue({
    accepted: true,
    provider_id: "provider",
    check_id: "query-check",
  });
  setup();
  fireEvent.click(await screen.findByRole("button", { name: "检测详情" }));
  fireEvent.click(
    await screen.findByRole("button", { name: /模型验证 · 已受理/ }),
  );
  fireEvent.click(await screen.findByRole("button", { name: "查询原任务" }));
  await waitFor(() =>
    expect(api.query).toHaveBeenCalledWith(
      { id: "old-task" },
      expect.objectContaining({
        headers: expect.objectContaining({
          "Idempotency-Key": expect.any(String),
        }),
      }),
    ),
  );
  expect(api.network).not.toHaveBeenCalled();
  expect(api.auth).not.toHaveBeenCalled();
});
