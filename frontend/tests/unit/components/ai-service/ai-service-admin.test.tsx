import {
  cleanup,
  fireEvent,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { render, findFeedback } from "../../../feedback";
import { RequestError } from "@/lib/request";
import { AiServiceAdmin } from "@/components/ai-service/ai-service-admin";
import { SpaceContext } from "@/components/spaces/space-context";
import {
  arkEmbedding,
  prepareModel,
  prepareRoute,
  submitCreditGrant,
  loadAiServiceConfiguration,
  reconcileGateway,
} from "@/lib/ai-service-admin";

const send = vi.hoisted(() => vi.fn());
const actor = vi.hoisted(() => ({ role: "platform_admin" }));
vi.mock("@/lib/request", async (loadOriginal) => ({
  ...(await loadOriginal<typeof import("@/lib/request")>()),
  request: send,
}));
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

async function chooseOption(label: string, name: string | RegExp) {
  const trigger = screen.getByRole("combobox", { name: label });
  fireEvent.keyDown(trigger, { key: "ArrowDown" });
  const option = await screen.findByRole("option", { name });
  fireEvent.keyDown(option, { key: "Enter" });
}

it("opens a prepared model at API Key verification without requesting manual pricing", async () => {
  send.mockResolvedValueOnce({
    models: [{ model, enabled: false, draft: true }],
    onboarding: [review],
    routes: [],
    credentials: [],
    quotas: [],
    unknown: [],
  });
  render(<AiServiceAdmin entry="providers" />);
  fireEvent.click(await screen.findByRole("button", { name: "新增接入" }));
  const dialog = screen.getByRole("dialog", { name: "新增接入" });
  fireEvent.click(
    within(dialog).getByRole("option", { name: /^text\/default/ }),
  );

  expect(await within(dialog).findByLabelText("API Key")).toBeTruthy();
  expect(screen.getByRole("dialog", { name: "新增接入" })).toBe(dialog);
  expect(document.querySelector('[aria-current="step"]')).toBeNull();
  expect(
    within(dialog).queryByRole("listbox", { name: "模型列表" }),
  ).toBeNull();
  expect(within(dialog).getByText("待验证")).toBeTruthy();
  expect(within(dialog).queryByRole("button", { name: "验证详情" })).toBeNull();
  expect(
    within(dialog).getByRole("checkbox", {
      name: "确认生成试跑，最多使用 2 积分",
    }),
  ).toBeTruthy();
  expect(screen.queryByLabelText("销售输入价")).toBeNull();
  expect(screen.queryByLabelText("采购输入价")).toBeNull();
  expect(
    screen.getByRole<HTMLButtonElement>("button", {
      name: "确认并试跑",
    }).disabled,
  ).toBe(true);
  expect(send.mock.calls.map(([path]) => path)).toEqual([
    "/api/ai/admin/configuration",
    `/api/ai/admin/onboarding/${verificationID}`,
  ]);
  fireEvent.click(within(dialog).getByRole("button", { name: "更换型号" }));
  expect(
    within(dialog).getByRole("listbox", { name: "模型列表" }),
  ).toBeTruthy();
  expect(within(dialog).queryByLabelText("API Key")).toBeNull();
});

it("reuses a serving supplier model without requesting a key or sending a probe", async () => {
  const active = { ...review, state: "active", available: true };
  send
    .mockResolvedValueOnce({
      models: [{ model, enabled: true }],
      onboarding: [active],
      routes: [
        {
          route: {
            id: verificationID,
            model_id: modelID,
            upstream_model: review.upstream_model,
            credential,
            group: "account",
            purchase: price,
          },
          enabled: true,
        },
      ],
      credentials: [{ id: credential, supplier_id: supplier, state: "active" }],
    })
    .mockResolvedValueOnce(active);
  render(<AiServiceAdmin entry="providers" />);
  fireEvent.click(await screen.findByRole("button", { name: "新增接入" }));
  const dialog = screen.getByRole("dialog", { name: "新增接入" });
  fireEvent.click(
    within(dialog).getByRole("option", { name: /^text\/default.*可用/ }),
  );
  expect(await within(dialog).findByText("可用")).toBeTruthy();
  expect(within(dialog).queryByLabelText("API Key")).toBeNull();
  expect(send.mock.calls.map(([path]) => path)).toEqual([
    "/api/ai/admin/configuration",
    `/api/ai/admin/onboarding/${verificationID}`,
  ]);
});

it("groups channels by supplier and keeps reconciliation in platform billing", async () => {
  const configuration = {
    models: [{ model, enabled: true }],
    routes: [
      {
        route: { ...mediaRoute, id: verificationID },
        provider_key: "volcengine_ark",
        enabled: true,
      },
    ],
    onboarding: [
      {
        ...review,
        id: "pending",
        state: "failed",
        upstream_model: "pending-model",
        provider_key: "volcengine_ark",
        supplier_id: supplier,
        group: "image-account",
      },
    ],
    quotas: [{ name: "image-account", concurrency: 2, rpm: 60 }],
    credentials: [],
    unknown: [
      {
        request_id: modelID,
        attempt_id: verificationID,
        scope_id: supplier,
        state: "unknown",
      },
    ],
  };
  send.mockResolvedValue(configuration);
  const entry = render(<AiServiceAdmin entry="providers" />);
  const table = await screen.findByRole("table", { name: "供应商" });
  expect(
    within(table)
      .getByRole("link", { name: "查看 火山方舟" })
      .getAttribute("href"),
  ).toBe("/providers?provider=volcengine_ark");
  expect(screen.queryByText("pending-model")).toBeNull();
  const nav = screen.getByRole("navigation", { name: "AI 服务管理" });
  expect(
    within(nav)
      .getAllByRole("link")
      .map((link) => link.textContent),
  ).toEqual(["模型", "渠道"]);
  expect(screen.queryByRole("tab", { name: "对账" })).toBeNull();

  entry.rerender(
    <AiServiceAdmin entry="providers" initialProvider="volcengine_ark" />,
  );
  const routes = await screen.findByRole("table", { name: "供应商路由" });
  expect(
    within(routes).getByRole("link", { name: "查看路由" }).getAttribute("href"),
  ).toBe(`/providers/${verificationID}`);
  expect(screen.getByRole("table", { name: "接入记录" })).toBeTruthy();
  expect(screen.getByText("pending-model")).toBeTruthy();
  expect(screen.getByRole("button", { name: "查看回执" })).toBeTruthy();

  entry.rerender(<AiServiceAdmin entry="billing" />);
  expect(await screen.findByRole("table", { name: "积分赠送单" })).toBeTruthy();
  expect(screen.queryByRole("navigation", { name: "AI 服务管理" })).toBeNull();
  fireEvent.mouseDown(screen.getByRole("tab", { name: "对账" }), {
    button: 0,
    ctrlKey: false,
  });
  fireEvent.click(screen.getByRole("button", { name: "核对账单" }));
  expect(screen.getByLabelText<HTMLInputElement>("原请求 ID").value).toBe(
    modelID,
  );
  expect(screen.getByLabelText<HTMLInputElement>("原尝试 ID").value).toBe(
    verificationID,
  );
});

it("does not request gateway configuration for a non-admin account", () => {
  actor.role = "member";
  render(<AiServiceAdmin />);
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
      <AiServiceAdmin entry="providers" />
    </SpaceContext.Provider>,
  );
  const procurement = await screen.findByRole("link", { name: "渠道" });
  expect(procurement.getAttribute("aria-current")).toBe("page");
  fireEvent.click(screen.getByRole("button", { name: "新增接入" }));
  fireEvent.click(screen.getByRole("button", { name: "使用已有模型" }));
  expect(screen.getByLabelText<HTMLInputElement>("验证付款空间 ID").value).toBe(
    space.id,
  );
  fireEvent.change(screen.getByLabelText("供应商模型号"), {
    target: { value: "edited-model" },
  });
  fireEvent.click(screen.getByRole("button", { name: "关闭对话框" }));
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

it.each([
  { concurrency: 1, rpm: 60 },
  { concurrency: 2, rpm: 1 },
])(
  "explains when a serving quota group cannot admit a verification (%j)",
  async (limits) => {
    const group = "serving-account";
    send.mockResolvedValueOnce({
      models: [{ model, enabled: true }],
      routes: [
        {
          enabled: true,
          route: {
            id: verificationID,
            model_id: modelID,
            supplier,
            credential,
            group,
            base_url: "https://api.example.com/v1",
            upstream_model: "supplier-sku",
            protocol: "chat",
            purchase: price,
            priority: 0,
          },
        },
      ],
      credentials: [{ id: credential, supplier_id: supplier, state: "active" }],
      quotas: [{ name: group, ...limits }],
    });
    render(<AiServiceAdmin entry="providers" />);
    fireEvent.click(await screen.findByRole("button", { name: "新增接入" }));
    fireEvent.click(screen.getByRole("button", { name: "使用已有模型" }));
    const save = screen.getByRole<HTMLButtonElement>("button", {
      name: "准备验证",
    });
    await chooseOption("供应商账号", /serving-account/);
    expect(screen.getByText("共享配额暂时无法验证")).toBeTruthy();
    expect(save.disabled).toBe(true);

    await chooseOption("供应商账号", "新建供应商账号");
    expect(screen.queryByText("共享配额暂时无法验证")).toBeNull();
    expect(save.disabled).toBe(false);
  },
);

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
    submitCreditGrant({
      scope_id: supplier,
      key: "order-1",
      reason: "test",
      credits: 1000000001,
    }),
  ).rejects.toThrow();
  await submitCreditGrant({
    scope_id: supplier,
    key: "order-1",
    reason: "test",
    credits: 100,
  });
  expect(send.mock.lastCall?.[0]).toBe("/api/ai/admin/credits");
});

