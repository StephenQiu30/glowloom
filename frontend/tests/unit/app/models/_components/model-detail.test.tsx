import {
  cleanup,
  fireEvent,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
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
  previewPrice: vi.fn(),
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
  previewAdminSalePrice: api.previewPrice,
  activateModelRelease: api.activateRelease,
  setAdminModelStatus: api.status,
  getAdminProvider: async () => ({
    provider: { status: "active" },
    credential: { id: "key-a", status: "active" },
  }),
  listProviderChecks: async () => ({ current_checks: [] }),
  getProviderConfig: async () => ({
    id: "config-a",
    config: { adapter_key: "volcengine_ark" },
  }),
  listModelReleases: async () => ({ items: [] }),
  listCapabilityDefaults: async () => ({ items: [] }),
}));

beforeEach(() => vi.resetAllMocks());
afterEach(cleanup);

it("模型配置只显示不可用状态，不展示原因或原因入口", async () => {
  api.model.mockResolvedValue({
    model: {
      id: "model-a",
      display_name: "待发布模型",
      status: "disabled",
      available: false,
      unavailable_reasons: ["disabled", "release_required", "disabled"],
      mode_availability: [
        {
          mode: "text2image",
          available: false,
          unavailable_reasons: [
            "disabled",
            "release_required",
            "mode_unverified",
          ],
        },
      ],
    },
    versions: [],
    prices: [],
  });
  render(<ModelDetail id="model-a" capabilities={[]} onSaved={vi.fn()} />);
  await screen.findByRole("heading", { name: "待发布模型" });
  const status = within(screen.getByLabelText("当前可用性"));
  expect(status.getByText("不可用")).toBeTruthy();
  expect(status.queryByRole("button")).toBeNull();
  expect(status.queryByText(/已停用|未发布|待验证/)).toBeNull();
  expect(status.queryByText(/disabled|text2image|配置状态/)).toBeNull();
  expect(screen.queryByRole("button", { name: "查看原因" })).toBeNull();
});

it("部分模式可用时只显示服务端可用结论", async () => {
  api.model.mockResolvedValue({
    model: {
      id: "model-a",
      display_name: "部分可用模型",
      status: "active",
      available: true,
      mode_availability: [
        { mode: "text2image", available: true },
        {
          mode: "image2image",
          available: false,
          unavailable_reasons: ["mode_unverified"],
        },
      ],
    },
    versions: [],
    prices: [],
  });
  render(<ModelDetail id="model-a" capabilities={[]} onSaved={vi.fn()} />);
  await screen.findByRole("heading", { name: "部分可用模型" });
  const status = within(screen.getByLabelText("当前可用性"));
  expect(status.getByText("可用")).toBeTruthy();
  expect(status.queryByRole("button")).toBeNull();
  expect(status.queryByText(/图生图|待验证|text2image|active/)).toBeNull();
});

it("全部可用时只显示状态，不提供空的原因入口", async () => {
  api.model.mockResolvedValue({
    model: { id: "model-a", display_name: "可用模型", available: true },
    versions: [],
    prices: [],
  });
  render(<ModelDetail id="model-a" capabilities={[]} onSaved={vi.fn()} />);
  await screen.findByRole("heading", { name: "可用模型" });
  const status = within(screen.getByLabelText("当前可用性"));
  expect(status.getByText("可用")).toBeTruthy();
  expect(status.queryByRole("button", { name: "查看原因" })).toBeNull();
});

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

