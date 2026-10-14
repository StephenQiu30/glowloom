import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { GenerationDialog } from "./generation-dialog";
import { RequestError } from "@/lib/request";
const api = vi.hoisted(() => ({
  models: vi.fn(),
  quote: vi.fn(),
  confirm: vi.fn(),
  get: vi.fn(),
  list: vi.fn(),
  cancel: vi.fn(),
}));
vi.mock("@/api/models", () => ({
  listProjectModels: api.models,
}));
vi.mock("@/api/operations", () => ({
  createFreeQuote: api.quote,
  confirmOperationQuote: api.confirm,
  getOperation: api.get,
  listProjectOperations: api.list,
  cancelOperation: api.cancel,
}));
vi.mock("@/components/projects/project-picker", () => ({
  ProjectPicker: () => null,
}));
vi.mock("@/components/subjects/subject-list", () => ({
  SubjectList: () => null,
}));
vi.mock("@/components/media/project-media", () => ({
  ProjectMediaPreview: () => null,
  ProjectMediaPicker: () => null,
}));
const id = "11111111-1111-4111-8111-111111111111",
  project = {
    id: "22222222-2222-4222-8222-222222222222",
    canvas_id: "33333333-3333-4333-8333-333333333333",
    name: "验证项目",
    aspect_ratio: "16:9",
    style_key: null,
    revision: 1,
    update_time: "2026-10-13T00:00:00Z",
  };
beforeEach(() => {
  vi.clearAllMocks();
  api.models.mockResolvedValue({
    items: [
      {
        id,
        key: "actual",
        display_name: "真实模型",
        capability: "image.generate",
        status: "active",
        provider_status: "active",
        credential_present: true,
        current_version: {
          modes: ["text_to_image"],
          limits: { max_outputs: 4 },
          param_schema: [],
        },
      },
    ],
  });
});
afterEach(cleanup);
it("未确认的报价重试复用冻结请求，确认前不提交生成", async () => {
  const quote = {
    operation_id: id,
    quote_micros: 100,
    available_micros: 1000,
    confirmable: true,
    expires_at: "2099-01-01",
  };
  api.quote
    .mockRejectedValueOnce(new RequestError("request_unconfirmed"))
    .mockResolvedValue(quote);
  render(
    <GenerationDialog
      seed={{ project, prompt: "清晨的海边", capability: "image" }}
      onClose={() => {}}
    />,
  );
  await screen.findByRole("option", { name: "真实模型" });
  fireEvent.change(screen.getByLabelText("生成模型"), {
    target: { value: "actual" },
  });
  fireEvent.click(screen.getByRole("button", { name: "获取生成报价" }));
  await screen.findByRole("alert");
  fireEvent.click(screen.getByRole("button", { name: "重试原报价" }));
  await screen.findByRole("button", { name: "确认并开始生成" });
  expect(api.models.mock.calls[0][0].capability).toBe("image.generate");
  expect(api.quote.mock.calls[0]).toEqual(api.quote.mock.calls[1]);
  expect(api.confirm).not.toHaveBeenCalled();
  expect(screen.getByLabelText("生成模型").hasAttribute("disabled")).toBe(true);
  api.confirm.mockResolvedValue({
    reservation_id: id,
    budget_revision: 2,
    available_micros: 900,
  });
  api.get.mockResolvedValue({
    id,
    project_id: project.id,
    status: "completed",
    inputs: [],
    outputs: [],
  });
  fireEvent.click(screen.getByRole("button", { name: "确认并开始生成" }));
  await screen.findByRole("heading", { name: "已完成" });
  expect(api.confirm).toHaveBeenCalledOnce();
});
it("过期报价允许返回编辑，不会执行确认", async () => {
  api.quote.mockResolvedValue({
    operation_id: id,
    quote_micros: 100,
    available_micros: 1000,
    confirmable: true,
    expires_at: "2000-01-01",
  });
  render(
    <GenerationDialog
      seed={{ project, prompt: "测试", capability: "image" }}
      onClose={() => {}}
    />,
  );
  await screen.findByRole("option", { name: "真实模型" });
  fireEvent.change(screen.getByLabelText("生成模型"), {
    target: { value: "actual" },
  });
  fireEvent.click(screen.getByRole("button", { name: "获取生成报价" }));
  await screen.findByText("报价已过期，请重新获取。");
  expect(
    screen
      .getByRole("button", { name: "确认并开始生成" })
      .hasAttribute("disabled"),
  ).toBe(true);
  fireEvent.click(screen.getByRole("button", { name: "返回编辑" }));
  await waitFor(() =>
    expect(screen.getByLabelText("生成模型").hasAttribute("disabled")).toBe(
      false,
    ),
  );
  expect(api.confirm).not.toHaveBeenCalled();
});

it("画布参考素材沿真实模型角色进入报价，优先采用模型支持的图生图模式", async () => {
  api.models.mockResolvedValue({
    items: [
      {
        id,
        key: "actual",
        display_name: "真实模型",
        capability: "image.generate",
        status: "active",
        provider_status: "active",
        credential_present: true,
        input_roles: ["reference_image"],
        current_version: {
          modes: ["text_to_image", "image_to_image"],
          limits: {
            max_outputs: 4,
            roles: { reference_image: { types: ["image"] } },
          },
          param_schema: [],
        },
      },
    ],
  });
  api.quote.mockRejectedValue(new RequestError("invalid_request", 422));
  render(
    <GenerationDialog
      seed={{
        project,
        prompt: "海边",
        capability: "image",
        reference: { assetID: id, kind: "image" },
      }}
      onClose={() => {}}
    />,
  );
  await screen.findByRole("option", { name: "真实模型" });
  fireEvent.change(screen.getByLabelText("生成模型"), {
    target: { value: "actual" },
  });
  expect((screen.getByLabelText("生成模式") as HTMLSelectElement).value).toBe(
    "image_to_image",
  );
  fireEvent.click(screen.getByRole("button", { name: "获取生成报价" }));
  await waitFor(() => expect(api.quote).toHaveBeenCalledOnce());
  expect(api.quote.mock.calls[0][1].media_inputs).toEqual([
    { role: "reference_image", media_asset_id: id },
  ]);
  expect(api.confirm).not.toHaveBeenCalled();
});

it("模型不支持画布参考时必须明确取消参考，不能静默发出不含素材的报价", async () => {
  render(
    <GenerationDialog
      seed={{
        project,
        prompt: "海边",
        capability: "image",
        reference: { assetID: id, kind: "image" },
      }}
      onClose={() => {}}
    />,
  );
  await screen.findByRole("option", { name: "真实模型" });
  fireEvent.change(screen.getByLabelText("生成模型"), {
    target: { value: "actual" },
  });
  expect(
    screen
      .getByRole("button", { name: "获取生成报价" })
      .hasAttribute("disabled"),
  ).toBe(true);
  expect(api.quote).not.toHaveBeenCalled();
  fireEvent.click(screen.getByRole("button", { name: "不使用此参考素材" }));
  expect(
    screen
      .getByRole("button", { name: "获取生成报价" })
      .hasAttribute("disabled"),
  ).toBe(false);
});
