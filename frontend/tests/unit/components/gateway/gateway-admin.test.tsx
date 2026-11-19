import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { GatewayAdmin } from "@/components/gateway/gateway-admin";
import { SpaceContext } from "@/components/spaces/space-context";
import {
  prepareModel,
  prepareRoute,
  fundCredits,
  loadGatewayConfiguration,
  reconcileGateway,
} from "@/lib/ai-gateway-admin";

const send = vi.hoisted(() => vi.fn());
const actor = vi.hoisted(() => ({ role: "platform_admin" }));
vi.mock("@/lib/request", () => ({ request: send }));
vi.mock("@/components/auth/session", () => ({
  useOptionalSession: () => ({
    actor: { id: "admin", platform_role: actor.role },
  }),
}));
const supplier = "11111111-1111-4111-8111-111111111111";
const credential = "22222222-2222-4222-8222-222222222222";
const modelID = "33333333-3333-4333-8333-333333333333";
const verificationID = "44444444-4444-4444-8444-444444444444";
const review = {
  id: verificationID,
  model_id: modelID,
  state: "prepared",
  upstream_model: "supplier-sku",
  scope_id: supplier,
  project_id: credential,
  plan: {
    credits: 2,
    purchase_micros: 500,
    probes: [{ name: "非流式", input: { model: "text/default" } }],
  },
  progress: [],
};
const price = {
  input: 1000000,
  cache_read: 0,
  cache_write: 0,
  output: 2000000,
  request: 0,
  currency: "CNY",
  fx: "1",
  multiplier: "1",
  discount: "1",
};
const model = {
  id: modelID,
  name: "text/default",
  protocol: "chat",
  price,
  max_input: 4096,
  max_output: 1024,
  purchase_cap: 10000000,
  tools: false,
  stream: false,
};
const mediaDriver = {
  adapter_key: "openai",
  base_url: "https://api.openai.com/v1",
  region: "overseas",
  capabilities: ["image"],
  timeout_seconds: 60,
  concurrency_limit: 1,
  rate_limit_per_min: 60,
};
const mediaRoute = {
  model_id: modelID,
  supplier,
  credential,
  group: "image-account",
  base_url: mediaDriver.base_url,
  upstream_model: "gpt-image-1",
  protocol: "media",
  purchase: { ...price, input: 0, output: 0, request: 1000 },
  priority: 0,
  driver_config: mediaDriver,
};
beforeEach(() => {
  vi.clearAllMocks();
  actor.role = "platform_admin";
  send.mockImplementation(async (path: string) =>
    path.endsWith("configuration")
      ? {
          models: [{ model, enabled: true }],
          routes: [],
          credentials: [
            {
              id: credential,
              supplier_id: supplier,
              state: "active",
              key: "must-not-render",
            },
          ],
          quotas: [],
          unknown: [],
        }
      : path.endsWith("/routes") || path.endsWith(verificationID)
        ? review
        : undefined,
  );
});
afterEach(cleanup);

it("does not request gateway configuration for a non-admin account", () => {
  actor.role = "member";
  render(<GatewayAdmin />);
  expect(screen.getByText("暂无访问权限")).toBeTruthy();
  expect(send).not.toHaveBeenCalled();
});

it("starts the provider entry on procurement and asks before discarding an edited form", async () => {
  const space = {
    id: supplier,
    name: "验收空间",
    kind: "personal",
    role: "owner",
    team_id: "",
    writable: true,
  };
  render(
    <SpaceContext.Provider
      value={{
        selected: space,
        spaces: [space],
        pending: false,
        error: "",
        refresh: async () => {},
        select: async () => {},
      }}
    >
      <GatewayAdmin entry="providers" />
    </SpaceContext.Provider>,
  );
  const procurement = await screen.findByRole("tab", {
    name: "供应商与采购价",
  });
  expect(procurement.getAttribute("aria-selected")).toBe("true");
  fireEvent.click(screen.getByRole("button", { name: "准备采购路由" }));
  expect(screen.getByLabelText<HTMLInputElement>("验证付款空间 ID").value).toBe(
    space.id,
  );
  fireEvent.change(screen.getByLabelText("供应商模型号"), {
    target: { value: "edited-model" },
  });
  fireEvent.mouseDown(screen.getByRole("tab", { name: "模型与销售价" }), {
    button: 0,
    ctrlKey: false,
  });
  expect(screen.getByRole("alertdialog")).toBeTruthy();
  fireEvent.click(screen.getByRole("button", { name: "继续填写" }));
  expect(screen.getByLabelText<HTMLInputElement>("供应商模型号").value).toBe(
    "edited-model",
  );
  expect(send.mock.calls.some(([path]) => path.endsWith("/routes"))).toBe(
    false,
  );
});

