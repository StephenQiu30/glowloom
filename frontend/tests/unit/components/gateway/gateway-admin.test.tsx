import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { GatewayAdmin } from "@/components/gateway/gateway-admin";
import {
  publishModel,
  publishRoute,
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

it("publishes immutable sale versions only through the new gateway, preserving decimal strings", async () => {
  await publishModel(model);
  await publishModel(model);
  const first = send.mock.calls[0];
  const second = send.mock.calls[1];
  expect(first[0]).toBe("/api/ai/admin/models");
  expect(first[1].data.price.fx).toBe("1");
  expect(first[1].data.id).not.toBe(modelID);
  expect(second[1].data.id).not.toBe(first[1].data.id);
  await expect(
    publishModel({ ...model, max_output: 1048577 }),
  ).rejects.toThrow();
  await expect(
    publishModel({ ...model, price: { ...price, discount: "1.1" } }),
  ).rejects.toThrow();
  await expect(
    publishModel({ ...model, price: { ...price, input: 0, output: 0 } }),
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
  await publishRoute(route, "secret-once", 3, 60);
  expect(send.mock.calls[0][0]).toBe("/api/ai/admin/routes");
  expect(send.mock.calls[0][1].data.route.purchase.input).toBe(200000);
  expect(send.mock.calls[0][1].data.route).not.toHaveProperty("price");
  expect(send.mock.calls[0][1].data.key).toBe("secret-once");
  await expect(
    publishRoute({ ...route, base_url: "http://localhost:1122" }, "x", 1, 1),
  ).rejects.toThrow();
  await expect(
    fundCredits(supplier, "invoice-1", 1000000001),
  ).rejects.toThrow();
  await fundCredits(supplier, "invoice-1", 100);
  expect(send.mock.lastCall?.[0]).toBe("/api/ai/admin/credits");
});

it("loads new configuration, keeps prices separate, and clears write-only secret even on failure", async () => {
  render(<GatewayAdmin />);
  await screen.findByText("text/default");
  expect(send.mock.calls[0][0]).toBe("/api/ai/admin/configuration");
  expect(screen.queryByText("must-not-render")).toBeNull();
  fireEvent.mouseDown(screen.getByRole("tab", { name: "供应商路由" }), {
    button: 0,
    ctrlKey: false,
  });
  fireEvent.change(screen.getByLabelText("供应商 ID"), {
    target: { value: supplier },
  });
  fireEvent.change(screen.getByLabelText("凭据 ID"), {
    target: { value: credential },
  });
  fireEvent.change(screen.getByLabelText("共享配额组"), {
    target: { value: "account-a" },
  });
  fireEvent.change(screen.getByLabelText("供应商地址"), {
    target: { value: "https://api.example.com/v1" },
  });
  fireEvent.change(screen.getByLabelText("供应商模型号"), {
    target: { value: "supplier-sku" },
  });
  const secret = screen.getByLabelText<HTMLInputElement>("供应商密钥");
  fireEvent.change(secret, { target: { value: "secret-once" } });
  fireEvent.change(screen.getByLabelText("采购输入价"), {
    target: { value: "200000" },
  });
  send.mockRejectedValueOnce(new Error("cannot-save"));
  fireEvent.click(screen.getByRole("button", { name: "发布供应商路由" }));
  await waitFor(() => expect(secret.value).toBe(""));
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
    credentials: [],
    quotas: [],
    unknown: [],
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
    publishModel({
      ...model,
      protocol: "media",
      capability: "image.generate",
      modes: ["text2image"],
      parameters: {},
      price: { ...price, input: 0, output: 0, request: 1000 },
    }),
  ).resolves.toBeUndefined();
  await expect(
    publishModel({
      ...model,
      protocol: "media",
      capability: "image.generate",
      modes: ["text2image"],
      parameters: {},
    }),
  ).rejects.toThrow();
});

it("publishes a complete supported media driver from the admin form, keeping adapter_key and dedicated secret separate", async () => {
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
  });
  render(<GatewayAdmin />);
  await screen.findByText("image/default");
  fireEvent.mouseDown(screen.getByRole("tab", { name: "供应商路由" }), {
    button: 0,
    ctrlKey: false,
  });
  for (const [label, value] of [
    ["供应商 ID", supplier],
    ["凭据 ID", credential],
    ["共享配额组", mediaRoute.group],
    ["供应商地址", mediaDriver.base_url],
    ["供应商模型号", mediaRoute.upstream_model],
    ["供应商密钥", "synthetic-dedicated-secret"],
    ["采购按次价", "1000"],
  ]) {
    fireEvent.change(screen.getByLabelText(label), { target: { value } });
  }
  fireEvent.click(screen.getByRole("button", { name: "媒体驱动高级配置" }));
  fireEvent.change(screen.getByLabelText("媒体驱动配置 JSON"), {
    target: { value: JSON.stringify(mediaDriver) },
  });
  fireEvent.click(screen.getByRole("button", { name: "发布供应商路由" }));
  await waitFor(() =>
    expect(send).toHaveBeenCalledWith(
      "/api/ai/admin/routes",
      expect.objectContaining({
        method: "POST",
        data: expect.objectContaining({
          key: "synthetic-dedicated-secret",
          route: expect.objectContaining({
            protocol: "media",
            driver_config: mediaDriver,
          }),
        }),
      }),
    ),
  );
  expect(screen.getByLabelText<HTMLInputElement>("供应商密钥").value).toBe("");
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
      publishRoute(
        {
          ...mediaRoute,
          driver_config: { ...mediaDriver, [field]: "synthetic-secret" },
        },
        "",
        1,
        60,
      ),
    ).rejects.toThrow();
    expect(send).not.toHaveBeenCalled();
  },
);

it("rejects a secret nested inside a nominally supported driver field", async () => {
  await expect(
    publishRoute(
      {
        ...mediaRoute,
        driver_config: {
          ...mediaDriver,
          capabilities: [{ apiKey: "synthetic-secret" }],
        },
      },
      "",
      1,
      60,
    ),
  ).rejects.toThrow();
  expect(send).not.toHaveBeenCalled();
});
