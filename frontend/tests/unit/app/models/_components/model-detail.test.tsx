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
    model: { id: "model-a", display_name: "示例模型", status: "disabled" },
    versions: [],
    prices: [],
  });
  render(<ModelDetail id="model-a" capabilities={[]} onSaved={vi.fn()} />);
  await screen.findByRole("heading", { name: "示例模型" });
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

it("归档及恢复沿用最新模型版本，恢复后才可修改配置", async () => {
  let model = {
    id: "model-a",
    display_name: "示例模型",
    status: "disabled",
    revision: 1,
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
  fireEvent.click(await screen.findByRole("button", { name: "归档模型" }));
  fireEvent.click(screen.getByRole("button", { name: "确认归档" }));
  const restore = await screen.findByRole("button", { name: "恢复模型" });
  expect(screen.queryByRole("tab", { name: "模型设置" })).toBeNull();
  fireEvent.click(restore);
  fireEvent.click(screen.getByRole("button", { name: "确认恢复" }));
  await screen.findByRole("tab", { name: "模型设置" });
  expect(api.status.mock.calls.map(([, body]) => body)).toEqual([
    { status: "archived", expected_revision: 1 },
    { status: "disabled", expected_revision: 2 },
  ]);
});