it("publishes immutable sale versions only through the new gateway, preserving decimal strings", async () => {
  await prepareModel(model);
  await prepareModel(model);
  const first = send.mock.calls[0];
  const second = send.mock.calls[1];
  expect(first[0]).toBe("/api/ai/admin/models");
  expect(first[1].data.price.fx).toBe("1");
  expect(first[1].data.id).not.toBe(modelID);
  expect(second[1].data.id).not.toBe(first[1].data.id);
  await expect(
    prepareModel({ ...model, max_output: 1048577 }),
  ).rejects.toThrow();
  await expect(
    prepareModel({ ...model, price: { ...price, discount: "1.1" } }),
  ).rejects.toThrow();
  await expect(
    prepareModel({ ...model, price: { ...price, input: 0, output: 0 } }),
  ).rejects.toThrow();
});

it("keeps supplier procurement independent from sale and refuses invalid amounts/endpoints", async () => {
  const route = {
    model_id: modelID,
    supplier,
    credential,
    group: "account-a",
    base_url: "https://api.example.com/v1",
    upstream_model: "supplier-sku",
    protocol: "chat",
    purchase: { ...price, input: 200000, output: 500000 },
    priority: 0,
  };
  await prepareRoute(route, { scope: supplier, concurrency: 3, rpm: 60 });
  expect(send.mock.calls[0][0]).toBe("/api/ai/admin/routes");
  expect(send.mock.calls[0][1].data.route.purchase.input).toBe(200000);
  expect(send.mock.calls[0][1].data.route).not.toHaveProperty("price");
  expect(send.mock.calls[0][1].data).not.toHaveProperty("key");
  await expect(
    prepareRoute(
      { ...route, base_url: "http://localhost:1122" },
      { scope: supplier, concurrency: 1, rpm: 1 },
    ),
  ).rejects.toThrow();
  await expect(
    fundCredits(supplier, "invoice-1", 1000000001),
  ).rejects.toThrow();
  await fundCredits(supplier, "invoice-1", 100);
  expect(send.mock.lastCall?.[0]).toBe("/api/ai/admin/credits");
});

it("saves the keyless procurement plan before showing API Key and budget confirmation", async () => {
  render(<GatewayAdmin />);
  await screen.findByText("text/default");
  expect(send.mock.calls[0][0]).toBe("/api/ai/admin/configuration");
  expect(screen.queryByText("must-not-render")).toBeNull();
  fireEvent.mouseDown(screen.getByRole("tab", { name: "供应商与采购价" }), {
    button: 0,
    ctrlKey: false,
  });
  fireEvent.click(screen.getByRole("button", { name: "准备采购路由" }));
  expect(screen.queryByLabelText("API Key")).toBeNull();
  for (const [label, value] of [
    ["验证付款空间 ID", supplier],
    ["共享配额组", "account-a"],
    ["供应商地址", "https://api.example.com/v1"],
    ["供应商模型号", "supplier-sku"],
    ["采购输入价", "0.20"],
  ])
    fireEvent.change(screen.getByLabelText(label), { target: { value } });
  fireEvent.click(screen.getByRole("button", { name: "保存并查看验证预算" }));
  const secret = await screen.findByLabelText<HTMLInputElement>("API Key");
  expect(screen.getByText("¥0.0005")).toBeTruthy();
  expect(screen.getByText("2 积分")).toBeTruthy();
  expect(
    send.mock.calls.find(([path]) => path.endsWith("/routes"))?.[1].data,
  ).not.toHaveProperty("key");
  fireEvent.change(secret, { target: { value: "secret-once" } });
  const verify = screen.getByRole("button", { name: "确认预算并验证" });
  fireEvent.click(verify);
  expect(send.mock.calls.some(([path]) => path.endsWith("/verify"))).toBe(
    false,
  );
  fireEvent.click(
    screen.getByRole("checkbox", { name: "确认以上测试和费用上限" }),
  );
  send.mockImplementation(async (path: string) => {
    if (path.endsWith("/verify")) throw new Error("cannot-save");
    return review;
  });
  fireEvent.click(verify);
  await waitFor(() => expect(secret.value).toBe(""));
  expect(send).toHaveBeenCalledWith(
    `/api/ai/admin/onboarding/${verificationID}/verify`,
    expect.objectContaining({
      data: { key: "secret-once", credits: 2, purchase_micros: 500 },
    }),
  );
  expect(localStorage.length).toBe(0);
  expect(sessionStorage.length).toBe(0);
  expect(screen.queryByText("secret-once")).toBeNull();
});

