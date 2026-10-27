import { pageBusinessMemory } from "@/lib/page-business-memory";
import { cleanup, fireEvent, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { render } from "../../../../feedback";
import { PresetConnect } from "@/app/providers/_components/preset-connect";
import { RequestError } from "@/lib/request";
const api = vi.hoisted(() => ({
  connect: vi.fn(),
  key: vi.fn(),
  receipt: vi.fn(),
  onboarding: vi.fn(),
}));
vi.mock("@/components/auth/session", () => ({
  useSession: () => ({
    actor: { id: "admin-fixture", platform_role: "platform_admin" },
  }),
}));
vi.mock("@/api/settings", () => ({
  connectProviderPreset: api.connect,
  setAdminCredential: api.key,
  getConfigReceipt: api.receipt,
  getProviderOnboarding: api.onboarding.mockResolvedValue({
    provider_id: "connection",
    status: "disabled",
    credential_id: "credential",
    models: [
      {
        id: "model",
        display_name: "DeepSeek Flash",
        capability: "text.generate",
      },
    ],
  }),
}));
const presets: API.Preset[] = [
  {
    id: "preset",
    version_id: "version",
    document: {
      name: "OpenRouter",
      model_templates: [{ name: "Reviewed text" }],
    },
  },
];
const created = vi.fn();
const saved = vi.fn();
beforeEach(() => {
  vi.clearAllMocks();
  pageBusinessMemory.clear();
  api.connect.mockResolvedValue({ provider_id: "connection" });
  api.key.mockResolvedValue({
    id: "credential",
    provider_id: "connection",
    last4: "test",
    status: "active",
  });
});
afterEach(cleanup);
function setup() {
  render(
    <PresetConnect presets={presets} onCreated={created} onSaved={saved} />,
  );
  fireEvent.click(screen.getByRole("radio", { name: /OpenRouter/ }));
  fireEvent.change(screen.getByLabelText("API Key"), {
    target: { value: "fixture-api-key-test" },
  });
  fireEvent.click(screen.getByRole("button", { name: "保存连接" }));
}
it("选预设和 Key 即可保存，不要求名称、技术标识或 JSON", async () => {
  setup();
  await waitFor(() => expect(saved).toHaveBeenCalledOnce());
  expect(api.connect).toHaveBeenCalledWith(
    { id: "preset" },
    expect.objectContaining({ version_id: "version", name: "" }),
  );
  expect(api.connect.mock.calls[0][1]).not.toHaveProperty("key");
  expect(api.key).toHaveBeenCalledWith(
    { id: "connection" },
    { label: "平台统一 Key", secret: { api_key: "fixture-api-key-test" } },
    expect.anything(),
  );
  expect(screen.getByLabelText("API Key").getAttribute("type")).toBe(
    "password",
  );
  expect((screen.getByLabelText("API Key") as HTMLInputElement).value).toBe("");
  expect(JSON.stringify(localStorage)).not.toContain("fixture-api-key-test");
});
it("Key 保存失败后继续写原连接，不重复创建", async () => {
  api.key.mockRejectedValueOnce(new RequestError("invalid_request", 422));
  setup();
  await waitFor(() => expect(api.key).toHaveBeenCalledOnce());
  fireEvent.change(screen.getByLabelText("API Key"), {
    target: { value: "fixture-api-key-test" },
  });
  await waitFor(() =>
    expect(
      (
        screen.getByRole("button", {
          name: "继续保存 API Key",
        }) as HTMLButtonElement
      ).disabled,
    ).toBe(false),
  );
  fireEvent.click(screen.getByRole("button", { name: "继续保存 API Key" }));
  await waitFor(() => expect(saved).toHaveBeenCalledOnce());
  expect(api.connect).toHaveBeenCalledOnce();
  expect(api.key).toHaveBeenCalledTimes(2);
});
it("创建回执丢失后查询原结果，不自动再次创建或补发 Key", async () => {
  api.connect.mockRejectedValueOnce(new RequestError("request_unconfirmed"));
  api.receipt.mockResolvedValue({ result: { provider_id: "connection" } });
  setup();
  await screen.findByRole("button", { name: "查询回执" });
  fireEvent.click(screen.getByRole("button", { name: "查询回执" }));
  await waitFor(() => expect(created).toHaveBeenCalledWith("connection"));
  expect(
    screen
      .getByRole("radio", { name: /OpenRouter/ })
      .getAttribute("aria-checked"),
  ).toBe("true");
  expect(api.connect).toHaveBeenCalledOnce();
  expect(api.key).not.toHaveBeenCalled();
  expect(JSON.stringify(localStorage)).not.toContain("fixture-api-key-test");
  fireEvent.change(screen.getByLabelText("API Key"), {
    target: { value: "fixture-api-key-test" },
  });
  fireEvent.click(screen.getByRole("button", { name: "继续保存 API Key" }));
  await waitFor(() => expect(saved).toHaveBeenCalledOnce());
  expect(api.connect).toHaveBeenCalledOnce();
});
it("Key 写入未知时恢复只查回执，不再次写 Key", async () => {
  api.key.mockRejectedValueOnce(new RequestError("request_unconfirmed"));
  api.receipt.mockResolvedValue({
    result: {
      id: "credential",
      provider_id: "connection",
      status: "active",
      last4: "test",
    },
  });
  setup();
  await waitFor(() => expect(api.key).toHaveBeenCalledOnce());
  await waitFor(() =>
    expect(
      (screen.getByRole("button", { name: "查询回执" }) as HTMLButtonElement)
        .disabled,
    ).toBe(false),
  );
  fireEvent.click(screen.getByRole("button", { name: "查询回执" }));
  await waitFor(() => expect(saved).toHaveBeenCalledOnce());
  expect(api.connect).toHaveBeenCalledOnce();
  expect(api.key).toHaveBeenCalledOnce();
});

it("错误连接的 Key 回执不能被认作保存成功", async () => {
  api.key.mockRejectedValueOnce(new RequestError("request_unconfirmed"));
  api.receipt.mockResolvedValue({
    result: {
      id: "credential",
      provider_id: "other",
      status: "active",
      last4: "test",
    },
  });
  setup();
  await waitFor(() => expect(api.key).toHaveBeenCalledOnce());
  await waitFor(() =>
    expect(
      (screen.getByRole("button", { name: "查询回执" }) as HTMLButtonElement)
        .disabled,
    ).toBe(false),
  );
  fireEvent.click(screen.getByRole("button", { name: "查询回执" }));
  await waitFor(() => expect(api.receipt).toHaveBeenCalledOnce());
  expect(saved).not.toHaveBeenCalled();
  expect(pageBusinessMemory.length).toBeGreaterThan(0);
});

it("创建结果未知时先查回执，仅在未找到时重放原请求标识", async () => {
  api.connect.mockRejectedValueOnce(new RequestError("request_unconfirmed"));
  api.receipt.mockRejectedValueOnce(new RequestError("not_found", 404));
  setup();
  await waitFor(() => expect(api.connect).toHaveBeenCalledOnce());
  fireEvent.change(screen.getByLabelText("API Key"), {
    target: { value: "fixture-api-key-test" },
  });
  await waitFor(() =>
    expect(
      (
        screen.getByRole("button", {
          name: "保存连接",
        }) as HTMLButtonElement
      ).disabled,
    ).toBe(false),
  );
  fireEvent.click(screen.getByRole("button", { name: "保存连接" }));
  await waitFor(() => expect(saved).toHaveBeenCalledOnce());
  expect(api.receipt).toHaveBeenCalledOnce();
  expect(api.connect).toHaveBeenCalledTimes(2);
  expect(api.connect.mock.calls[0][1].request_id).toBe(
    api.connect.mock.calls[1][1].request_id,
  );
});
it("同页重开沿用待确认凭据的连接和请求，不要求再次新建", async () => {
  pageBusinessMemory.setItem(
    "glowloom.catalog.pending.v1.admin-fixture.preset.onboarding.credential",
    JSON.stringify({
      id: "original-key-request",
      action: "credential.set",
      target: "connection",
    }),
  );
  api.receipt.mockRejectedValueOnce(new RequestError("not_found", 404));
  render(
    <PresetConnect presets={presets} onCreated={created} onSaved={saved} />,
  );
  fireEvent.change(screen.getByLabelText("API Key"), {
    target: { value: "fixture-api-key-test" },
  });
  await waitFor(() =>
    expect(
      (
        screen.getByRole("button", {
          name: "继续保存 API Key",
        }) as HTMLButtonElement
      ).disabled,
    ).toBe(false),
  );
  fireEvent.click(screen.getByRole("button", { name: "继续保存 API Key" }));
  await waitFor(() => expect(saved).toHaveBeenCalledOnce());
  expect(api.connect).not.toHaveBeenCalled();
  expect(api.key.mock.calls[0][0]).toEqual({ id: "connection" });
  expect(api.key.mock.calls[0][2].headers["Idempotency-Key"]).toBe(
    "original-key-request",
  );
});

it("Key 输入先选预设，支持显隐，提交后清空并恢复隐藏", async () => {
  render(
    <PresetConnect presets={presets} onCreated={created} onSaved={saved} />,
  );
  expect(screen.getByLabelText("API Key")).toHaveProperty("disabled", true);
  fireEvent.click(screen.getByRole("radio", { name: /OpenRouter/ }));
  fireEvent.change(screen.getByLabelText("API Key"), {
    target: { value: "fixture-api-key-test" },
  });
  fireEvent.click(screen.getByRole("button", { name: "显示 API Key" }));
  expect(screen.getByLabelText("API Key")).toHaveProperty("type", "text");
  fireEvent.click(screen.getByRole("button", { name: "保存连接" }));
  await waitFor(() => expect(saved).toHaveBeenCalledOnce());
  expect(screen.getByLabelText("API Key")).toHaveProperty("value", "");
  expect(screen.getByLabelText("API Key")).toHaveProperty("type", "password");
});
it("从其他预设打开时仍恢复原创建请求，不替换其平台", () => {
  pageBusinessMemory.setItem(
    "glowloom.catalog.pending.v1.admin-fixture.preset.onboarding.create",
    JSON.stringify({
      id: "original",
      action: "preset.connect",
      target: "preset",
    }),
  );
  render(
    <PresetConnect
      presets={[...presets, { id: "other", document: { name: "其他预设" } }]}
      initialPreset="other"
      onCreated={created}
      onSaved={saved}
    />,
  );
  expect(
    screen
      .getByRole("radio", { name: /OpenRouter/ })
      .getAttribute("aria-checked"),
  ).toBe("true");
  expect(api.connect).not.toHaveBeenCalled();
});
it("按目录版本提交候选模型，切换平台会清空旧选择", async () => {
  const candidates: API.Preset[] = [
    {
      ...presets[0],
      model_catalog: {
        version: "catalog-v1",
        models: [
          {
            key: "chat",
            display_name: "通用聊天",
            provider_model_id: "vendor/chat",
            capability: "text.generate",
          },
        ],
      },
    },
    {
      id: "other",
      version_id: "v2",
      document: { name: "DeepSeek" },
      model_catalog: {
        version: "catalog-v2",
        models: [
          {
            key: "ds",
            display_name: "DeepSeek Flash",
            capability: "text.generate",
            provider_model_id: "deepseek-flash",
          },
        ],
      },
    },
  ];
  render(
    <PresetConnect presets={candidates} onCreated={created} onSaved={saved} />,
  );
  fireEvent.click(screen.getByRole("radio", { name: /OpenRouter/ }));
  expect(screen.getByRole("checkbox", { name: "通用聊天" })).toBeDefined();
  fireEvent.click(screen.getByRole("checkbox", { name: "通用聊天" }));
  fireEvent.click(screen.getByRole("radio", { name: /DeepSeek/ }));
  fireEvent.click(screen.getByRole("checkbox", { name: "DeepSeek Flash" }));
  fireEvent.change(screen.getByLabelText("API Key"), {
    target: { value: "fixture-api-key-test" },
  });
  expect(screen.getByRole("button", { name: "接入服务" })).toHaveProperty(
    "disabled",
    false,
  );
  await waitFor(() =>
    expect(
      screen
        .getByRole("button", { name: "接入服务" })
        .closest("form")!
        .checkValidity(),
    ).toBe(true),
  );
  fireEvent.click(screen.getByRole("button", { name: "接入服务" }));
  await screen.findByText("服务已接入。");
  expect(saved).not.toHaveBeenCalled();
  expect(api.onboarding).not.toHaveBeenCalled();
  expect(screen.queryByRole("button", { name: "验证模型" })).toBeNull();
  fireEvent.click(screen.getByRole("button", { name: "管理服务" }));
  expect(saved).toHaveBeenCalledWith("connection");
  expect(api.connect).toHaveBeenCalledWith(
    { id: "other" },
    expect.objectContaining({
      catalog_version: "catalog-v2",
      candidate_keys: ["ds"],
    }),
  );
});

it("跨分类多选保留选择，全选排除不可用模型，空分类不虚构模型", async () => {
  render(
    <PresetConnect
      presets={[
        {
          id: "preset",
          version_id: "v1",
          document: { name: "火山方舟" },
          model_catalog: {
            version: "v1",
            models: [
              {
                key: "text",
                display_name: "文本模型",
                capability: "text.generate",
              },
              {
                key: "image",
                display_name: "图片模型",
                capability: "image.generate",
              },
              {
                key: "video",
                display_name: "需配置视频",
                capability: "video.generate",
                requires_model_selection: true,
              },
            ],
          },
        },
      ]}
      onCreated={created}
      onSaved={saved}
    />,
  );
  fireEvent.click(screen.getByRole("radio", { name: "火山方舟" }));
  fireEvent.click(screen.getByRole("checkbox", { name: "文本模型" }));
  fireEvent.mouseDown(screen.getByRole("tab", { name: "图片" }), {
    button: 0,
    ctrlKey: false,
  });
  fireEvent.click(await screen.findByRole("checkbox", { name: "图片模型" }));
  expect(screen.getByText("已选 2")).toBeDefined();
  fireEvent.mouseDown(screen.getByRole("tab", { name: "音频" }), {
    button: 0,
    ctrlKey: false,
  });
  await screen.findByText("暂无可接入的音频模型");
  fireEvent.click(screen.getByRole("button", { name: "取消全选" }));
  expect(screen.getByText("已选 0")).toBeDefined();
  fireEvent.click(screen.getByRole("button", { name: "全选" }));
  fireEvent.change(screen.getByLabelText("API Key"), {
    target: { value: "fixture-key" },
  });
  fireEvent.click(screen.getByRole("button", { name: "接入服务" }));
  await screen.findByText("服务已接入。");
  expect(api.connect).toHaveBeenCalledWith(
    { id: "preset" },
    expect.objectContaining({ candidate_keys: ["text", "image"] }),
  );
  expect(api.onboarding).not.toHaveBeenCalled();
});