it("saves the keyless procurement plan before showing API Key and budget confirmation", async () => {
  render(<AiServiceAdmin entry="providers" />);
  await screen.findByRole("table", { name: "供应商" });
  expect(send.mock.calls[0][0]).toBe("/api/ai/admin/configuration");
  expect(screen.queryByText("must-not-render")).toBeNull();
  fireEvent.click(screen.getByRole("button", { name: "新增接入" }));
  fireEvent.click(screen.getByRole("button", { name: "使用已有模型" }));
  expect(screen.queryByLabelText("API Key")).toBeNull();
  for (const [label, value] of [
    ["验证付款空间 ID", supplier],
    ["共享配额组", "account-a"],
    ["供应商地址", "https://api.example.com/v1"],
    ["供应商模型号", "supplier-sku"],
    ["采购输入价", "0.20"],
  ])
    fireEvent.change(screen.getByLabelText(label), { target: { value } });
  const dialog = screen.getByRole("dialog", { name: "新增接入" });
  fireEvent.click(screen.getByRole("button", { name: "准备验证" }));
  const secret =
    await within(dialog).findByLabelText<HTMLInputElement>("API Key");
  expect(screen.getByRole("dialog", { name: "新增接入" })).toBe(dialog);
  expect(
    screen.getByRole("checkbox", { name: "确认生成试跑，最多使用 2 积分" }),
  ).toBeTruthy();
  expect(
    send.mock.calls.find(([path]) => path.endsWith("/routes"))?.[1].data,
  ).not.toHaveProperty("key");
  const verify = screen.getByRole<HTMLButtonElement>("button", {
    name: "确认并试跑",
  });
  expect(verify.disabled).toBe(true);
  fireEvent.change(secret, { target: { value: "secret-once" } });
  expect(verify.disabled).toBe(true);
  fireEvent.click(
    screen.getByRole("checkbox", { name: "确认生成试跑，最多使用 2 积分" }),
  );
  expect(verify.disabled).toBe(false);
  send.mockImplementation(async (path: string) => {
    if (path.endsWith("/verify")) throw new Error("cannot-save");
    return review;
  });
  fireEvent.click(verify);
  await waitFor(() => expect(secret.value).toBe(""));
  expect(send).toHaveBeenCalledWith(
    `/api/ai/admin/onboarding/${verificationID}/verify`,
    expect.objectContaining({
      data: {
        key: "secret-once",
        credits: 2,
        purchase_micros: 500,
        confirm_trial: true,
      },
    }),
  );
  expect(localStorage.length).toBe(0);
  expect(sessionStorage.length).toBe(0);
  expect(screen.queryByText("secret-once")).toBeNull();
  await findFeedback("cannot-save");
  fireEvent.click(within(dialog).getByRole("button", { name: "关闭对话框" }));
  expect(screen.queryByRole("alertdialog")).toBeNull();
  expect(screen.queryByRole("dialog", { name: "新增接入" })).toBeNull();
});

