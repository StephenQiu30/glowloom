import { cleanup, fireEvent, screen } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { render } from "../../../../feedback";
import { ModelConfigurationCheck } from "@/app/models/_components/model-configuration-check";

const api = vi.hoisted(() => ({
  configuration: vi.fn(),
  probe: vi.fn(),
  receipt: vi.fn(),
}));
vi.mock("@/components/auth/session", () => ({
  useSession: () => ({ actor: { id: "test-admin" } }),
}));
vi.mock("@/api/settings", () => ({
  checkModelConfiguration: api.configuration,
  testAdminCredential: api.probe,
  getProviderCheck: api.receipt,
  getConfigReceipt: vi.fn(),
}));
afterEach(cleanup);

it("一键验证无价格草稿，仅调用配置检查及只读凭据诊断", async () => {
  api.configuration.mockResolvedValue({
    provider_id: "provider",
    config_version_id: "config",
    model_version_id: "version",
    credential_id: "fixture-key",
    provider_model_id: "fixture-model",
    parameters_valid: true,
  });
  api.probe.mockResolvedValue({
    accepted: true,
    provider_id: "provider",
    check_id: "fixture-receipt",
  });
  api.receipt.mockResolvedValue({
    current: true,
    config_version_id: "config",
    credential_id: "fixture-key",
    status: "succeeded",
    auth_result: "ok",
    discovery_result: "ok",
    candidates: [{ provider_model_id: "fixture-model" }],
  });
  render(
    <ModelConfigurationCheck
      id="draft"
      provider="provider"
      config="config"
      version="version"
    />,
  );
  fireEvent.click(screen.getByRole("button", { name: "验证配置" }));
  await screen.findByText("参数通过 · 目录可见");
  expect(api.configuration).toHaveBeenCalledExactlyOnceWith({ id: "draft" });
  expect(api.probe).toHaveBeenCalledExactlyOnceWith(
    { id: "provider", credential_id: "fixture-key" },
    { headers: { "Idempotency-Key": expect.any(String) } },
  );
});
