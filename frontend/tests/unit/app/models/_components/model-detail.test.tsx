import { cleanup, fireEvent, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { render } from "../../../../feedback";
import { ModelDetail } from "@/app/models/_components/model-detail";
import { RequestError } from "@/lib/request";

const api = vi.hoisted(() => ({
  model: vi.fn(),
  status: vi.fn(),
  check: vi.fn(),
  checkResult: vi.fn(),
  createRelease: vi.fn(),
  savePrice: vi.fn(),
  activateRelease: vi.fn(),
}));
vi.mock("@/components/auth/session", () => ({
  useSession: () => ({ actor: { id: "admin" } }),
}));
vi.mock("@/api/settings", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/api/settings")>()),
  getAdminModel: api.model,
  testAdminCredential: api.check,
  getProviderCheck: api.checkResult,
  createModelRelease: api.createRelease,
  publishAdminModelPrice: api.savePrice,
  activateModelRelease: api.activateRelease,
  setAdminModelStatus: api.status,
  getProviderConfig: async () => ({
    id: "config-a",
    config: { adapter_key: "volcengine_ark" },
  }),
  listModelReleases: async () => ({ items: [] }),
  listCapabilityDefaults: async () => ({ items: [] }),
}));

beforeEach(() => vi.resetAllMocks());
afterEach(cleanup);

it("详情读取失败时结束加载并允许重试同一模型", async () => {
  api.model.mockRejectedValueOnce(
    new RequestError("dependency_unavailable", 503),
  );
  render(<ModelDetail id="model-a" capabilities={[]} onSaved={vi.fn()} />);
  await waitFor(() => expect(screen.queryByText("正在加载配置…")).toBeNull());
  expect(screen.getByRole("alert").textContent).toContain("服务暂不可用");
  api.model.mockResolvedValueOnce({
    model: { id: "model-a", display_name: "待接入模型", status: "disabled" },
    versions: [],
    prices: [],
  });
  fireEvent.click(screen.getByRole("button", { name: "重试加载" }));
  await screen.findByRole("heading", { name: "待接入模型" });
  expect(screen.queryByRole("button", { name: "重试加载" })).toBeNull();
  expect(api.model.mock.calls.map(([input]) => input.id)).toEqual([
    "model-a",
    "model-a",
  ]);
});

it("模型设置、价格和发布分屏，切换后不丢未保存的配置", async () => {
  api.model.mockResolvedValue({
    model: {
      id: "model-a",
      provider_id: "provider-a",
      display_name: "示例模型",
      status: "disabled",
    },
    versions: [{ id: "version-a", provider_model_id: "preset-model-id" }],
    prices: [],
  });
  render(<ModelDetail id="model-a" capabilities={[]} onSaved={vi.fn()} />);
  await screen.findByRole("heading", { name: "示例模型" });
  expect(screen.queryByRole("button", { name: "归档模型" })).toBeNull();
  expect(screen.queryByRole("button", { name: "验证配置" })).toBeNull();
  fireEvent.change(screen.getByLabelText("上游模型 ID"), {
    target: { value: "draft-model-id" },
  });
  expect(screen.queryByRole("button", { name: "核验并保存" })).toBeNull();
  fireEvent.mouseDown(screen.getByRole("tab", { name: "价格" }), { button: 0 });
  expect(
    await screen.findByRole("button", { name: "核验并保存" }),
  ).toBeTruthy();
  expect(screen.queryByRole("button", { name: "保存配置" })).toBeNull();
  fireEvent.mouseDown(screen.getByRole("tab", { name: "模型设置" }), {
    button: 0,
  });
  expect((screen.getByLabelText("上游模型 ID") as HTMLInputElement).value).toBe(
    "draft-model-id",
  );
});