it("normalizes empty configuration and discards any credential read-back fields", async () => {
  send.mockResolvedValueOnce({
    models: null,
    routes: null,
    credentials: null,
    quotas: null,
    unknown: null,
  });
  expect(await loadGatewayConfiguration()).toEqual({
    models: [],
    routes: [],
    onboarding: [],
    credentials: [],
    quotas: [],
    unknown: [],
    media_drivers: [],
  });
  const configuration = await loadGatewayConfiguration();
  expect(configuration.credentials[0]).toEqual({
    id: credential,
    supplier_id: supplier,
    state: "active",
  });
});

it("requires concrete credit preview before manual funding and keeps business reference", async () => {
  render(<GatewayAdmin />);
  await screen.findByText("text/default");
  fireEvent.mouseDown(screen.getByRole("tab", { name: "积分与对账" }), {
    button: 0,
    ctrlKey: false,
  });
  fireEvent.change(screen.getByLabelText("积分空间 ID"), {
    target: { value: supplier },
  });
  fireEvent.change(screen.getByLabelText("唯一业务凭据"), {
    target: { value: "admin-invoice-001" },
  });
  fireEvent.change(screen.getByLabelText("增加积分"), {
    target: { value: "100" },
  });
  fireEvent.click(screen.getByRole("button", { name: "核对入账信息" }));
  expect(send).toHaveBeenCalledTimes(1);
  expect(screen.getByRole("status").textContent).toContain(supplier);
  expect(screen.getByRole("status").textContent).toContain("100 积分");
  fireEvent.click(screen.getByRole("button", { name: "确认增加积分" }));
  await waitFor(() =>
    expect(send).toHaveBeenCalledWith(
      "/api/ai/admin/credits",
      expect.objectContaining({
        data: { scope_id: supplier, key: "admin-invoice-001", credits: 100 },
      }),
    ),
  );
});

it("rejects contradictory supplier evidence and publishes media only with fixed positive pricing", async () => {
  await expect(
    reconcileGateway({
      request_id: modelID,
      attempt_id: credential,
      reference: "statement-original",
      reason: "核对原单",
      not_charged: true,
      usage: { input: 1, cache_read: 0, cache_write: 0, output: 1 },
    }),
  ).rejects.toThrow();
  await expect(
    prepareModel({
      ...model,
      protocol: "media",
      capability: "image.generate",
      modes: ["text2image"],
      parameters: {},
      price: { ...price, input: 0, output: 0, request: 1000 },
    }),
  ).resolves.toBeUndefined();
  await expect(
    prepareModel({
      ...model,
      protocol: "media",
      capability: "image.generate",
      modes: ["text2image"],
      parameters: {},
    }),
  ).rejects.toThrow();
});

