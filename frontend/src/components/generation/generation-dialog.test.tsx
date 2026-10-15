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
  identity: vi.fn(),
  create: vi.fn(),
  save: vi.fn(),
  current: vi.fn(),
  resolve: vi.fn(),
  preview: vi.fn(),
}));
vi.mock("@/api/identity", () => ({ getSession: api.identity }));
vi.mock("@/api/models", () => ({ listProjectModels: api.models }));
vi.mock("@/api/operations", () => ({
  createFreeQuote: api.quote,
  confirmOperationQuote: api.confirm,
  getOperation: api.get,
  listProjectOperations: api.list,
  cancelOperation: api.cancel,
}));
vi.mock("@/api/promptEditor", () => ({
  validatePromptFragment: async () => {},
  createPromptDraft: api.create,
  savePromptDraft: api.save,
  readPromptDraft: vi.fn(),
  listPromptDrafts: vi.fn(),
  currentPromptSubmission: api.current,
  resolvePromptSubmission: api.resolve,
}));
vi.mock("@/api/media", () => ({
  getMediaPreview: api.preview,
  uploadMediaAsset: vi.fn(),
}));
vi.mock("./draft-save", async (importOriginal) => ({
  ...(await importOriginal<typeof import("./draft-save")>()),
  branchJournal: {
    list: async () => [],
    write: async () => {},
    remove: async () => true,
  },
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
const id = "11111111-1111-4111-8111-111111111111";
const version = "44444444-4444-4444-8444-444444444444";
const project = {
  id: "22222222-2222-4222-8222-222222222222",
  canvas_id: "33333333-3333-4333-8333-333333333333",
  name: "验证项目",
  aspect_ratio: "16:9",
  style_key: null,
  revision: 1,
  update_time: "2026-10-13T00:00:00Z",
};
let current: API.SubmissionCurrent;
let quote: API.QuoteResponse;
function saved(body: API.DraftRequest) {
  return {
    draft_id: id,
    revision: body.expected_revision! + 1,
    draft_hash: "draft-hash",
    input_hash: "input-hash",
    draft: {
      ...body.draft,
      mode: body.draft?.mode ?? null,
      modelVersion: body.draft?.modelVersion ?? null,
    },
  };
}
beforeEach(() => {
  vi.resetAllMocks();
  current = { epoch: 0 };
  quote = {
    operation_id: id,
    quote_micros: 100,
    available_micros: 1000,
    confirmable: true,
    expires_at: "2099-01-01",
    confirm_key: "confirm-key",
    submission_id: id,
  };
  api.identity.mockResolvedValue({ actor: { id } });
  api.create.mockImplementation(async (_, body) => saved(body));
  api.save.mockImplementation(async (_, body) => saved(body));
  api.current.mockImplementation(async () => current);
  api.resolve.mockImplementation(async () => (current = { epoch: 2 }));
  api.quote.mockImplementation(async (_, body) => {
    current = {
      epoch: 1,
      submission: {
        id,
        state: "quoted",
        operation_id: id,
        confirm_key: "confirm-key",
        input_source: body.input_source,
        quote,
      },
    };
    return quote;
  });
  api.confirm.mockImplementation(async () => {
    current = {
      ...current,
      submission: { ...current.submission, state: "accepted" },
    };
    return { reservation_id: id, budget_revision: 2, available_micros: 900 };
  });
  api.get.mockResolvedValue({
    id,
    project_id: project.id,
    status: "completed",
    inputs: [],
    outputs: [],
  });
  api.preview.mockResolvedValue({
    asset: { id, project_id: project.id, kind: "image", revision: 3 },
  });
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
          id: version,
          modes: ["text_to_image"],
          limits: { max_outputs: 4 },
          param_schema: [],
        },
      },
    ],
  });
});
afterEach(cleanup);
async function open(reference = false) {
  render(
    <GenerationDialog
      seed={{
        project,
        prompt: "清晨的海边",
        capability: "image",
        ...(reference
          ? { reference: { assetID: id, kind: "image" as const } }
          : {}),
      }}
      onClose={() => {}}
    />,
  );
  await screen.findByRole("option", { name: "真实模型" });
  await waitFor(() =>
    expect(screen.getByLabelText("生成模型").hasAttribute("disabled")).toBe(
      false,
    ),
  );
  fireEvent.change(screen.getByLabelText("生成模型"), {
    target: { value: version },
  });
  await waitFor(() =>
    expect((screen.getByLabelText("生成模式") as HTMLSelectElement).value).toBe(
      "text_to_image",
    ),
  );
}

