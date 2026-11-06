import { cleanup, fireEvent, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { render } from "../../../../feedback";
import { ModelDetail } from "@/app/models/_components/model-detail";
import { RequestError } from "@/lib/request";

const api = vi.hoisted(() => ({ model: vi.fn(), status: vi.fn() }));
vi.mock("@/components/auth/session", () => ({
  useSession: () => ({ actor: { id: "admin" } }),
}));
vi.mock("@/api/settings", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/api/settings")>()),
  getAdminModel: api.model,
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
  expect(screen.queryByRole("button", { name: "保存价格" })).toBeNull();
  fireEvent.mouseDown(screen.getByRole("tab", { name: "价格" }), { button: 0 });
  expect(await screen.findByRole("button", { name: "保存价格" })).toBeTruthy();
  expect(screen.queryByRole("button", { name: "保存配置" })).toBeNull();
  fireEvent.mouseDown(screen.getByRole("tab", { name: "模型设置" }), {
    button: 0,
  });
  expect((screen.getByLabelText("上游模型 ID") as HTMLInputElement).value).toBe(
    "draft-model-id",
  );
});

it("预设模型从普通详情核验采购价，缺少采购价时不显示销售价表单", async () => {
  api.model.mockResolvedValue({
    model: { id: "model-a", display_name: "示例预设" },
    candidate: { model: { provider_model_id: "preset-model-id" } },
    versions: [{ id: "version-a", provider_model_id: "preset-model-id" }],
    prices: [],
  });
  render(
    <ModelDetail
      id="model-a"
      capabilities={[]}
      initialView="pricing"
      onSaved={vi.fn()}
    />,
  );
  const save = await screen.findByRole("button", { name: "保存采购价" });
  expect((save as HTMLButtonElement).disabled).toBe(true);
  expect(screen.getByText("选择计费方式")).toBeTruthy();

  fireEvent.mouseDown(screen.getByRole("tab", { name: "销售价" }), {
    button: 0,
  });
  expect(await screen.findByText("请先核验采购价")).toBeTruthy();
  expect(screen.queryByRole("button", { name: "保存销售价" })).toBeNull();
});

it("已有采购价的预设沿用计量单位，销售价仍需独立保存", async () => {
  api.model.mockResolvedValue({
    model: { id: "model-a", display_name: "示例预设" },
    candidate: { model: { provider_model_id: "preset-model-id" } },
    versions: [{ id: "version-a", provider_model_id: "preset-model-id" }],
    prices: [
      {
        id: "procurement-a",
        version_no: 1,
        source_url: "https://example.com/price",
        reviewed_at: "2026-11-06T00:00:00Z",
        unit: "per_image",
        currency: "CNY",
        rule: { base_micros: 120000 },
      },
    ],
  });
  render(
    <ModelDetail
      id="model-a"
      capabilities={[]}
      initialView="pricing"
      onSaved={vi.fn()}
    />,
  );
  await screen.findByRole("button", { name: "保存销售价" });
  expect(
    screen.getByRole("combobox", { name: "计费方式" }).textContent,
  ).toContain("每张图片");
  expect((screen.getByLabelText("价格（CNY）") as HTMLInputElement).value).toBe(
    "",
  );

  fireEvent.mouseDown(screen.getByRole("tab", { name: "采购价" }), {
    button: 0,
  });
  await screen.findByRole("button", { name: "保存采购价" });
  expect((screen.getByLabelText("价格（CNY）") as HTMLInputElement).value).toBe(
    "0.12",
  );
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