it("prepares a supported media driver with an explicit probe and no key", async () => {
  send.mockResolvedValueOnce({
    models: [
      {
        model: {
          ...model,
          name: "image/default",
          protocol: "media",
          price: mediaRoute.purchase,
          capability: "image.generate",
          modes: ["text2image"],
          parameters: {},
        },
        enabled: true,
      },
    ],
    routes: [],
    credentials: [],
    quotas: [],
    unknown: [],
    media_drivers: [
      {
        key: "openai",
        name: "OpenAI",
        base_url: mediaDriver.base_url,
        region: "overseas",
        parameters: { "image.generate": ["size"] },
        custom_url: true,
      },
    ],
  });
  render(<GatewayAdmin />);
  await screen.findByText("image/default");
  fireEvent.mouseDown(screen.getByRole("tab", { name: "供应商与采购价" }), {
    button: 0,
    ctrlKey: false,
  });
  fireEvent.click(screen.getByRole("button", { name: "准备采购路由" }));
  fireEvent.change(screen.getByLabelText("媒体驱动"), {
    target: { value: "openai" },
  });
  for (const [label, value] of [
    ["验证付款空间 ID", supplier],
    ["验证项目 ID", credential],
    ["共享配额组", mediaRoute.group],
    ["供应商地址", mediaDriver.base_url],
    ["供应商模型号", mediaRoute.upstream_model],
    ["采购按次价", "0.001"],
  ]) {
    fireEvent.change(screen.getByLabelText(label), { target: { value } });
  }
  expect(screen.queryByLabelText(/配置 JSON/)).toBeNull();
  expect(screen.queryByLabelText(/API Key/)).toBeNull();
  fireEvent.change(screen.getByLabelText("验证提示词"), {
    target: { value: "一张白底图" },
  });
  fireEvent.click(screen.getByRole("button", { name: "保存并查看验证预算" }));
  await waitFor(() =>
    expect(send).toHaveBeenCalledWith(
      "/api/ai/admin/routes",
      expect.objectContaining({
        method: "POST",
        data: expect.objectContaining({
          project_id: credential,
          media_tests: [
            expect.objectContaining({
              mode: "text2image",
              prompt: "一张白底图",
            }),
          ],
          route: expect.objectContaining({
            protocol: "media",
            driver_config: mediaDriver,
          }),
        }),
      }),
    ),
  );
  expect(
    send.mock.calls.find(([path]) => path.endsWith("/routes"))?.[1].data,
  ).not.toHaveProperty("key");
});

it("saves a media model with typed modes, parameters and exact prices after collapsing advanced fields", async () => {
  render(<GatewayAdmin />);
  fireEvent.click(await screen.findByRole("button", { name: "新建模型预设" }));
  fireEvent.change(screen.getByLabelText("销售模型名称"), {
    target: { value: "image/preset" },
  });
  fireEvent.change(screen.getByLabelText("模型协议"), {
    target: { value: "media" },
  });
  fireEvent.click(screen.getByRole("button", { name: "添加模式" }));
  fireEvent.change(screen.getByLabelText("生成模式 2", { exact: true }), {
    target: { value: "image2image" },
  });
  const advanced = screen.getByRole("button", { name: "生成参数与默认值" });
  fireEvent.click(advanced);
  fireEvent.click(screen.getByRole("button", { name: "添加参数" }));
  fireEvent.change(screen.getByLabelText("参数名称"), {
    target: { value: "watermark" },
  });
  fireEvent.change(screen.getByLabelText("参数类型"), {
    target: { value: "boolean" },
  });
  fireEvent.change(screen.getByLabelText("默认值", { exact: true }), {
    target: { value: "false" },
  });
  fireEvent.click(advanced);
  fireEvent.change(screen.getByLabelText("销售按次价"), {
    target: { value: "0.001895" },
  });
  expect(screen.queryByLabelText("API Key")).toBeNull();
  fireEvent.click(screen.getByRole("button", { name: "保存并继续采购配置" }));
  await waitFor(() =>
    expect(send).toHaveBeenCalledWith(
      "/api/ai/admin/models",
      expect.objectContaining({
        data: expect.objectContaining({
          protocol: "media",
          capability: "image.generate",
          modes: ["text2image", "image2image"],
          parameters: { watermark: { type: "boolean", default: false } },
          price: expect.objectContaining({
            request: 1895,
            input: 0,
            output: 0,
          }),
        }),
      }),
    ),
  );
});

it.each([
  "apiKey",
  "api_key",
  "key",
  "secret",
  "authorization",
  "password",
  "ciphertext",
])(
  "rejects secret-bearing media configuration field %s before sending",
  async (field) => {
    await expect(
      prepareRoute(
        {
          ...mediaRoute,
          driver_config: { ...mediaDriver, [field]: "synthetic-secret" },
        },
        { scope: supplier, concurrency: 1, rpm: 60 },
      ),
    ).rejects.toThrow();
    expect(send).not.toHaveBeenCalled();
  },
);

it("rejects a secret nested inside a nominally supported driver field", async () => {
  await expect(
    prepareRoute(
      {
        ...mediaRoute,
        driver_config: {
          ...mediaDriver,
          capabilities: [{ apiKey: "synthetic-secret" }],
        },
      },
      { scope: supplier, concurrency: 1, rpm: 60 },
    ),
  ).rejects.toThrow();
  expect(send).not.toHaveBeenCalled();
});