it("未确认的报价重试复用保存后的草稿指针和幂等键，确认前不提交生成", async () => {
  api.quote.mockRejectedValueOnce(new RequestError("request_unconfirmed"));
  await open();
  fireEvent.click(screen.getByRole("button", { name: "检查费用" }));
  await screen.findByRole("alert");
  fireEvent.click(screen.getByRole("button", { name: "检查费用" }));
  await screen.findByRole("button", { name: "确认费用并生成" });
  expect(api.models.mock.calls[0][0].capability).toBe("image.generate");
  expect(api.quote.mock.calls[0]).toEqual(api.quote.mock.calls[1]);
  expect(api.quote.mock.calls[0][1]).toEqual({
    input_source: { kind: "prompt_draft", draft_id: id, revision: 2 },
    expected_submission_epoch: 0,
    force_regenerate: false,
  });
  expect(api.confirm).not.toHaveBeenCalled();
  fireEvent.click(screen.getByRole("button", { name: "确认费用并生成" }));
  await screen.findByRole("heading", { name: "已完成" });
  expect(api.confirm).toHaveBeenCalledOnce();
});

it("过期报价阻止确认，明确放弃后可以重新检查费用", async () => {
  quote.expires_at = "2000-01-01";
  await open();
  fireEvent.click(screen.getByRole("button", { name: "检查费用" }));
  await screen.findByText("报价已过期，请重新检查费用。");
  expect(
    screen
      .getByRole("button", { name: "确认费用并生成" })
      .hasAttribute("disabled"),
  ).toBe(true);
  fireEvent.click(screen.getByRole("button", { name: "放弃旧报价并重新检查" }));
  await screen.findByRole("button", { name: "检查费用" });
  expect(api.resolve.mock.calls[0][1]).toEqual({
    action: "discard_quote",
    expected_epoch: 1,
  });
  expect(api.confirm).not.toHaveBeenCalled();
});

it("画布参考保留版本与模型角色，选择图生图模式后先保存草稿再报价", async () => {
  const models = await api.models();
  models.items[0].input_roles = ["reference_image"];
  models.items[0].current_version.modes = ["text_to_image", "image_to_image"];
  models.items[0].current_version.limits.roles = {
    reference_image: { types: ["image"], for_modes: ["image_to_image"] },
  };
  await open(true);
  expect(
    screen.getByRole("button", { name: "检查费用" }).hasAttribute("disabled"),
  ).toBe(true);
  fireEvent.change(screen.getByLabelText("生成模式"), {
    target: { value: "image_to_image" },
  });
  await waitFor(() =>
    expect(
      screen.getByRole("button", { name: "检查费用" }).hasAttribute("disabled"),
    ).toBe(false),
  );
  fireEvent.click(screen.getByRole("button", { name: "检查费用" }));
  await waitFor(() => expect(api.quote).toHaveBeenCalledOnce());
  const draft = api.save.mock.calls[0][1].draft;
  expect(draft.mode).toBe("image_to_image");
  expect(draft.references).toEqual([
    expect.objectContaining({
      role: "reference_image",
      source: { kind: "media", sourceId: id, sourceVersion: "3" },
    }),
  ]);
  expect(api.quote.mock.calls[0][1].input_source.revision).toBe(2);
  expect(api.confirm).not.toHaveBeenCalled();
});

it("模型不支持参考时必须明确移除参考和标签，不能静默丢弃", async () => {
  await open(true);
  expect(
    screen.getByRole("button", { name: "检查费用" }).hasAttribute("disabled"),
  ).toBe(true);
  expect(api.quote).not.toHaveBeenCalled();
  fireEvent.click(screen.getByRole("button", { name: "移除" }));
  fireEvent.click(
    await screen.findByRole("button", { name: "移除参考及标签" }),
  );
  await waitFor(() =>
    expect(
      screen.getByRole("button", { name: "检查费用" }).hasAttribute("disabled"),
    ).toBe(false),
  );
});

it("报价后编辑输入会阻止确认旧报价", async () => {
  await open();
  fireEvent.click(screen.getByRole("button", { name: "检查费用" }));
  await screen.findByRole("button", { name: "确认费用并生成" });
  fireEvent.change(screen.getByLabelText("候选数量"), {
    target: { value: "2" },
  });
  expect(
    screen
      .getByRole("button", { name: "确认费用并生成" })
      .hasAttribute("disabled"),
  ).toBe(true);
  expect(api.confirm).not.toHaveBeenCalled();
});

it("确认结果未知时锁定编辑并使用原确认键重试", async () => {
  api.confirm.mockRejectedValueOnce(new RequestError("request_unconfirmed"));
  await open();
  fireEvent.click(screen.getByRole("button", { name: "检查费用" }));
  fireEvent.click(
    await screen.findByRole("button", { name: "确认费用并生成" }),
  );
  const retry = await screen.findByRole("button", { name: "重试原确认" });
  await waitFor(() => expect(retry.hasAttribute("disabled")).toBe(false));
  expect(screen.getByLabelText("生成模型").hasAttribute("disabled")).toBe(true);
  fireEvent.click(retry);
  await screen.findByRole("heading", { name: "已完成" });
  expect(api.confirm.mock.calls[0]).toEqual(api.confirm.mock.calls[1]);
});
