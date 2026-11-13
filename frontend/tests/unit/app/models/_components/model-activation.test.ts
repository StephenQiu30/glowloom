import { beforeEach, expect, it, vi } from "vitest";
import { changeModelActivation } from "@/app/models/_components/model-activation";

const api = vi.hoisted(() => ({
  model: vi.fn(),
  releases: vi.fn(),
  provider: vi.fn(),
  config: vi.fn(),
  checks: vi.fn(),
  check: vi.fn(),
  test: vi.fn(),
  create: vi.fn(),
  activate: vi.fn(),
  status: vi.fn(),
  defaults: vi.fn(),
}));
vi.mock("@/api/settings", () => ({
  getAdminModel: api.model,
  listModelReleases: api.releases,
  getAdminProvider: api.provider,
  getProviderConfig: api.config,
  listProviderChecks: api.checks,
  getProviderCheck: api.check,
  testAdminCredential: api.test,
  createModelRelease: api.create,
  activateModelRelease: api.activate,
  setAdminModelStatus: api.status,
  listCapabilityDefaults: api.defaults,
}));
const oldRelease = {
  id: "live",
  model_version_id: "old-model",
  price_version_id: "old-price",
  config_version_id: "config",
  status: "active",
  revision: 8,
};
const verified = {
  id: "check",
  current: true,
  kind: "credential_auth",
  status: "completed",
  auth_result: "ok",
  adapter_result: "ok",
  config_version_id: "config",
  credential_id: "key",
};
const run = vi.fn(
  async <T>(
    _action: string,
    _target: string,
    execute: (id: string) => Promise<T>,
  ) => execute("request"),
);
const change = (intent: "enable" | "disable" | "update") =>
  changeModelActivation(
    "model",
    intent,
    run as Parameters<typeof changeModelActivation>[2],
    new AbortController().signal,
  );
beforeEach(() => {
  vi.resetAllMocks();
  api.model.mockResolvedValue({
    model: {
      id: "model",
      provider_id: "provider",
      status: "disabled",
      revision: 5,
      capability: "image.generate",
      current_version_id: "new-model",
    },
    versions: [{ id: "new-model" }],
    prices: [
      { id: "new-price", rule: { procurement_price_version_id: "cost" } },
    ],
  });
  api.releases.mockResolvedValue({ items: [oldRelease] });
  api.provider.mockResolvedValue({
    provider: { status: "active" },
    credential: { id: "key", status: "active" },
  });
  api.config.mockResolvedValue({ id: "config" });
  api.defaults.mockResolvedValue({
    items: [{ capability: "image.generate", release_id: "live", revision: 3 }],
  });
  api.checks.mockResolvedValue({ current_checks: [verified] });
  api.activate.mockResolvedValue({ id: "live", status: "active" });
  api.status.mockResolvedValue({
    id: "model",
    status: "disabled",
    revision: 6,
  });
  api.create.mockResolvedValue({ id: "new", revision: 1 });
});

it("重新启用恢复原版本，保留最新模型和价格草稿，不改默认模型", async () => {
  await change("enable");
  expect(api.create).not.toHaveBeenCalled();
  expect(api.test).not.toHaveBeenCalled();
  expect(api.activate).toHaveBeenCalledWith(
    { id: "live" },
    expect.objectContaining({
      expected_revision: 8,
      expected_model_revision: 5,
      expected_active_release_id: "live",
      set_default: false,
      credential_check_id: "check",
    }),
    expect.any(Object),
  );
});

it("应用更新显式绑定新模型和价格，并发切换原默认版本", async () => {
  await change("update");
  expect(api.create).toHaveBeenCalledWith(
    { id: "model" },
    {
      request_id: "request",
      model_version_id: "new-model",
      price_version_id: "new-price",
      config_version_id: "config",
    },
    expect.any(Object),
  );
  expect(api.activate).toHaveBeenCalledWith(
    { id: "new" },
    expect.objectContaining({
      set_default: true,
      expected_default_revision: 3,
    }),
    expect.any(Object),
  );
});

it("首次启用自动执行只读鉴权，通过后再激活", async () => {
  api.releases.mockResolvedValue({ items: [] });
  api.checks.mockResolvedValue({ current_checks: [] });
  api.test.mockResolvedValue({ accepted: true, check_id: "check" });
  api.check.mockResolvedValue(verified);
  await change("enable");
  expect(api.test).toHaveBeenCalledOnce();
  expect(api.check).toHaveBeenCalledWith({ id: "check" }, expect.any(Object));
  expect(api.activate).toHaveBeenCalledOnce();
});

it("价格缺失时拒绝首次启用，不启动验证或写入", async () => {
  api.releases.mockResolvedValue({ items: [] });
  api.model.mockResolvedValue({
    model: { provider_id: "provider", revision: 5 },
    versions: [{ id: "new-model" }],
    prices: [],
  });
  await expect(change("enable")).rejects.toThrow("价格");
  expect(run).not.toHaveBeenCalled();
});

it("连接配置变化不将旧版本偷偷替换成草稿", async () => {
  api.config.mockResolvedValue({ id: "changed-config" });
  await expect(change("enable")).rejects.toThrow("应用更新");
  expect(api.create).not.toHaveBeenCalled();
  expect(api.activate).not.toHaveBeenCalled();
});

it("已启用但不可用的模型仍可停用，不依赖 Key 或连接状态", async () => {
  api.model.mockResolvedValue({
    model: {
      id: "model",
      provider_id: "provider",
      status: "active",
      available: false,
      revision: 7,
    },
  });
  await change("disable");
  expect(api.status).toHaveBeenCalledWith(
    { id: "model" },
    { status: "disabled", expected_revision: 7 },
    expect.objectContaining({ headers: { "Idempotency-Key": "request" } }),
  );
  expect(api.provider).not.toHaveBeenCalled();
  expect(api.test).not.toHaveBeenCalled();
});

it("鉴权失败或结果未确认时不创建和激活版本", async () => {
  api.checks.mockResolvedValue({ current_checks: [] });
  api.test.mockResolvedValue({ accepted: true, check_id: "bad" });
  api.check.mockResolvedValue({ ...verified, auth_result: "failed" });
  await expect(change("enable")).rejects.toThrow("验证未通过");
  expect(api.create).not.toHaveBeenCalled();
  expect(api.activate).not.toHaveBeenCalled();
});