it("模型设置与价格分屏，顶部开关独立于配置草稿", async () => {
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
  expect(screen.queryByRole("tab", { name: "发布" })).toBeNull();
  expect(screen.getByRole("switch", { name: "启用 示例模型" })).toBeTruthy();
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

it("管理员选择同步候选后显式提交原候选和平台倍率", async () => {
  api.model.mockResolvedValue({
    model: { id: "model-a", display_name: "聚合模型", revision: 7 },
    candidate: {
      model: { adapter_key: "openrouter", provider_model_id: "vendor/model" },
    },
    versions: [{ id: "version-a", provider_model_id: "vendor/model" }],
    prices: [],
    price_candidates: [
      {
        id: "price-candidate-a",
        provider_model_id: "vendor/model",
        source_url: "https://openrouter.ai/api/v1/models",
        source_hash: "a".repeat(64),
        parser_version: "openrouter_models_v1",
        source_fetched_at: "2026-11-08T04:00:00Z",
        currency: "USD",
        unit: "per_1k_tokens",
        rule: {
          input_micros_per_1k: 2500,
          output_micros_per_1k: 10000,
        },
        source_pricing: { prompt: "0.0000025", completion: "0.00001" },
        status: "pending_review",
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
  await screen.findByRole("heading", { name: "聚合模型" });
  expect(
    screen.queryByRole("button", { name: "载入候选并人工核验" }),
  ).toBeNull();
  fireEvent.click(screen.getByRole("button", { name: "官方价目同步候选" }));
  fireEvent.click(screen.getByRole("button", { name: "载入候选并人工核验" }));
  expect(
    screen.getByLabelText<HTMLInputElement>("每千输入 Token（USD）").value,
  ).toBe("0.0025");
  fireEvent.change(screen.getByLabelText("兑人民币汇率"), {
    target: { value: "7.2" },
  });
  fireEvent.change(screen.getByLabelText("平台销售倍率"), {
    target: { value: "1.5" },
  });
  fireEvent.change(screen.getByLabelText("销售折扣（折）"), {
    target: { value: "8" },
  });
  api.previewPrice.mockResolvedValue({
    procurement_micros: 72000,
    sale_micros: 86400,
    regular_credits: 2,
    sale_credits: 1,
  });
  fireEvent.click(screen.getByText("价格试算"));
  fireEvent.click(screen.getByRole("button", { name: "试算示例用量" }));
  await screen.findByText(/折前 2 积分 · 折后 1 积分/);
  expect(api.previewPrice.mock.calls[0][0]).toMatchObject({
    discount_factor: "0.8",
    markup_multiplier: "1.5",
    output_tokens: 1000,
  });
  expect(api.savePrice).not.toHaveBeenCalled();

  fireEvent.click(screen.getByRole("button", { name: "核验并保存" }));
  await waitFor(() => expect(api.savePrice).toHaveBeenCalledOnce());
  const submitted = api.savePrice.mock.calls[0][1];
  expect(submitted).toMatchObject({
    candidate_id: "price-candidate-a",
    use_official_rate: true,
    currency: "USD",
    fx_rate_to_cny: "7.2",
    markup_multiplier: "1.5",
    discount_factor: "0.8",
    source_url: "https://openrouter.ai/api/v1/models",
    rule: {
      input_micros_per_1k: 2500,
      output_micros_per_1k: 10000,
    },
  });
});

it("直连价目页候选加载计费单位并要求管理员手动录价", async () => {
  api.model.mockResolvedValue({
    model: { id: "model-a", display_name: "DeepSeek Flash", revision: 3 },
    candidate: {
      model: { adapter_key: "deepseek", provider_model_id: "deepseek-flash" },
    },
    versions: [{ id: "version-a", provider_model_id: "deepseek-flash" }],
    prices: [],
    price_candidates: [
      {
        id: "deepseek-page-candidate",
        provider_model_id: "deepseek-flash",
        source_url: "https://api-docs.deepseek.com/zh-cn/quick_start/pricing/",
        source_hash: "b".repeat(64),
        parser_version: "official_page_review_v1",
        source_fetched_at: "2026-11-09T03:00:00Z",
        currency: "CNY",
        unit: "per_1k_tokens",
        source_pricing: {
          match_status: "matched",
          matched_rows: ["缓存命中输入 | 0.04 元"],
        },
        status: "needs_review",
      },
    ],
  });
  api.savePrice.mockResolvedValue({ id: "deepseek-sale-a" });
  render(
    <ModelDetail
      id="model-a"
      capabilities={[]}
      initialView="pricing"
      onSaved={vi.fn()}
    />,
  );
  await screen.findByRole("heading", { name: "DeepSeek Flash" });
  fireEvent.click(screen.getByRole("button", { name: "官方价目同步候选" }));
  fireEvent.click(screen.getByRole("button", { name: "载入来源并人工录价" }));
  await screen.findByLabelText("每千输入 Token（CNY）");
  fireEvent.change(screen.getByLabelText("每千输入 Token（CNY）"), {
    target: { value: "0.002" },
  });
  fireEvent.change(screen.getByLabelText("每千输出 Token（CNY）"), {
    target: { value: "0.008" },
  });
  fireEvent.click(screen.getByRole("button", { name: "核验并保存" }));
  await waitFor(() => expect(api.savePrice).toHaveBeenCalledOnce());
  const submitted = api.savePrice.mock.calls[0][1];
  expect(submitted).toMatchObject({
    candidate_id: "deepseek-page-candidate",
    use_official_rate: true,
    source_url: "https://api-docs.deepseek.com/zh-cn/quick_start/pricing/",
    currency: "CNY",
    markup_multiplier: "1",
    rule: { input_micros_per_1k: 2000, output_micros_per_1k: 8000 },
  });
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

it("开启 Switch 自动进行只读验证并启用已保存的配置", async () => {
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
      onSaved={vi.fn()}
    />,
  );
  const toggle = await screen.findByRole<HTMLButtonElement>("switch", {
    name: "启用 Seedream",
  });
  await waitFor(() => expect(toggle.disabled).toBe(false));
  expect(toggle.getAttribute("aria-checked")).toBe("false");
  fireEvent.click(toggle);
  expect(toggle.disabled).toBe(true);
  expect(toggle.getAttribute("aria-checked")).toBe("false");
  await waitFor(() =>
    expect(api.activateRelease).toHaveBeenCalledWith(
      { id: "release-a" },
      expect.objectContaining({
        credential_check_id: "check-a",
        expected_model_revision: 5,
        enable_provider: false,
        set_default: false,
      }),
      expect.any(Object),
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
    expect.any(Object),
  );
});

it.each([
  { unit: "per_image", rule: { base_micros: 120000 }, canSave: true },
  { unit: undefined, rule: undefined, canSave: false },
])(
  "展示内置价目，只对已支持的计量预填保存表单",
  async ({ unit, rule, canSave }) => {
    api.model.mockResolvedValue({
      model: { id: "model-a", display_name: "预设模型" },
      versions: [{ id: "version-a" }],
      prices: [],
      price_preset: {
        currency: "CNY",
        source_url: "https://example.com/pricing",
        reference_lines: ["供应商计量和费率"],
        unit,
        rule,
      },
    });
    render(
      <ModelDetail
        id="model-a"
        capabilities={[]}
        initialView="pricing"
        onSaved={vi.fn()}
      />,
    );
    const reference = await screen.findByRole("button", {
      name: "官方计价参考",
    });
    expect(reference.getAttribute("aria-expanded")).toBe("false");
    expect(screen.queryByRole("link", { name: "查看官方价目" })).toBeNull();
    fireEvent.click(reference);
    expect(
      screen.getByRole("link", { name: "查看官方价目" }).getAttribute("href"),
    ).toBe("https://example.com/pricing");
    if (canSave) {
      expect(screen.getByLabelText<HTMLInputElement>("价格（CNY）").value).toBe(
        "0.12",
      );
      expect(
        (
          screen.getByRole("button", {
            name: "核验并保存",
          }) as HTMLButtonElement
        ).disabled,
      ).toBe(false);
    } else {
      expect(screen.getByText(/计量规则尚未接入/)).toBeTruthy();
      expect(screen.queryByRole("button", { name: "核验并保存" })).toBeNull();
    }
  },
);