it("normalizes empty configuration and discards any credential read-back fields", async () => {
  send.mockResolvedValueOnce({
    models: null,
    routes: null,
    credentials: null,
    quotas: null,
    unknown: null,
  });
  expect(await loadAiServiceConfiguration()).toEqual({
    model_presets: [],
    key_endpoints: [],
    credit_grants: [],
    models: [],
    routes: [],
    onboarding: [],
    credentials: [],
    quotas: [],
    unknown: [],
    media_drivers: [],
  });
  const configuration = await loadAiServiceConfiguration();
  expect(configuration.credentials[0]).toEqual({
    id: credential,
    supplier_id: supplier,
    state: "active",
  });
});

it("previews a promotional grant and submits it for independent review", async () => {
  render(<AiServiceAdmin entry="billing" />);
  await screen.findByRole("table", { name: "积分赠送单" });
  fireEvent.click(screen.getByRole("button", { name: "赠送积分" }));
  fireEvent.change(screen.getByLabelText("积分空间 ID"), {
    target: { value: supplier },
  });
  fireEvent.change(screen.getByLabelText("唯一业务凭据"), {
    target: { value: "admin-invoice-001" },
  });
  fireEvent.change(screen.getByLabelText("赠送积分"), {
    target: { value: "100" },
  });
  fireEvent.change(screen.getByLabelText("赠送原因"), {
    target: { value: "测试积分" },
  });
  fireEvent.click(screen.getByRole("button", { name: "核对赠送信息" }));
  expect(send).toHaveBeenCalledTimes(1);
  expect(screen.getByRole("status").textContent).toContain(supplier);
  expect(screen.getByRole("status").textContent).toContain("100 通用积分");
  fireEvent.click(screen.getByRole("button", { name: "提交复核" }));
  await waitFor(() =>
    expect(send).toHaveBeenCalledWith(
      "/api/ai/admin/credits",
      expect.objectContaining({
        data: {
          scope_id: supplier,
          key: "admin-invoice-001",
          credits: 100,
          reason: "测试积分",
        },
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
    key_endpoints: [{ provider: "openai", base_url: mediaDriver.base_url }],
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
  render(<AiServiceAdmin entry="providers" />);
  await screen.findByRole("table", { name: "供应商" });
  fireEvent.click(screen.getByRole("button", { name: "新增接入" }));
  fireEvent.click(screen.getByRole("button", { name: "使用已有模型" }));
  await chooseOption("媒体驱动", /OpenAI/i);
  expect(screen.getByRole("button", { name: "准备接入" })).toBeTruthy();
  expect(screen.queryByLabelText("验证项目 ID")).toBeNull();
  await chooseOption("验证方式", "生成试跑");
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
  fireEvent.click(screen.getByRole("button", { name: "准备验证" }));
  await waitFor(() =>
    expect(send).toHaveBeenCalledWith(
      "/api/ai/admin/routes",
      expect.objectContaining({
        method: "POST",
        data: expect.objectContaining({
          project_id: credential,
          verification: "generation_trial",
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
  render(<AiServiceAdmin entry="providers" />);
  fireEvent.click(await screen.findByRole("button", { name: "新增接入" }));
  fireEvent.change(screen.getByLabelText("销售模型名称"), {
    target: { value: "image/preset" },
  });
  await chooseOption("模型协议", "媒体生成");
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
  fireEvent.click(screen.getByRole("button", { name: "保存模型" }));
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

it("stages video Token prices and metering data without using a fixed request price", async () => {
  const videoMetering = {
    resolution: "540p",
    frames_per_second: 30,
    pixels_per_token: 1024,
    minimum_tokens: 10,
    geometries: { "3:2": { width: 810, height: 540 } },
  };
  await prepareModel({
    ...model,
    protocol: "media",
    capability: "video.generate",
    modes: ["text2video"],
    parameters: {
      resolution: { type: "string", default: "540p" },
      ratio: { type: "string", default: "3:2" },
      duration: { type: "integer" },
    },
    output_policy: "provider_success_v1",
    max_output: 86000,
    price: { ...price, input: 0, output: 78000000 },
    video_metering: videoMetering,
  });
  expect(send).toHaveBeenCalledWith(
    "/api/ai/admin/models",
    expect.objectContaining({
      data: expect.objectContaining({
        video_metering: videoMetering,
        max_output: 86000,
        price: expect.objectContaining({ output: 78000000, request: 0 }),
      }),
    }),
  );
});

it("prepares an input-priced Ark embedding release and enforces its fixed supplier route", async () => {
  const inputPrice = { ...price, output: 0 };
  const embedding = {
    ...model,
    protocol: arkEmbedding.protocol,
    price: inputPrice,
    max_output: 1,
  };
  await prepareModel(embedding);
  expect(send.mock.calls[0][1].data).toMatchObject({
    protocol: "ark_embedding",
    capability: "text.embed",
    tools: false,
    stream: false,
    price: inputPrice,
  });
  await expect(prepareModel({ ...embedding, stream: true })).rejects.toThrow();
  const route = {
    model_id: modelID,
    supplier,
    group: "ark",
    base_url: arkEmbedding.baseURL,
    upstream_model: arkEmbedding.model,
    protocol: arkEmbedding.protocol,
    purchase: inputPrice,
    priority: 0,
  };
  const options = {
    scope: supplier,
    project: credential,
    concurrency: 2,
    rpm: 60,
  };
  await expect(
    prepareRoute({ ...route, upstream_model: "different" }, options),
  ).rejects.toThrow();
  await prepareRoute(route, options);
  expect(send.mock.calls[1][1].data.route).toMatchObject({
    protocol: arkEmbedding.protocol,
    upstream_model: arkEmbedding.model,
  });
});

it("lists only models with an active channel and filters by capability", async () => {
  const draftID = "88888888-8888-4888-8888-888888888888";
  const videoID = "55555555-5555-4555-8555-555555555555";
  const routeID = "66666666-6666-4666-8666-666666666666";
  const disabledCredential = "77777777-7777-4777-8777-777777777777";
  send.mockResolvedValueOnce({
    models: [
      {
        model: {
          ...model,
          name: "Image model",
          protocol: "media",
          capability: "image.generate",
        },
        enabled: true,
      },
      {
        model: {
          ...model,
          id: videoID,
          name: "Video model",
          protocol: "media",
          capability: "video.generate",
        },
        enabled: true,
      },
      {
        model: {
          ...model,
          id: draftID,
          name: "Pending model",
          protocol: "media",
          capability: "image.generate",
        },
        enabled: false,
        draft: true,
      },
    ],
    routes: [
      {
        route: { ...mediaRoute, id: routeID },
        enabled: true,
        provider_key: "volcengine_ark",
      },
      {
        route: {
          ...mediaRoute,
          id: verificationID,
          model_id: videoID,
          credential: disabledCredential,
          upstream_model: "video-upstream",
        },
        enabled: true,
      },
    ],
    credentials: [
      { id: credential, supplier_id: supplier, state: "active" },
      { id: disabledCredential, supplier_id: supplier, state: "draining" },
    ],
    onboarding: [{ ...review, model_id: draftID, provider_key: "byteplus" }],
    quotas: [],
    unknown: [],
  });
  render(<AiServiceAdmin />);
  const table = await screen.findByRole("table", { name: "可用模型" });
  const imageRow = within(table).getByRole("row", { name: /Image model/ });
  expect(
    within(imageRow)
      .getByRole("link", { name: "配置售价" })
      .getAttribute("href"),
  ).toBe(`/models/${modelID}`);
  expect(
    within(imageRow)
      .getByRole("cell", { name: "Image model" })
      .querySelector("img")
      ?.getAttribute("src"),
  ).toBe("/brand/providers/volcengine.svg");
  expect(within(table).queryByText("Video model")).toBeNull();
  expect(within(table).queryByText("Pending model")).toBeNull();
  expect(screen.queryByRole("button", { name: "新增接入" })).toBeNull();
  fireEvent.change(screen.getByRole("combobox", { name: "模型能力" }), {
    target: { value: "video" },
  });
  expect(await screen.findByText("暂无可用模型")).toBeTruthy();
  fireEvent.change(screen.getByRole("combobox", { name: "模型能力" }), {
    target: { value: "image" },
  });
  expect(await screen.findByText("Image model")).toBeTruthy();
});

it("defaults text model sale rates to the supplier tariff and saves an override", async () => {
  const preset = {
    key: "version/seed-pro",
    provider: "火山方舟",
    adapter: "volcengine_ark",
    name: "Doubao Seed 2.1 Pro",
    upstream: "doubao-seed-2-1-pro-260915",
    base_url: "https://ark.cn-beijing.volces.com/api/v3",
    capability: "text.generate",
    modes: ["text_to_text"],
    fields: [],
    rates: ["缓存存储 ¥0.017 / 百万 Token / 小时"],
    currency: "CNY",
    request_micros: null,
    token_price: {
      input: 6_000_000,
      cache_read: 1_200_000,
      output: 30_000_000,
    },
    concurrency: 2,
    rpm: 30,
  };
  send.mockImplementation(async (path: string) =>
    path.endsWith("/configuration") ? { model_presets: [preset] } : undefined,
  );
  render(<AiServiceAdmin entry="providers" />);
  fireEvent.click(await screen.findByRole("button", { name: "新增接入" }));
  const dialog = screen.getByRole("dialog", { name: "新增接入" });
  fireEvent.click(
    within(dialog).getByRole("option", { name: /^Doubao Seed 2\.1 Pro/ }),
  );
  expect(
    within(dialog).getByLabelText<HTMLInputElement>("销售输入价").value,
  ).toBe("6");
  const output = within(dialog).getByLabelText<HTMLInputElement>("销售输出价");
  expect(output.value).toBe("30");
  fireEvent.click(within(dialog).getByRole("button", { name: "更多设置" }));
  expect(
    within(dialog).getByLabelText<HTMLInputElement>("销售缓存读取价").value,
  ).toBe("1.2");
  fireEvent.change(output, { target: { value: "31" } });
  fireEvent.click(within(dialog).getByRole("button", { name: "保存模型" }));
  await waitFor(() =>
    expect(send).toHaveBeenCalledWith(
      "/api/ai/admin/models",
      expect.objectContaining({
        data: expect.objectContaining({
          price: expect.objectContaining({
            input: 6_000_000,
            output: 31_000_000,
            cache_read: 1_200_000,
            currency: "CNY",
          }),
        }),
      }),
    ),
  );
});

it("uses the supplier video price for the selected output resolution", async () => {
  const preset = {
    key: "version/seedance",
    provider: "火山方舟",
    adapter: "volcengine_ark",
    name: "Seedance 2.5",
    upstream: "doubao-seedance-2-5-260628",
    base_url: "https://ark.cn-beijing.volces.com/api/v3",
    capability: "video.generate",
    modes: ["text2video"],
    fields: [],
    rates: [],
    currency: "CNY",
    request_micros: null,
    video_estimate: {
      resolutions: {
        "720p": { cny_per_million_without_video: 70, geometries: {} },
        "1080p": { cny_per_million_without_video: 77, geometries: {} },
      },
    },
    concurrency: 2,
    rpm: 30,
  };
  send.mockResolvedValueOnce({
    model_presets: [preset],
    media_drivers: [
      {
        key: preset.adapter,
        name: preset.provider,
        base_url: preset.base_url,
        region: "domestic",
        parameters: { "video.generate": [] },
        custom_url: false,
      },
    ],
  });
  render(<AiServiceAdmin entry="providers" />);
  fireEvent.click(await screen.findByRole("button", { name: "新增接入" }));
  const dialog = screen.getByRole("dialog", { name: "新增接入" });
  fireEvent.click(
    within(dialog).getByRole("option", { name: /^Seedance 2\.5/ }),
  );
  expect(within(dialog).getByText(/选择供应商支持的分辨率/)).toBeTruthy();
  await chooseOption("输出分辨率", "720p");
  expect(
    within(dialog).getByLabelText<HTMLInputElement>("销售视频 Token 单价")
      .value,
  ).toBe("70");
  await chooseOption("输出分辨率", "1080p");
  expect(
    within(dialog).getByLabelText<HTMLInputElement>("销售视频 Token 单价")
      .value,
  ).toBe("77");
});

it("carries a selected supplier preset through draft and procurement without a key", async () => {
  const preset = {
    key: "version/seed-tts",
    provider: "豆包语音",
    adapter: "volcengine_speech",
    name: "豆包语音合成 2.0",
    upstream: "seed-tts-2.0",
    base_url: "https://openspeech.bytedance.com/api/v3",
    capability: "audio.generate",
    modes: ["text2audio"],
    fields: [
      {
        field: "speaker",
        label: "音色",
        type: "string",
        default: "demo-voice",
      },
    ],
    concurrency: 2,
    rpm: 30,
    rates: ["¥0.123456 / 次"],
    currency: "CNY",
    request_micros: 123456,
    price_source_url: "https://pricing.example.test/speech",
  };
  const models: { model: typeof model; enabled: boolean; draft: boolean }[] =
    [];
  send.mockImplementation(async (path, options) => {
    if (path.endsWith("/configuration"))
      return {
        models,
        model_presets: [preset],
        media_drivers: [
          {
            key: preset.adapter,
            name: preset.provider,
            base_url: preset.base_url,
            region: "domestic",
            parameters: { "audio.generate": ["speaker"] },
            custom_url: false,
          },
        ],
      };
    if (path.endsWith("/models"))
      models.push({ model: options.data, enabled: false, draft: true });
  });
  render(<AiServiceAdmin entry="providers" />);
  fireEvent.click(await screen.findByRole("button", { name: "新增接入" }));
  const dialog = screen.getByRole("dialog", { name: "新增接入" });
  fireEvent.click(
    within(dialog).getByRole("option", { name: new RegExp(preset.name) }),
  );
  expect(screen.getByRole("dialog", { name: "新增接入" })).toBe(dialog);
  const reference = screen.getByRole("button", { name: "供应商参考价" });
  expect(reference.getAttribute("aria-expanded")).toBe("false");
  fireEvent.click(reference);
  expect(screen.getByText("¥0.123456 / 次")).toBeTruthy();
  expect(
    screen.getByRole("link", { name: "供应商价目" }).getAttribute("href"),
  ).toBe(preset.price_source_url);
  expect(screen.getByLabelText<HTMLInputElement>("销售按次价").value).toBe(
    "0.123456",
  );
  const settings = screen.getByRole("button", { name: "更多设置" });
  expect(settings.getAttribute("aria-expanded")).toBe("false");
  fireEvent.click(settings);
  expect(settings.getAttribute("aria-expanded")).toBe("true");
  expect(screen.getByLabelText<HTMLInputElement>("销售模型名称").value).toBe(
    preset.name,
  );
  fireEvent.click(settings);
  fireEvent.change(screen.getByLabelText("销售按次价"), {
    target: { value: "1" },
  });
  fireEvent.click(screen.getByRole("button", { name: "保存模型" }));
  expect(await within(dialog).findByDisplayValue(preset.upstream)).toBeTruthy();
  expect(screen.getByDisplayValue(preset.base_url)).toBeTruthy();
  expect(screen.getByLabelText<HTMLInputElement>("采购按次价").value).toBe(
    "0.123456",
  );
  expect(models[0].model).toMatchObject({
    preset_key: preset.key,
    modes: ["text2audio"],
    parameters: { speaker: { type: "string", default: "demo-voice" } },
  });
  expect(
    send.mock.calls.some(
      ([path]) => path.includes("/verify") || path.endsWith("/routes"),
    ),
  ).toBe(false);
});

it("shows insufficient verification credits in one centered toast and keeps the unsaved key for retry", async () => {
  const draft = { ...review, plan: { ...review.plan, credits: 120 } };
  let confirmed = false;
  send.mockImplementation(async (path: string) => {
    if (path.endsWith("configuration")) {
      return {
        models: [{ model, enabled: false, draft: true }],
        onboarding: [draft],
        routes: [],
        credentials: [],
        quotas: [],
        unknown: [],
      };
    }
    if (path.endsWith("/verify") && !confirmed) {
      throw new RequestError("insufficient_credits", 402);
    }
    return confirmed ? { ...draft, state: "verifying" } : draft;
  });
  render(<AiServiceAdmin entry="providers" />);
  fireEvent.click(await screen.findByRole("button", { name: "新增接入" }));
  const dialog = screen.getByRole("dialog", { name: "新增接入" });
  fireEvent.click(
    within(dialog).getByRole("option", { name: /^text\/default/ }),
  );
  const key = await within(dialog).findByLabelText<HTMLInputElement>("API Key");
  fireEvent.change(key, { target: { value: "fixture-key" } });
  fireEvent.click(
    within(dialog).getByRole("checkbox", {
      name: "确认生成试跑，最多使用 120 积分",
    }),
  );
  fireEvent.click(within(dialog).getByRole("button", { name: "确认并试跑" }));
  const feedback = await findFeedback("验证空间积分不足，需至少 120 积分");
  expect(
    feedback.closest("[data-sonner-toaster]")?.getAttribute("data-x-position"),
  ).toBe("center");
  expect(within(dialog).queryByText("操作未完成")).toBeNull();
  expect(key.value).toBe("fixture-key");
  expect(localStorage.length).toBe(0);
  expect(sessionStorage.length).toBe(0);
  confirmed = true;
  fireEvent.click(within(dialog).getByRole("button", { name: "确认并试跑" }));
  expect(await within(dialog).findByText("验证中")).toBeTruthy();
  expect(within(dialog).queryByLabelText("API Key")).toBeNull();
  expect(key.value).toBe("");
});

it("enables a Seedance channel after a zero-credit credential check", async () => {
  const draft = {
    ...review,
    plan: {
      method: "read_only_key",
      credits: 0,
      purchase_micros: 0,
      probes: [],
    },
  };
  let live = false;
  send.mockImplementation(async (path: string) => {
    if (path.endsWith("configuration")) {
      return {
        models: [{ model, enabled: live, draft: !live }],
        onboarding: [draft],
        routes: [],
        credentials: [],
        quotas: [],
        unknown: [],
      };
    }
    if (path.endsWith("/verify")) live = true;
    return live ? { ...draft, state: "active", available: true } : draft;
  });
  render(<AiServiceAdmin entry="providers" />);
  fireEvent.click(await screen.findByRole("button", { name: "新增接入" }));
  const dialog = screen.getByRole("dialog", { name: "新增接入" });
  fireEvent.click(
    within(dialog).getByRole("option", { name: /^text\/default/ }),
  );
  const key = await within(dialog).findByLabelText("API Key");
  expect(within(dialog).getByText("只读鉴权 · 0 积分")).toBeTruthy();
  fireEvent.change(key, { target: { value: "simulated-key" } });
  fireEvent.click(within(dialog).getByRole("button", { name: "保存接入" }));
  expect(await within(dialog).findByRole("status")).toBeTruthy();
  await findFeedback("接入已保存");
  expect(send).toHaveBeenCalledWith(
    `/api/ai/admin/onboarding/${verificationID}/verify`,
    expect.objectContaining({
      data: { key: "simulated-key", credits: 0, purchase_micros: 0 },
    }),
  );
  expect(within(dialog).getByRole("status").textContent).toBe("可用");
});

it("previews whole credits and publishes only the edited sale price", async () => {
  const route = { ...mediaRoute, id: verificationID };
  const saleURL = `/api/ai/admin/models/${modelID}/price`;
  send.mockImplementation(async (path: string) => {
    if (path.endsWith("/configuration"))
      return {
        models: [{ model: { ...model, sale_version: 3 }, enabled: true }],
        routes: [{ route, provider_key: "volcengine_ark", enabled: true }],
        credentials: [
          { id: credential, supplier_id: supplier, state: "active" },
        ],
      };
    if (path === `${saleURL}/preview`) return { credits: 2 };
    if (path === saleURL) return { ...model, sale_version: 4 };
    throw new Error("Unexpected mutation");
  });
  render(<AiServiceAdmin entityID={modelID} />);
  const multiplier = await screen.findByLabelText("倍率");
  expect(screen.queryByText("采购价")).toBeNull();
  expect(screen.queryByRole("button", { name: "新增接入" })).toBeNull();
  fireEvent.change(multiplier, { target: { value: "1.25" } });
  fireEvent.click(screen.getByRole("button", { name: "预览积分" }));
  expect(await screen.findByText("预计 2 积分")).toBeTruthy();
  const editedPrice = { ...price, multiplier: "1.25" };
  expect(send).toHaveBeenCalledWith(
    `${saleURL}/preview`,
    expect.objectContaining({
      data: {
        version: 3,
        price: editedPrice,
        usage: { input: 1000, output: 1000 },
      },
    }),
  );
  expect(send.mock.calls.map(([path]) => path)).toEqual([
    "/api/ai/admin/configuration",
    `${saleURL}/preview`,
  ]);
  fireEvent.click(screen.getByRole("button", { name: "发布售价" }));
  const dialog = screen.getByRole("alertdialog");
  expect(
    within(dialog).getByText(
      "新报价立即使用新售价，已确认的任务仍按原价结算。",
    ),
  ).toBeTruthy();
  const confirm = within(dialog).getByRole<HTMLButtonElement>("button", {
    name: "确认发布",
  });
  await waitFor(() => expect(confirm.disabled).toBe(false));
  fireEvent.click(confirm);
  await findFeedback("售价已发布。");
  expect(send.mock.calls.at(2)).toEqual([
    saleURL,
    expect.objectContaining({
      method: "PUT",
      data: { version: 3, price: editedPrice },
    }),
  ]);
});