it("官方费率只核验并保存一次，提交同价规则和同一核验时间", async () => {
  api.model.mockResolvedValue({
    model: { id: "model-a", display_name: "示例预设", revision: 5 },
    candidate: { model: { provider_model_id: "preset-model-id" } },
    versions: [{ id: "version-a", provider_model_id: "preset-model-id" }],
    prices: [
      {
        id: "legacy-rate",
        version_no: 4,
        unit: "per_image",
        currency: "CNY",
        source_url: "https://example.com/pricing",
        reviewed_at: "2026-11-06T00:00:00Z",
        rule: { base_micros: 120000, markup_multiplier: 1.5 },
      },
    ],
  });
  api.savePrice.mockResolvedValue({ id: "sale-a" });
  render(
    <ModelDetail
      id="model-a"
      capabilities={[]}
      initialView="pricing"
      onSaved={vi.fn()}
    />,
  );
  const save = await screen.findByRole("button", { name: "核验并保存" });
  expect(screen.getByLabelText<HTMLInputElement>("价格（CNY）").value).toBe(
    "0.12",
  );
  expect(screen.getByLabelText<HTMLInputElement>("官方价格来源").value).toBe(
    "https://example.com/pricing",
  );
  expect(screen.queryByRole("tab", { name: "销售价" })).toBeNull();
  expect(screen.queryByLabelText("核验时间")).toBeNull();
  expect(screen.queryByText("v4")).toBeNull();
  fireEvent.click(save);
  await waitFor(() => expect(api.savePrice).toHaveBeenCalledOnce());
  const submitted = api.savePrice.mock.calls[0][1];
  expect(submitted).toMatchObject({
    use_official_rate: true,
    source_url: "https://example.com/pricing",
    rule: { base_micros: 120000 },
    effective_from: expect.any(String),
  });
  expect(submitted.rule).toEqual({ base_micros: 120000 });
  expect(submitted.reviewed_at).toBe(submitted.effective_from);
});

it("已归档模型可恢复，恢复后才可修改配置", async () => {
  let model = {
    id: "model-a",
    display_name: "示例模型",
    status: "archived",
    revision: 2,
  };
  api.model.mockImplementation(async () => ({
    model,
    versions: [],
    prices: [],
  }));
  api.status.mockImplementation(async (_path, body) => {
    model = { ...model, status: body.status, revision: model.revision + 1 };
    return model;
  });
  render(<ModelDetail id="model-a" capabilities={[]} onSaved={vi.fn()} />);
  const restore = await screen.findByRole("button", { name: "恢复模型" });
  expect(screen.queryByRole("tab", { name: "模型设置" })).toBeNull();
  fireEvent.click(restore);
  fireEvent.click(screen.getByRole("button", { name: "确认恢复" }));
  await screen.findByRole("tab", { name: "模型设置" });
  expect(api.status.mock.calls.map(([, body]) => body)).toEqual([
    { status: "disabled", expected_revision: 2 },
  ]);
});

it("通过当前 Key 的只读验证后使用已保存的配置和价格发布", async () => {
  api.model.mockResolvedValue({
    model: {
      id: "model-a",
      provider_id: "provider-a",
      display_name: "Seedream",
      status: "disabled",
      revision: 5,
    },
    versions: [{ id: "version-a" }],
    prices: [
      { id: "price-a", rule: { procurement_price_version_id: "supplier-a" } },
    ],
  });
  api.check.mockResolvedValue({
    accepted: true,
    provider_id: "provider-a",
    check_id: "check-a",
  });
  api.checkResult.mockResolvedValue({
    id: "check-a",
    kind: "credential_auth",
    status: "completed",
    current: true,
    auth_result: "ok",
    adapter_result: "ok",
    config_version_id: "config-a",
    credential_id: "key-a",
  });
  api.createRelease.mockResolvedValue({ id: "release-a", revision: 1 });
  api.activateRelease.mockResolvedValue({ id: "release-a", status: "active" });

  render(
    <ModelDetail
      id="model-a"
      capabilities={[]}
      providers={[
        {
          provider: { id: "provider-a", status: "disabled" },
          credential: { id: "key-a", status: "active" },
        },
      ]}
      initialView="release"
      onSaved={vi.fn()}
    />,
  );
  const publish = await screen.findByRole<HTMLButtonElement>("button", {
    name: "发布模型",
  });
  expect(publish.disabled).toBe(true);
  expect(screen.queryByText("发布记录")).toBeNull();
  expect(screen.queryByText("扣费项目（必选）")).toBeNull();
  const verify = screen.getByRole<HTMLButtonElement>("button", {
    name: "验证 Key 和服务",
  });
  await waitFor(() => expect(verify.disabled).toBe(false));
  fireEvent.click(verify);
  await screen.findByText("Key 有效，服务可访问。");
  expect(api.check).toHaveBeenCalledWith(
    { id: "provider-a", credential_id: "key-a" },
    expect.objectContaining({ headers: expect.any(Object) }),
  );
  expect(api.createRelease).not.toHaveBeenCalled();

  fireEvent.click(publish);
  await waitFor(() =>
    expect(api.activateRelease).toHaveBeenCalledWith(
      { id: "release-a" },
      expect.objectContaining({
        credential_check_id: "check-a",
        expected_model_revision: 5,
        enable_provider: true,
        set_default: false,
      }),
    ),
  );
  expect(api.createRelease).toHaveBeenCalledWith(
    { id: "model-a" },
    {
      request_id: expect.any(String),
      model_version_id: "version-a",
      price_version_id: "price-a",
      config_version_id: "config-a",
    },
  );
});
