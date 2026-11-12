import { render, findFeedback } from "../../../feedback";
import {
  cleanup,
  fireEvent,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import {
  TaskDetail,
  GenerationDialog,
} from "@/components/generation/generation-dialog";
import { RequestError } from "@/lib/request";
import { RichGeneration } from "@/components/generation/rich-generation";
import { emptyPromptDraft } from "@/components/generation/prompt-document";
import { mediaBinding } from "@/components/generation/reference-controls";
import type { CanvasReferenceSource } from "@/components/generation/reference-source";

const api = vi.hoisted(() => ({
  models: vi.fn(),
  quote: vi.fn(),
  estimate: vi.fn(),
  confirm: vi.fn(),
  get: vi.fn(),
  list: vi.fn(),
  cancel: vi.fn(),
  recover: vi.fn(),
  identity: vi.fn(),
  preferences: vi.fn(),
  create: vi.fn(),
  save: vi.fn(),
  current: vi.fn(),
  resolve: vi.fn(),
  preview: vi.fn(),
  validate: vi.fn(),
  upload: vi.fn(),
  uploadResult: vi.fn(),
  drafts: vi.fn(),
  branchList: vi.fn(),
  branchWrite: vi.fn(),
}));
vi.mock("@/api/identity", () => ({
  getSession: api.identity,
  getMyPreferences: api.preferences,
}));
vi.mock("@/api/models", () => ({ listProjectModels: api.models }));
vi.mock("@/api/operations", () => ({
  createFreeQuote: api.quote,
  estimateGeneration: api.estimate,
  confirmOperationQuote: api.confirm,
  getOperation: api.get,
  listProjectOperations: api.list,
  cancelOperation: api.cancel,
  recoverOperation: api.recover,
}));
vi.mock("@/api/promptEditor", () => ({
  validatePromptFragment: api.validate,
  createPromptDraft: api.create,
  savePromptDraft: api.save,
  readPromptDraft: vi.fn(),
  listPromptDrafts: api.drafts,
  currentPromptSubmission: api.current,
  resolvePromptSubmission: api.resolve,
  resolvePromptUpload: api.uploadResult,
}));
vi.mock("@/api/media", () => ({
  getMediaPreview: api.preview,
  uploadMediaAsset: api.upload,
}));
vi.mock("@/components/generation/draft-save", async (importOriginal) => ({
  ...(await importOriginal<
    typeof import("@/components/generation/draft-save")
  >()),
  branchJournal: {
    list: api.branchList,
    write: api.branchWrite,
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
  api.branchList.mockResolvedValue([]);
  api.branchWrite.mockResolvedValue(undefined);
  api.validate.mockResolvedValue({});
  api.estimate.mockResolvedValue({
    credits: "2.4",
    kind: "estimate",
    basis: "per_image",
    quantity: "2",
  });
  api.uploadResult.mockResolvedValue({ found: false });
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
  api.preferences.mockResolvedValue({
    prompt_suffix: "",
    theme: "system",
    revision: "1",
  });
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
        available: true,
        mode_availability: [
          { mode: "text_to_image", available: true },
          { mode: "text2audio", available: true },
        ],
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
it("音乐空目录保留纯音乐草稿，不显示文本朗读", async () => {
  api.models.mockResolvedValue({ items: [] });
  const composer = {
    ...emptyPromptDraft(),
    mode: "music_instrumental",
    params: { duration_ms: 60000 },
  };
  render(
    <RichGeneration
      projectID={project.id}
      seed={{
        project,
        capability: "audio",
        creationType: "music",
        prompt: "",
        composer,
      }}
      presentation="home"
    />,
  );
  await screen.findByLabelText("音乐创作编辑器");
  expect(screen.getByText("纯音乐")).toBeTruthy();
  expect(screen.queryByText("文本朗读")).toBeNull();
  expect(api.create.mock.calls[0][1].draft.mode).toBe("music_instrumental");
  expect(api.quote).not.toHaveBeenCalled();
});

it("动作模仿通过统一参考入口添加内容且没有可见正文", async () => {
  api.models.mockResolvedValue({ items: [] });
  const composer = { ...emptyPromptDraft(), mode: "motion" };
  render(
    <RichGeneration
      projectID={project.id}
      seed={{
        project,
        capability: "video",
        creationType: "motion",
        prompt: "",
        composer,
      }}
      presentation="home"
    />,
  );
  await screen.findByLabelText("动作模仿创作编辑器");
  fireEvent.click(await screen.findByRole("button", { name: "添加参考" }));
  expect(
    await screen.findByRole("option", { name: "从参考素材添加" }),
  ).toBeTruthy();
  expect(screen.getByRole("option", { name: "上传参考内容" })).toBeTruthy();
  expect(screen.queryByRole("textbox", { name: "创作提示词" })).toBeNull();
  expect(api.quote).not.toHaveBeenCalled();
});

it("首页内联富文本保存正文并保留草稿找回入口，不触发报价", async () => {
  const onDraftReady = vi.fn();
  api.drafts.mockResolvedValue({ items: [] });
  render(
    <RichGeneration
      projectID={project.id}
      presentation="home"
      seed={{ project, prompt: "首页初始正文", capability: "image" }}
      onDraftReady={onDraftReady}
    />,
  );
  const editor = await screen.findByRole("textbox", { name: "创作提示词" });
  expect(editor.getAttribute("contenteditable")).toBe("true");
  expect(onDraftReady).toHaveBeenCalledWith(id);
  fireEvent.paste(editor, {
    clipboardData: {
      files: [],
      getData: (type: string) => (type === "text/plain" ? "首页补充正文" : ""),
    },
  });
  fireEvent.pointerDown(screen.getByRole("button", { name: "更多工具" }), {
    button: 0,
    ctrlKey: false,
  });
  fireEvent.click(await screen.findByRole("menuitem", { name: "保存草稿" }));
  await waitFor(() =>
    expect(
      JSON.stringify(api.save.mock.calls.at(-1)?.[1].draft.document),
    ).toContain("首页补充正文"),
  );
  fireEvent.pointerDown(screen.getByRole("button", { name: "更多工具" }), {
    button: 0,
    ctrlKey: false,
  });
  fireEvent.click(await screen.findByRole("menuitem", { name: "找回草稿" }));
  expect(await screen.findByText("暂无保存草稿")).toBeTruthy();
  expect(api.drafts).toHaveBeenCalledWith({ pid: project.id });
  expect(screen.getByRole("textbox", { name: "创作提示词" })).toBe(editor);
  expect(api.quote).not.toHaveBeenCalled();
  expect(api.confirm).not.toHaveBeenCalled();
});
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
  await waitFor(() =>
    expect(screen.getByRole("combobox", { name: "生成模型" })).toHaveProperty(
      "disabled",
      false,
    ),
  );
  fireEvent.keyDown(screen.getByRole("combobox", { name: "生成模型" }), {
    key: "ArrowDown",
  });
  fireEvent.click(await screen.findByRole("option", { name: /真实模型/ }));
  await waitFor(() =>
    expect(
      screen.getByRole("combobox", { name: "生成模式" }).textContent,
    ).toContain("text_to_image"),
  );
}

async function compactWorkspace(
  withReference = false,
  sources: CanvasReferenceSource[] = [],
) {
  const draft = emptyPromptDraft();
  draft.document.content[0].content = [
    { type: "text", text: "保留同一富文本会话" },
  ];
  draft.modelVersion = version;
  draft.mode = "text_to_image";
  draft.params = { output_count: 1, aspect_ratio: "16:9" };
  const models = await api.models();
  if (withReference) {
    const binding = mediaBinding(
      { id, kind: "image", revision: 3 },
      "reference_image",
    );
    draft.references.push(binding);
    draft.labelState.highWater.image = 1;
    draft.labelState.allocations[binding.id] = {
      labelKind: "image",
      ordinal: 1,
    };
    models.items[0].input_roles = ["reference_image"];
    models.items[0].current_version.limits.roles = {
      reference_image: { types: ["image"], max_count: 4 },
    };
  }
  models.items[0].current_version.param_schema = [
    {
      field: "aspect_ratio",
      label: "画幅",
      type: "string",
      component: "select",
      enum: ["16:9", "1:1"],
    },
  ];
  api.models.mockResolvedValue(models);
  const pointer = {
    kind: "canvas_input" as const,
    canvas_id: project.canvas_id,
    node_id: id,
    canvas_revision: 7,
    input_revision: 2,
  };
  const canvas = {
    contextKey: `canvas/${id}`,
    draft,
    onChange: vi.fn(),
    waitForNode: vi.fn(async () => {}),
    referenceSources: sources,
    flush: vi.fn(async () => pointer),
  };
  const props = {
    projectID: project.id,
    presentation: "canvas" as const,
    canvas,
    seed: {
      project,
      prompt: "",
      capability: "image" as const,
      source: {
        canvas_id: project.canvas_id,
        node_id: id,
        revision: 7,
      },
    },
  };
  const view = render(<RichGeneration {...props} />);
  await screen.findByRole("textbox", { name: "创作提示词" });
  await waitFor(() =>
    expect(
      screen.getByRole("button", { name: "检查费用" }).hasAttribute("disabled"),
    ).toBe(false),
  );
  return { ...view, props, canvas, pointer };
}

it("画布紧凑呈现等新节点保存但不物化空输入，重渲染不重新初始化", async () => {
  const { props, canvas, rerender } = await compactWorkspace();
  const editor = screen.getByRole("textbox", { name: "创作提示词" });
  expect(editor.getAttribute("contenteditable")).toBe("true");
  expect(editor.getAttribute("data-placeholder")).toBe("描述创意，@ 引用素材");
  expect(editor.closest('[data-slot="input-group-control"]')).toBeTruthy();
  expect(
    screen.getByRole("combobox", { name: "生成模型" }).textContent,
  ).toContain("真实模型");
  expect(screen.queryByRole("combobox", { name: "生成模式" })).toBeNull();
  expect(screen.queryByText("全能参考")).toBeNull();
  expect(canvas.flush).not.toHaveBeenCalled();
  expect(canvas.waitForNode).toHaveBeenCalledOnce();
  expect(canvas.waitForNode.mock.invocationCallOrder[0]).toBeLessThan(
    api.current.mock.invocationCallOrder[0],
  );
  expect(api.current.mock.calls[0][0]).toMatchObject({
    pid: project.id,
    owner_kind: "canvas_input",
    canvas_id: project.canvas_id,
    node_id: id,
  });
  expect(api.create).not.toHaveBeenCalled();
  rerender(<RichGeneration {...props} canvas={{ ...canvas }} />);
  expect(screen.getByRole("textbox", { name: "创作提示词" })).toBe(editor);
  expect(canvas.flush).not.toHaveBeenCalled();
  expect(canvas.waitForNode).toHaveBeenCalledOnce();
});

it("紧凑参数使用模型schema与候选上限，同一画布来源报价后才允许确认", async () => {
  const { canvas, pointer } = await compactWorkspace();
  fireEvent.click(screen.getByRole("button", { name: "生成参数" }));
  expect(screen.getByRole("combobox", { name: "画面比例" })).toBeTruthy();
  fireEvent.keyDown(screen.getByRole("combobox", { name: "生成数量" }), {
    key: "ArrowDown",
  });
  expect(await screen.findAllByRole("option")).toHaveLength(4);
  fireEvent.click(screen.getByRole("option", { name: "2" }));
  await waitFor(() =>
    expect(canvas.onChange).toHaveBeenLastCalledWith(
      expect.objectContaining({
        params: expect.objectContaining({ output_count: 2 }),
      }),
    ),
  );
  fireEvent.keyDown(document, { key: "Escape" });
  fireEvent.click(screen.getByRole("button", { name: "检查费用" }));
  await screen.findByRole("button", { name: "确认生成" });
  expect(api.quote.mock.calls[0][1].input_source).toEqual(pointer);
  expect(api.confirm).not.toHaveBeenCalled();
  fireEvent.click(screen.getByRole("button", { name: "确认生成" }));
  await waitFor(() => expect(api.confirm).toHaveBeenCalledOnce());
});

it("已选项目但没有可用模型时，参数入口显示模型前提且不触发报价", async () => {
  api.models.mockResolvedValue({ items: [] });
  render(
    <RichGeneration
      projectID={project.id}
      presentation="home"
      seed={{ project, prompt: "保留正文", capability: "image" }}
    />,
  );
  await screen.findByRole("textbox", { name: "创作提示词" });
  const trigger = screen.getByRole("button", { name: "生成参数" });
  await waitFor(() => expect(trigger.hasAttribute("disabled")).toBe(false));
  fireEvent.click(trigger);
  expect(
    await screen.findByText("请先选择可用模型，再配置生成参数。"),
  ).toBeTruthy();
  expect(screen.queryByRole("combobox", { name: "所属项目" })).toBeNull();
  expect(api.quote).not.toHaveBeenCalled();
});

it("默认全能参考来自真实目录合同，无模式切换且不重建正文或触发生成", async () => {
  const models = await api.models();
  models.items[0].current_version.modes.push("omni_reference");
  models.items[0].mode_availability.push({
    mode: "omni_reference",
    available: true,
  });
  api.models.mockResolvedValue(models);
  const { canvas } = await compactWorkspace();
  const editor = screen.getByRole("textbox", { name: "创作提示词" });
  expect(screen.queryByRole("combobox", { name: "生成模式" })).toBeNull();
  expect(screen.queryByText("需要调整")).toBeNull();
  expect(screen.getByText("全能参考")).toBeTruthy();
  await waitFor(() =>
    expect(canvas.onChange.mock.calls.at(-1)?.[0].mode).toBe("omni_reference"),
  );
  expect(screen.getByRole("textbox", { name: "创作提示词" })).toBe(editor);
  expect(editor.textContent).toContain("保留同一富文本会话");
  expect(api.quote).not.toHaveBeenCalled();
  expect(api.confirm).not.toHaveBeenCalled();
});

it("紧凑加号与提及入口仍调用已有引用控制器", async () => {
  await compactWorkspace();
  fireEvent.click(screen.getByRole("button", { name: "添加参考" }));
  expect(await screen.findByRole("option", { name: "添加主体" })).toBeTruthy();
  expect(screen.getByRole("option", { name: "上传参考内容" })).toBeTruthy();
  expect(screen.getByRole("option", { name: "粘贴站内引用" })).toBeTruthy();
  fireEvent.keyDown(document, { key: "Escape" });
  fireEvent.click(screen.getByRole("button", { name: "提及素材或主体" }));
  expect(await screen.findByRole("dialog", { name: "引用候选" })).toBeTruthy();
  expect(document.querySelector('[data-slot="dialog-overlay"]')).toBeNull();
  await waitFor(() =>
    expect(document.activeElement).toBe(
      screen.getByRole("textbox", { name: "创作提示词" }),
    ),
  );
  expect(screen.getByRole("option", { name: "主体" })).toBeTruthy();
  expect(screen.getByRole("option", { name: "图片" })).toBeTruthy();
  expect(
    screen.queryByRole("option", { name: "搜索项目素材与主体" }),
  ).toBeNull();
});

it("@分类候选展示已连接内容，核验后替换为真实引用且不混入未连接素材", async () => {
  const linked = {
    nodeID: crypto.randomUUID(),
    edgeID: crypto.randomUUID(),
    title: "已连接海报",
    kind: "image" as const,
    assetID: id,
  };
  const { canvas, props, rerender } = await compactWorkspace(false, [
    linked,
    {
      nodeID: crypto.randomUUID(),
      title: "未连接海报",
      kind: "image",
      assetID: crypto.randomUUID(),
    },
  ]);
  rerender(
    <RichGeneration
      {...props}
      canvas={{ ...canvas, incomingSources: [linked] }}
    />,
  );
  api.preview.mockResolvedValue({
    asset: { id, project_id: project.id, kind: "image", revision: 3 },
  });
  fireEvent.click(screen.getByRole("button", { name: "提及素材或主体" }));
  expect(
    await screen.findByRole("option", { name: "已连接海报" }),
  ).toBeTruthy();
  expect(screen.queryByRole("dialog", { name: "图片引用候选" })).toBeNull();
  fireEvent.click(await screen.findByRole("option", { name: "图片" }));
  expect(screen.queryByRole("option", { name: /未连接海报/ })).toBeNull();
  const submenu = await screen.findByRole("dialog", { name: "图片引用候选" });
  fireEvent.click(within(submenu).getByRole("option", { name: /已连接海报/ }));
  await waitFor(() =>
    expect(canvas.onChange.mock.calls.at(-1)?.[0].references).toEqual([
      expect.objectContaining({
        id: linked.edgeID,
        source: { kind: "media", sourceId: id, sourceVersion: "3" },
      }),
    ]),
  );
  expect(
    screen
      .getByRole("textbox", { name: "创作提示词" })
      .querySelectorAll("[data-prompt-reference]"),
  ).toHaveLength(1);
  expect(screen.queryByRole("dialog", { name: "引用候选" })).toBeNull();
});

it("@核验期间连接被移除时拒绝旧结果，不把断线素材加入正文", async () => {
  const linked = {
    nodeID: crypto.randomUUID(),
    edgeID: crypto.randomUUID(),
    title: "待核验海报",
    kind: "image" as const,
    assetID: id,
  };
  const { canvas, props, rerender } = await compactWorkspace();
  rerender(
    <RichGeneration
      {...props}
      canvas={{ ...canvas, incomingSources: [linked] }}
    />,
  );
  let resolve!: (value: { asset: API.AssetSummary }) => void;
  api.preview.mockImplementation(
    () =>
      new Promise((done) => {
        resolve = done;
      }),
  );
  fireEvent.click(screen.getByRole("button", { name: "提及素材或主体" }));
  fireEvent.click(await screen.findByRole("option", { name: "图片" }));
  const submenu = await screen.findByRole("dialog", { name: "图片引用候选" });
  fireEvent.click(within(submenu).getByRole("option", { name: /待核验海报/ }));
  await waitFor(() => expect(resolve).toBeTypeOf("function"));
  rerender(
    <RichGeneration {...props} canvas={{ ...canvas, incomingSources: [] }} />,
  );
  resolve({
    asset: { id, project_id: project.id, kind: "image", revision: 3 },
  });
  await findFeedback("参考连接或来源已变化");
  expect(canvas.onChange.mock.calls.at(-1)?.[0].references ?? []).toEqual([]);
  expect(screen.getByRole("textbox", { name: "创作提示词" }).textContent).toBe(
    "@保留同一富文本会话",
  );
});

it("从画布添加先核验真实素材版本，同一来源不重复且不触发生成", async () => {
  const source = {
    nodeID: crypto.randomUUID(),
    title: "画布图片",
    kind: "image" as const,
    assetID: id,
  };
  const { canvas } = await compactWorkspace(false, [
    source,
    { nodeID: crypto.randomUUID(), title: "空图片", kind: "image" },
  ]);
  api.preview.mockResolvedValue({
    asset: { id, project_id: project.id, kind: "image", revision: 3 },
  });
  for (let attempt = 0; attempt < 2; attempt++) {
    fireEvent.click(screen.getByRole("button", { name: "添加参考" }));
    fireEvent.click(await screen.findByRole("option", { name: "从画布选择" }));
    expect(
      screen
        .getByRole("option", { name: /空图片/ })
        .getAttribute("aria-disabled"),
    ).toBe("true");
    fireEvent.click(screen.getByRole("option", { name: "画布图片" }));
    await waitFor(() => expect(api.preview).toHaveBeenCalledTimes(attempt + 1));
    await waitFor(() =>
      expect(screen.queryByRole("dialog", { name: "添加参考素材" })).toBeNull(),
    );
  }
  expect(canvas.onChange.mock.calls.at(-1)?.[0].references).toEqual([
    expect.objectContaining({
      source: { kind: "media", sourceId: id, sourceVersion: "3" },
    }),
  ]);
  expect(api.preview).toHaveBeenCalledWith(
    { pid: project.id, asset_id: id },
    expect.objectContaining({ signal: expect.any(AbortSignal) }),
  );
  expect(api.quote).not.toHaveBeenCalled();
  expect(api.confirm).not.toHaveBeenCalled();
});

it("画布素材回执跨项目时拒绝添加，保留原正文与输入", async () => {
  const { canvas } = await compactWorkspace(false, [
    {
      nodeID: crypto.randomUUID(),
      title: "画布图片",
      kind: "image",
      assetID: id,
    },
  ]);
  api.preview.mockResolvedValue({
    asset: { id, project_id: crypto.randomUUID(), kind: "image", revision: 3 },
  });
  fireEvent.click(screen.getByRole("button", { name: "添加参考" }));
  fireEvent.click(await screen.findByRole("option", { name: "从画布选择" }));
  fireEvent.click(screen.getByRole("option", { name: "画布图片" }));
  await findFeedback("参考素材范围不匹配");
  expect(canvas.onChange.mock.calls.at(-1)?.[0].references ?? []).toEqual([]);
  expect(
    screen.getByRole("textbox", { name: "创作提示词" }).textContent,
  ).toContain("保留同一富文本会话");
});

it("空上游只显示禁用占位，移除真实参考同时清除正文标签", async () => {
  const { canvas, props, rerender } = await compactWorkspace(true);
  rerender(
    <RichGeneration
      {...props}
      canvas={{
        ...canvas,
        incomingSources: [
          {
            nodeID: crypto.randomUUID(),
            edgeID: crypto.randomUUID(),
            title: "空上游",
            kind: "image",
          },
        ],
      }}
    />,
  );
  expect(
    screen
      .getByRole("button", { name: "空上游：内容为空" })
      .hasAttribute("disabled"),
  ).toBe(true);
  fireEvent.click(screen.getByRole("button", { name: "查看图片1详情" }));
  fireEvent.click(await screen.findByRole("button", { name: "移除" }));
  fireEvent.click(
    await screen.findByRole("button", { name: "移除参考及标签" }),
  );
  await waitFor(() =>
    expect(canvas.onChange.mock.calls.at(-1)?.[0].references).toHaveLength(0),
  );
  expect(
    JSON.stringify(canvas.onChange.mock.calls.at(-1)?.[0].document),
  ).not.toContain('"type":"reference"');
  expect(
    canvas.onChange.mock.calls.at(-1)?.[0].labelState.highWater.image,
  ).toBe(1);
  expect(api.confirm).not.toHaveBeenCalled();
});

it("紧凑引用默认缩略，详情内提及写入同一富文本会话且展开不重建", async () => {
  const { canvas } = await compactWorkspace(true);
  const editor = screen.getByRole("textbox", { name: "创作提示词" });
  expect(screen.queryByLabelText("图片1角色")).toBeNull();
  fireEvent.click(screen.getByRole("button", { name: "查看图片1详情" }));
  await screen.findByRole("dialog", { name: "参考详情" });
  expect(screen.getByLabelText("图片1角色")).toBeTruthy();
  fireEvent.click(screen.getByRole("button", { name: "提及" }));
  await waitFor(() =>
    expect(
      JSON.stringify(canvas.onChange.mock.calls.at(-1)?.[0].document),
    ).toContain('"type":"reference"'),
  );
  expect(screen.queryByRole("dialog", { name: "参考详情" })).toBeNull();
  fireEvent.click(screen.getByRole("button", { name: "展开编辑器" }));
  expect(screen.getByRole("textbox", { name: "创作提示词" })).toBe(editor);
  expect(editor.textContent).toContain("保留同一富文本会话");
  expect(editor.textContent).toContain("图片1");
  expect(editor.closest("[data-expanded]")?.getAttribute("data-expanded")).toBe(
    "true",
  );
  fireEvent.keyDown(editor, { key: "Escape" });
  expect(screen.getByRole("button", { name: "展开编辑器" })).toBeTruthy();
  expect(screen.getByRole("textbox", { name: "创作提示词" })).toBe(editor);
});

it("紧凑上传进入同一审核与恢复流程，失败重试复用引用身份和幂等键", async () => {
  const { canvas } = await compactWorkspace(true);
  const assetID = "55555555-5555-4555-8555-555555555555";
  api.upload
    .mockRejectedValueOnce(new RequestError("request_unconfirmed"))
    .mockResolvedValueOnce({
      asset: {
        id: assetID,
        project_id: project.id,
        kind: "image",
        revision: 4,
      },
    });
  fireEvent.click(screen.getByRole("button", { name: "添加参考" }));
  fireEvent.change(screen.getByLabelText("上传参考文件"), {
    target: {
      files: [new File(["fixture"], "reference.png", { type: "image/png" })],
    },
  });
  await screen.findByRole("dialog", { name: "参考详情" });
  const upload = screen.getByRole("button", { name: "上传 / 重试原请求" });
  expect(upload.hasAttribute("disabled")).toBe(true);
  fireEvent.click(
    screen.getByRole("button", { name: "检查费用", hidden: true }),
  );
  expect(api.quote).not.toHaveBeenCalled();
  fireEvent.click(
    screen.getByRole("checkbox", { name: "我已确认这些素材可以用于本次创作" }),
  );
  fireEvent.click(upload);
  await waitFor(() => expect(api.upload).toHaveBeenCalledOnce());
  await waitFor(() =>
    expect(
      screen
        .getByRole("button", { name: "上传 / 重试原请求" })
        .hasAttribute("disabled"),
    ).toBe(false),
  );
  fireEvent.click(screen.getByRole("button", { name: "上传 / 重试原请求" }));
  await waitFor(() => expect(api.upload).toHaveBeenCalledTimes(2));
  expect(api.upload.mock.calls[1]).toEqual(api.upload.mock.calls[0]);
  await waitFor(() =>
    expect(canvas.onChange.mock.calls.at(-1)?.[0].references).toEqual(
      expect.arrayContaining([
        expect.objectContaining({
          source: { kind: "media", sourceId: assetID, sourceVersion: "4" },
        }),
      ]),
    ),
  );
  expect(api.quote).not.toHaveBeenCalled();
});

it("模型目录503仍能输入和保存，局部重试恢复模型且保留正文引用与会话", async () => {
  api.models
    .mockRejectedValueOnce(new RequestError("dependency_unavailable", 503))
    .mockRejectedValueOnce(new RequestError("dependency_unavailable", 503));
  const draft = emptyPromptDraft();
  draft.document.content[0].content = [{ type: "text", text: "初始正文" }];
  draft.modelVersion = version;
  draft.mode = "text_to_image";
  const binding = mediaBinding(
    { id, kind: "image", revision: 3 },
    "reference_image",
  );
  draft.references = [binding];
  draft.labelState.highWater.image = 1;
  draft.labelState.allocations[binding.id] = { labelKind: "image", ordinal: 1 };
  const canvas = {
    contextKey: "model-failure",
    draft,
    onChange: vi.fn(),
    flush: vi.fn(async () => ({
      kind: "canvas_input" as const,
      canvas_id: project.canvas_id,
      node_id: id,
      canvas_revision: 1,
      input_revision: 1,
    })),
  };
  render(
    <RichGeneration
      projectID={project.id}
      presentation="canvas"
      canvas={canvas}
      seed={{
        project,
        prompt: "",
        capability: "image",
        source: {
          canvas_id: project.canvas_id,
          node_id: id,
          revision: 1,
        },
      }}
    />,
  );
  const editor = await screen.findByRole("textbox", { name: "创作提示词" });
  expect(editor.getAttribute("contenteditable")).toBe("true");
  expect(await findFeedback("模型暂不可用")).toBeTruthy();
  expect(screen.queryByText(/dependency_unavailable/)).toBeNull();
  expect(document.querySelector('[role="alert"]')).toBeNull();
  expect(screen.getByRole("button", { name: "重试模型目录" })).toBeTruthy();
  fireEvent.click(screen.getByRole("button", { name: "检查费用" }));
  expect(api.quote).not.toHaveBeenCalled();
  expect(screen.queryByText("需要调整")).toBeNull();
  fireEvent.paste(editor, {
    clipboardData: {
      files: [],
      getData: (type: string) =>
        type === "text/plain" ? "目录失败时新增正文" : "",
    },
  });
  await waitFor(() =>
    expect(
      JSON.stringify(canvas.onChange.mock.calls.at(-1)?.[0].document),
    ).toContain("目录失败时新增正文"),
  );
  fireEvent.keyDown(editor, { key: "s", ctrlKey: true });
  await waitFor(() => expect(canvas.flush).toHaveBeenCalledOnce());
  const edited = canvas.onChange.mock.calls.at(-1)?.[0];
  const identityCalls = api.identity.mock.calls.length;
  const submissionCalls = api.current.mock.calls.length;
  fireEvent.click(screen.getByRole("button", { name: "重试模型目录" }));
  await waitFor(() => expect(api.models).toHaveBeenCalledTimes(2));
  expect(await findFeedback("模型暂不可用")).toBeTruthy();
  fireEvent.click(screen.getByRole("button", { name: "重试" }));
  await waitFor(() =>
    expect(screen.queryByRole("button", { name: "重试模型目录" })).toBeNull(),
  );
  expect(
    screen.getByRole("combobox", { name: "生成模型" }).textContent,
  ).toContain("真实模型");
  expect(screen.getByRole("textbox", { name: "创作提示词" })).toBe(editor);
  expect(canvas.onChange.mock.calls.at(-1)?.[0]).toEqual(edited);
  expect(edited.references[0]).toEqual(binding);
  expect(api.identity).toHaveBeenCalledTimes(identityCalls);
  expect(api.current).toHaveBeenCalledTimes(submissionCalls);
  expect(canvas.flush).toHaveBeenCalledOnce();
  expect(api.quote).not.toHaveBeenCalled();
});

it("未确认的报价重试复用保存后的草稿指针和幂等键，确认前不提交生成", async () => {
  api.quote.mockRejectedValueOnce(new RequestError("request_unconfirmed"));
  await open();
  fireEvent.click(screen.getByRole("button", { name: "检查费用" }));
  await findFeedback();
  fireEvent.click(screen.getByRole("button", { name: "检查费用" }));
  await screen.findByRole("button", { name: "确认生成" });
  expect(api.models.mock.calls[0][0].capability).toBe("image.generate");
  expect(api.quote.mock.calls[0]).toEqual(api.quote.mock.calls[1]);
  expect(api.quote.mock.calls[0][1]).toEqual({
    input_source: { kind: "prompt_draft", draft_id: id, revision: 2 },
    expected_submission_epoch: 0,
    force_regenerate: false,
  });
  expect(api.confirm).not.toHaveBeenCalled();
  fireEvent.click(screen.getByRole("button", { name: "确认生成" }));
  await screen.findByRole("heading", { name: "已完成" });
  expect(api.confirm).toHaveBeenCalledOnce();
});

it("过期报价只允许更新费用，先释放旧报价再读取新报价", async () => {
  quote.expires_at = "2000-01-01";
  await open();
  fireEvent.click(screen.getByRole("button", { name: "检查费用" }));
  await screen.findByText("报价已过期，请重新检查费用。");
  expect(screen.queryByRole("button", { name: "确认生成" })).toBeNull();
  quote = { ...quote, expires_at: "2099-01-01" };
  fireEvent.click(screen.getByRole("button", { name: "更新费用" }));
  await screen.findByRole("button", { name: "确认生成" });
  expect(api.resolve.mock.calls[0][1]).toEqual({
    action: "discard_quote",
    expected_epoch: 1,
  });
  expect(api.quote).toHaveBeenCalledTimes(2);
  expect(api.resolve.mock.invocationCallOrder[0]).toBeLessThan(
    api.quote.mock.invocationCallOrder[1],
  );
  expect(api.quote.mock.calls[1][1].expected_submission_epoch).toBe(2);
  expect(api.quote.mock.calls[0][2].headers["Idempotency-Key"]).not.toBe(
    api.quote.mock.calls[1][2].headers["Idempotency-Key"],
  );
  expect(api.confirm).not.toHaveBeenCalled();
});

it("更新费用时释放旧报价失败，不创建另一报价或执行确认", async () => {
  quote.expires_at = "2000-01-01";
  await open();
  fireEvent.click(screen.getByRole("button", { name: "检查费用" }));
  await screen.findByRole("button", { name: "更新费用" });
  api.resolve.mockRejectedValueOnce(new RequestError("request_unconfirmed"));
  fireEvent.click(screen.getByRole("button", { name: "更新费用" }));
  await findFeedback();
  expect(api.quote).toHaveBeenCalledOnce();
  expect(api.confirm).not.toHaveBeenCalled();
  expect(screen.queryByRole("button", { name: "确认生成" })).toBeNull();
});

it("画布参考保留版本与模型角色，选择图生图模式后先保存草稿再报价", async () => {
  const models = await api.models();
  models.items[0].input_roles = ["reference_image"];
  models.items[0].current_version.modes = ["text_to_image", "image_to_image"];
  models.items[0].mode_availability = [
    { mode: "text_to_image", available: true },
    { mode: "image_to_image", available: true },
  ];
  models.items[0].current_version.limits.roles = {
    reference_image: { types: ["image"], for_modes: ["image_to_image"] },
  };
  await open(true);
  expect(
    screen.getByRole("button", { name: "检查费用" }).hasAttribute("disabled"),
  ).toBe(true);
  fireEvent.keyDown(screen.getByRole("combobox", { name: "生成模式" }), {
    key: "ArrowDown",
  });
  fireEvent.click(
    await screen.findByRole("option", { name: "image_to_image" }),
  );
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
  await screen.findByRole("button", { name: "确认生成" });
  fireEvent.change(screen.getByLabelText("候选数量"), {
    target: { value: "2" },
  });
  expect(screen.queryByRole("button", { name: "确认生成" })).toBeNull();
  expect(screen.getByRole("button", { name: "更新费用" })).toBeTruthy();
  expect(api.confirm).not.toHaveBeenCalled();
});

it("确认结果未知时锁定编辑并使用原确认键重试", async () => {
  api.confirm.mockRejectedValueOnce(new RequestError("request_unconfirmed"));
  await open();
  fireEvent.click(screen.getByRole("button", { name: "检查费用" }));
  fireEvent.click(await screen.findByRole("button", { name: "确认生成" }));
  const retry = await screen.findByRole("button", { name: "核对状态" });
  await waitFor(() => expect(retry.hasAttribute("disabled")).toBe(false));
  expect(screen.getByLabelText("生成模型").hasAttribute("disabled")).toBe(true);
  fireEvent.click(retry);
  await screen.findByRole("heading", { name: "已完成" });
  expect(api.confirm.mock.calls[0]).toEqual(api.confirm.mock.calls[1]);
});

it("模式查询发现当前模型停用后保留输入并禁用报价", async () => {
  const page = await api.models();
  page.items[0].current_version.modes = ["text_to_image", "image_to_image"];
  page.items[0].mode_availability = [
    { mode: "text_to_image", available: true },
    { mode: "image_to_image", available: true },
  ];
  await open();
  api.models.mockResolvedValue({
    items: [{ ...page.items[0], available: false, status: "disabled" }],
  });
  fireEvent.keyDown(screen.getByRole("combobox", { name: "生成模式" }), {
    key: "ArrowDown",
  });
  fireEvent.click(
    await screen.findByRole("option", { name: "image_to_image" }),
  );
  await screen.findByText("该模型当前不可用");
  expect(
    screen.getByRole("button", { name: "检查费用" }).hasAttribute("disabled"),
  ).toBe(true);
  expect(screen.getByRole("combobox", { name: "生成模式" }).textContent).toBe(
    "text_to_image",
  );
  expect(
    screen.getByRole("textbox", { name: "创作提示词" }).textContent,
  ).toContain("清晨的海边");
  expect(api.quote).not.toHaveBeenCalled();
});

it("重新显示页面时重新核验参考，撤权后保留正文并阻止报价", async () => {
  const page = await api.models();
  page.items[0].input_roles = ["reference_image"];
  page.items[0].current_version.limits.roles = {
    reference_image: { types: ["image"] },
  };
  await open(true);
  await waitFor(() =>
    expect(
      screen.getByRole("button", { name: "检查费用" }).hasAttribute("disabled"),
    ).toBe(false),
  );
  api.validate.mockRejectedValue(new Error("来源权限已撤销"));
  fireEvent(document, new Event("visibilitychange"));
  await waitFor(() => expect(api.validate).toHaveBeenCalledTimes(2));
  await screen.findByText("参考已失效或暂时无法核验，请重新选择或重新核验。");
  expect(
    screen.getByRole("button", { name: "检查费用" }).hasAttribute("disabled"),
  ).toBe(true);
  expect(
    screen.getByRole("textbox", { name: "创作提示词" }).textContent,
  ).toContain("清晨的海边");
  expect(api.quote).not.toHaveBeenCalled();
  api.validate.mockResolvedValue({});
  fireEvent.click(screen.getByRole("button", { name: "重新核验参考" }));
  await waitFor(() =>
    expect(
      screen.getByRole("button", { name: "检查费用" }).hasAttribute("disabled"),
    ).toBe(false),
  );
});

it("恢复报价即使正文相同也必须核对独立草稿修订", async () => {
  api.create.mockImplementation(async (_, body) => {
    const result = saved(body);
    result.revision = 2;
    result.draft = {
      ...result.draft,
      modelVersion: version,
      mode: "text_to_image",
    };
    current = {
      epoch: 1,
      submission: {
        id,
        state: "quoted",
        operation_id: id,
        confirm_key: "confirm-key",
        quote,
        input_source: { kind: "prompt_draft", draft_id: id, revision: 1 },
        draft: {
          ...result.draft,
          mode: "text_to_image",
          modelVersion: version,
        },
      },
    };
    return result;
  });
  render(
    <GenerationDialog
      seed={{ project, prompt: "清晨的海边", capability: "image" }}
      onClose={() => {}}
    />,
  );
  await screen.findByRole("button", { name: "更新费用" });
  expect(screen.queryByRole("button", { name: "确认生成" })).toBeNull();
  expect(api.confirm).not.toHaveBeenCalled();
});

it("画布共享富文本不读写本地恢复分支", async () => {
  await compactWorkspace();
  expect(api.branchList).not.toHaveBeenCalled();
  expect(api.branchWrite).not.toHaveBeenCalled();
  expect(screen.queryByText(/恢复.*分支/)).toBeNull();
});

it.each([
  ["audio", "audio.generate"],
  ["image", "image.edit"],
  ["text", "text.structured"],
] as const)(
  "再次创作%s使用原始%s能力筛选模型，进入时不报价",
  async (capability, exactCapability) => {
    render(
      <GenerationDialog
        seed={{
          project,
          prompt: "",
          capability,
          reuse: {
            id,
            project_id: project.id,
            status: "completed",
            capability: exactCapability,
            inputs: [{ role: "prompt", text: "真实历史描述" }],
            outputs: [],
          },
        }}
        onClose={() => {}}
      />,
    );
    await screen.findByRole("textbox", { name: "创作提示词" });
    expect(api.models).toHaveBeenCalledWith(
      expect.objectContaining({ capability: exactCapability }),
      expect.anything(),
    );
    api.list.mockResolvedValue({ items: [] });
    fireEvent.click(screen.getByRole("button", { name: "查看生成历史" }));
    await waitFor(() =>
      expect(api.list).toHaveBeenCalledWith(
        expect.objectContaining({ capability: exactCapability }),
      ),
    );
    expect(api.quote).not.toHaveBeenCalled();
    expect(api.confirm).not.toHaveBeenCalled();
  },
);
it("文档详情只展开选定结果，仍保留完整任务作为复用来源", async () => {
  const other = "55555555-5555-4555-8555-555555555555";
  const task: API.TaskDetail = {
    id,
    project_id: project.id,
    status: "completed",
    billing_outcome: {
      charge_state: "settled",
      charged_micros: 1,
      delivery_state: "ready",
      regenerate: "confirm",
      provider_settlement_state: "unknown",
      reconciliation_required: false,
    },
    capability: "text.structured",
    inputs: [{ role: "prompt", text: "原始提示词" }],
    params: { seed: 42 },
    outputs: [
      { id, kind: "json", moderation_status: "passed", text: "第一个文档" },
      {
        id: other,
        kind: "json",
        moderation_status: "passed",
        text: "第二个文档",
      },
    ],
  };
  api.get.mockResolvedValue(task);
  const reuse = vi.fn();
  render(
    <TaskDetail
      projectID={project.id}
      id={id}
      outputID={other}
      presentation="asset"
      onReuse={reuse}
    />,
  );
  await screen.findByText("第二个文档");
  expect(screen.queryByText("第一个文档")).toBeNull();
  expect(screen.getByText("原始提示词")).toBeTruthy();
  fireEvent.click(screen.getByRole("button", { name: "复用参数重新报价" }));
  expect(reuse).toHaveBeenCalledWith(task);
  expect(api.quote).not.toHaveBeenCalled();
});

it("asset detail accepts a completed official document and explains original-only preview", async () => {
  api.get.mockResolvedValue({
    id,
    project_id: project.id,
    capability: "document.generate",
    status: "completed",
    outputs: [
      { id, kind: "document", media_asset_id: id, moderation_status: "passed" },
    ],
  });
  render(
    <TaskDetail
      projectID={project.id}
      id={id}
      outputID={id}
      presentation="asset"
    />,
  );
  expect(
    await screen.findByText("正式文档仅提供原件，下载原件以查看。"),
  ).toBeTruthy();
});

it("文字结果采用等待画布保存并在失败时保留结果供重试", async () => {
  api.get.mockResolvedValue({
    id,
    project_id: project.id,
    status: "completed",
    capability: "text.generate",
    inputs: [],
    outputs: [
      { id, kind: "json", moderation_status: "passed", text: "可采用正文" },
    ],
  });
  let reject!: (cause: Error) => void;
  const adopt = vi.fn(
    () =>
      new Promise<void>((_, fail) => {
        reject = fail;
      }),
  );
  render(<TaskDetail projectID={project.id} id={id} onAdoptText={adopt} />);
  await screen.findByText("可采用正文");
  fireEvent.click(screen.getByRole("button", { name: "采用结果到画布" }));
  expect(adopt).toHaveBeenCalledWith(id, id);
  expect(
    (screen.getByRole("button", { name: "正在保存结果…" }) as HTMLButtonElement)
      .disabled,
  ).toBe(true);
  reject(new Error("节点结果已变化，请重新确认采用"));
  await findFeedback("节点结果已变化，请重新确认采用");
  expect(screen.getByText("可采用正文")).toBeTruthy();
  expect(
    (
      screen.getByRole("button", {
        name: "采用结果到画布",
      }) as HTMLButtonElement
    ).disabled,
  ).toBe(false);
});

it("助手紧凑展示保持提示词草稿归属并经正式报价后确认发送", async () => {
  render(
    <RichGeneration
      projectID={project.id}
      presentation="assistant"
      headerActions={<button>使用技能</button>}
      seed={{ project, prompt: "讨论海报构图", capability: "image" }}
    />,
  );
  const editor = await screen.findByRole("textbox", { name: "创作提示词" });
  expect(editor.getAttribute("contenteditable")).toBe("true");
  expect(editor.closest('[data-presentation="assistant"]')).toBeTruthy();
  expect(screen.getByRole("button", { name: "使用技能" })).toBeTruthy();
  expect(
    await screen.findByRole("button", { name: "从本地、画布或资产库添加" }),
  ).toBeTruthy();
  expect(screen.queryByRole("combobox", { name: "生成模式" })).toBeNull();
  expect(screen.queryByRole("combobox", { name: "生成模型" })).toBeNull();
  expect(screen.queryByText("全能参考")).toBeNull();
  fireEvent.click(screen.getByRole("button", { name: "模型与参数" }));
  fireEvent.keyDown(await screen.findByRole("combobox", { name: "生成模型" }), {
    key: "ArrowDown",
  });
  fireEvent.click(await screen.findByRole("option", { name: /真实模型/ }));
  await waitFor(() =>
    expect(
      api.save.mock.calls.some(
        (call) => call[1].draft.modelVersion === version,
      ),
    ).toBe(true),
  );
  fireEvent.click(screen.getByRole("button", { name: "发送消息" }));
  await waitFor(() => expect(api.quote).toHaveBeenCalledOnce());
  expect(api.quote.mock.calls[0]![1].input_source).toMatchObject({
    kind: "prompt_draft",
    draft_id: id,
  });
  expect(api.current.mock.calls[0]![0]).toMatchObject({
    owner_kind: "prompt_draft",
    draft_id: id,
  });
  expect(api.confirm).not.toHaveBeenCalled();
  expect(await screen.findByRole("dialog", { name: "确认生成" })).toBeTruthy();
});

it("音频目录与标题独立，朗读原文不追加视觉偏好且不显示素材入口", async () => {
  api.preferences.mockResolvedValue({ prompt_suffix: "视觉风格后缀" });
  api.models.mockResolvedValue({ items: [] });
  render(
    <GenerationDialog
      seed={{ project, prompt: "你好，欢迎回来。", capability: "audio" }}
      onClose={() => {}}
    />,
  );
  expect(screen.getByRole("heading", { name: "音频生成" })).toBeTruthy();
  await screen.findByRole("textbox", { name: "创作提示词" });
  expect(api.models.mock.calls[0][0].capability).toBe("audio.generate");
  expect(JSON.stringify(api.create.mock.calls[0][1])).not.toContain(
    "视觉风格后缀",
  );
  expect(screen.queryByRole("button", { name: "添加参考" })).toBeNull();
});

it("音频草稿保存后按来源指针报价，正文引用为空且不自动确认", async () => {
  const draft = emptyPromptDraft();
  draft.document.content[0].content = [{ type: "text", text: "欢迎回来。" }];
  draft.modelVersion = version;
  draft.mode = "text2audio";
  draft.params = { speaker: "fixture_voice", output_count: 1 };
  api.models.mockResolvedValue({
    items: [
      {
        id,
        key: "audio",
        display_name: "配音模型",
        capability: "audio.generate",
        status: "active",
        provider_status: "active",
        credential_present: true,
        available: true,
        mode_availability: [
          { mode: "text_to_image", available: true },
          { mode: "text2audio", available: true },
        ],
        current_version: {
          id: version,
          modes: ["text2audio"],
          limits: { max_outputs: 1 },
          param_schema: [
            {
              field: "speaker",
              label: "音色",
              type: "string",
              component: "input",
              default: "fixture_voice",
              required: true,
            },
          ],
        },
      },
    ],
  });
  render(
    <RichGeneration
      projectID={project.id}
      presentation="home"
      seed={{ project, prompt: "", composer: draft, capability: "audio" }}
    />,
  );
  await screen.findByRole("textbox", { name: "创作提示词" });
  fireEvent.click(screen.getByRole("button", { name: "检查费用" }));
  await screen.findByRole("button", { name: "确认生成" });
  expect(api.quote.mock.calls[0][1]).toMatchObject({
    input_source: { kind: "prompt_draft", draft_id: id },
  });
  expect(api.models.mock.calls[0][0].capability).toBe("audio.generate");
  expect(api.quote.mock.calls[0][1].capability).toBeUndefined();
  expect(api.create.mock.calls[0][1].draft.references).toEqual([]);
  expect(api.create.mock.calls[0][1].draft.mode).toBe("text2audio");
  expect(api.quote.mock.calls[0][1].media_inputs).toBeUndefined();
  expect(api.confirm).not.toHaveBeenCalled();
});

it("费用未决只允许原任务恢复且同一恢复请求复用幂等键", async () => {
  api.get.mockResolvedValue({
    id,
    project_id: project.id,
    status: "manual",
    outputs: [],
    billing_outcome: {
      charge_state: "pending",
      delivery_state: "recoverable",
      regenerate: "blocked",
      reconciliation_required: true,
    },
  });
  api.recover.mockRejectedValueOnce(new Error("reply lost")).mockResolvedValue({
    target_id: id,
    target_type: "operation",
    action: "recover",
    accepted: true,
  });
  render(<TaskDetail projectID={project.id} id={id} onReuse={vi.fn()} />);
  fireEvent.click(
    await screen.findByRole("button", { name: "继续取回原任务" }),
  );
  await waitFor(() => expect(api.recover).toHaveBeenCalledTimes(1));
  await waitFor(() =>
    expect(
      (
        screen.getByRole("button", {
          name: "继续取回原任务",
        }) as HTMLButtonElement
      ).disabled,
    ).toBe(false),
  );
  fireEvent.click(screen.getByRole("button", { name: "继续取回原任务" }));
  await waitFor(() => expect(api.recover).toHaveBeenCalledTimes(2));
  expect(api.recover.mock.calls[0][2].headers["Idempotency-Key"]).toBe(
    api.recover.mock.calls[1][2].headers["Idempotency-Key"],
  );
  expect(screen.queryByRole("button", { name: "复用参数重新报价" })).toBeNull();
  expect(api.confirm).not.toHaveBeenCalled();
  expect(api.quote).not.toHaveBeenCalled();
});

it("已扣费终态恢复交付后仍展示原正文，执行失败不伪称退款", async () => {
  api.get.mockResolvedValue({
    id,
    project_id: project.id,
    status: "failed",
    capability: "text.generate",
    outputs: [
      { id, kind: "json", moderation_status: "passed", text: "取回的原正文" },
    ],
    billing_outcome: {
      charge_state: "settled",
      charged_micros: 19,
      delivery_state: "ready",
      regenerate: "confirm",
      provider_settlement_state: "unknown",
      reconciliation_required: false,
    },
  });
  render(<TaskDetail projectID={project.id} id={id} />);
  await screen.findByText("取回的原正文");
  expect(screen.queryByText(/实扣：¥0.000019/)).toBeNull();
  fireEvent.click(screen.getByRole("button", { name: "费用记录" }));
  expect(screen.getByText(/实扣：¥0.000019/)).toBeTruthy();
  expect(screen.queryByText(/退款已入账/)).toBeNull();
  expect(api.quote).not.toHaveBeenCalled();
});
it("切换项目后迟到的原恢复回执不能覆盖当前任务或锁住按钮", async () => {
  let reply!: (value: API.WorkflowControlResult) => void;
  const other = "77777777-7777-4777-8777-777777777777";
  const original = {
    id,
    project_id: project.id,
    status: "manual",
    outputs: [],
    billing_outcome: {
      charge_state: "pending",
      delivery_state: "recoverable",
      regenerate: "blocked",
      reconciliation_required: true,
    },
  };
  api.get
    .mockResolvedValueOnce(original)
    .mockResolvedValue({ ...original, project_id: other });
  api.recover.mockImplementationOnce(
    () =>
      new Promise((resolve) => {
        reply = resolve;
      }),
  );
  const { rerender } = render(<TaskDetail projectID={project.id} id={id} />);
  fireEvent.click(
    await screen.findByRole("button", { name: "继续取回原任务" }),
  );
  await waitFor(() => expect(api.recover).toHaveBeenCalledOnce());
  rerender(<TaskDetail projectID={other} id={id} />);
  await waitFor(() =>
    expect(
      (
        screen.getByRole("button", {
          name: "继续取回原任务",
        }) as HTMLButtonElement
      ).disabled,
    ).toBe(false),
  );
  const calls = api.get.mock.calls.length;
  reply({
    target_id: id,
    target_type: "operation",
    action: "recover",
    accepted: true,
  });
  await new Promise((resolve) => setTimeout(resolve, 10));
  expect(api.get.mock.calls.length).toBe(calls);
  expect(api.quote).not.toHaveBeenCalled();
});

it.each([undefined, -1, Number.MAX_SAFE_INTEGER + 1])(
  "恢复报价金额%s不能展示时禁止新确认",
  async (amount) => {
    api.create.mockImplementation(async (_, body) => {
      const result = saved(body);
      result.draft = {
        ...result.draft,
        modelVersion: version,
        mode: "text_to_image",
      };
      current = {
        epoch: 1,
        submission: {
          id,
          state: "quoted",
          operation_id: id,
          confirm_key: "confirm-key",
          quote: { ...quote, quote_micros: amount },
          input_source: {
            kind: "prompt_draft",
            draft_id: id,
            revision: result.revision,
          },
          draft: {
            ...result.draft,
            mode: "text_to_image",
            modelVersion: version,
          },
        },
      };
      return result;
    });
    await open();
    const button = await screen.findByRole("button", {
      name: "确认生成",
    });
    expect((button as HTMLButtonElement).disabled).toBe(true);
    fireEvent.click(button);
    expect(api.confirm).not.toHaveBeenCalled();
  },
);
it.each([
  [0, "项目预算不足，请调整数量或联系管理员。"],
  [undefined, "当前报价暂不可确认，请检查项目预算或更新费用。"],
  [200_000, "当前报价暂不可确认，请检查项目预算或更新费用。"],
])(
  "不可确认报价的预算提示区分已知和未知余额：%s",
  async (available, message) => {
    api.create.mockImplementation(async (_, body) => {
      const result = saved(body);
      result.draft = {
        ...result.draft,
        modelVersion: version,
        mode: "text_to_image",
      };
      current = {
        epoch: 1,
        submission: {
          id,
          state: "quoted",
          operation_id: id,
          confirm_key: "confirm-key",
          quote: {
            ...quote,
            quote_micros: 120_000,
            available_micros: available,
            confirmable: false,
            quote_detail: { estimated_credits: 2 },
          },
          input_source: {
            kind: "prompt_draft",
            draft_id: id,
            revision: result.revision,
          },
          draft: {
            ...result.draft,
            mode: "text_to_image",
            modelVersion: version,
          },
        },
      };
      return result;
    });
    await open();
    const button = await screen.findByRole("button", { name: "确认生成" });
    expect(button.hasAttribute("disabled")).toBe(true);
    expect(screen.getByText(message)).toBeTruthy();
    if (available !== 0) expect(screen.queryByText(/^项目预算不足/)).toBeNull();
    expect(api.confirm).not.toHaveBeenCalled();
    expect(api.quote).not.toHaveBeenCalled();
  },
);

it("无法显示金额仍可沿原幂等键核对已发出的未知确认", async () => {
  api.confirm.mockImplementationOnce(async () => {
    quote.quote_micros = Number.MAX_SAFE_INTEGER + 1;
    throw new RequestError("request_unconfirmed");
  });
  await open();
  fireEvent.click(screen.getByRole("button", { name: "检查费用" }));
  fireEvent.click(await screen.findByRole("button", { name: "确认生成" }));
  const retry = await screen.findByRole("button", { name: "核对状态" });
  await waitFor(() =>
    expect((retry as HTMLButtonElement).disabled).toBe(false),
  );
  fireEvent.click(retry);
  await waitFor(() => expect(api.confirm).toHaveBeenCalledTimes(2));
  expect(api.confirm.mock.calls[0]).toEqual(api.confirm.mock.calls[1]);
});

it.each(["failed", "manual"])(
  "%s接受恢复后持续读取原任务直到完整交付，并停止轮询",
  async (status) => {
    const original = {
      id,
      project_id: project.id,
      status,
      capability: "text.generate",
      outputs: [],
      billing_outcome: {
        charge_state: "settled",
        charged_micros: 19,
        delivery_state: "recoverable",
        regenerate: "blocked",
        reconciliation_required: false,
      },
    };
    api.get
      .mockResolvedValueOnce(original)
      .mockResolvedValueOnce(original)
      .mockResolvedValue({
        ...original,
        outputs: [
          {
            id,
            kind: "json",
            moderation_status: "passed",
            text: "异步恢复原正文",
          },
        ],
        billing_outcome: {
          ...original.billing_outcome,
          delivery_state: "ready",
          regenerate: "confirm",
        },
      });
    api.recover.mockResolvedValue({
      target_id: id,
      target_type: "operation",
      action: "recover",
      accepted: true,
    });
    render(<TaskDetail projectID={project.id} id={id} />);
    fireEvent.click(
      await screen.findByRole("button", { name: "继续取回原任务" }),
    );
    await waitFor(() => expect(api.get).toHaveBeenCalledTimes(2));
    await screen.findByText("异步恢复原正文", {}, { timeout: 4500 });
    expect(api.get).toHaveBeenCalledTimes(3);
    await new Promise((resolve) => setTimeout(resolve, 3100));
    expect(api.get).toHaveBeenCalledTimes(3);
    expect(
      api.get.mock.calls.every(
        ([params]) => params.id === id && params.project_id === project.id,
      ),
    ).toBe(true);
    expect(api.quote).not.toHaveBeenCalled();
    expect(api.confirm).not.toHaveBeenCalled();
  },
  10000,
);
it("new explicit retry after acknowledged original query is still pending yields new recovery event", async () => {
  const state = {
    id,
    project_id: project.id,
    status: "manual",
    inputs: [],
    outputs: [],
    billing_outcome: {
      charge_state: "pending",
      delivery_state: "recoverable",
      regenerate: "blocked",
    },
  };
  api.get.mockResolvedValue(state);
  api.recover.mockResolvedValue({
    target_id: id,
    target_type: "operation",
    action: "recover",
    accepted: true,
  });
  render(<TaskDetail projectID={project.id} id={id} />);
  fireEvent.click(
    await screen.findByRole("button", { name: "继续取回原任务" }),
  );
  await waitFor(() => expect(api.get).toHaveBeenCalledTimes(2));
  fireEvent.click(screen.getByRole("button", { name: "继续取回原任务" }));
  await waitFor(() => expect(api.recover).toHaveBeenCalledTimes(2));
  expect(api.recover.mock.calls[0][2].headers["Idempotency-Key"]).not.toBe(
    api.recover.mock.calls[1][2].headers["Idempotency-Key"],
  );
});

it("报价确认只展示消耗和授权，技术快照默认折叠", async () => {
  quote.quote_detail = { estimated_credits: 8 };
  quote.final_prompt = "冻结的原提示词";
  quote.compiler_version = "compiler-v3";
  await compactWorkspace();
  const estimate = await screen.findByRole("button", {
    name: "预计消耗 2.4 积分",
  });
  expect(estimate.textContent).toBe("2.4");
  expect(estimate.querySelector("svg")).toBeTruthy();
  expect(screen.queryByText("计量 2 张")).toBeNull();
  expect(api.quote).not.toHaveBeenCalled();
  fireEvent.click(screen.getByRole("button", { name: "检查费用" }));
  await screen.findByRole("button", { name: "确认生成" });
  expect(screen.getByRole("dialog", { name: "确认生成" })).toBeTruthy();
  expect(screen.getByText("预计消耗 8 积分")).toBeTruthy();
  expect(screen.queryByRole("button", { name: "核对状态" })).toBeNull();
  expect(screen.queryByText("冻结的原提示词")).toBeNull();
  expect(screen.queryByText(/compiler-v3|提交 .*代次/)).toBeNull();
  fireEvent.click(screen.getByRole("button", { name: "查看生成详情" }));
  expect(screen.getByText("冻结的原提示词")).toBeTruthy();
  expect(screen.getByText(/compiler-v3/)).toBeTruthy();
  expect(api.confirm).not.toHaveBeenCalled();
});

it("首页取消报价后继续编辑，第二次检查费用仍打开确认弹窗", async () => {
  render(
    <RichGeneration
      projectID={project.id}
      presentation="home"
      seed={{ project, prompt: "清晨的海边", capability: "image" }}
    />,
  );
  const editor = await screen.findByRole("textbox", { name: "创作提示词" });
  await waitFor(() =>
    expect(
      screen
        .getByRole("combobox", { name: "生成模型" })
        .hasAttribute("disabled"),
    ).toBe(false),
  );
  fireEvent.keyDown(screen.getByRole("combobox", { name: "生成模型" }), {
    key: "ArrowDown",
  });
  fireEvent.click(await screen.findByRole("option", { name: /真实模型/ }));
  await waitFor(() =>
    expect(
      screen.getByRole("combobox", { name: "生成模型" }).textContent,
    ).toContain("真实模型"),
  );
  fireEvent.click(screen.getByRole("button", { name: "检查费用" }));
  await screen.findByRole("dialog", { name: "确认生成" });
  fireEvent.click(screen.getByRole("button", { name: "取消" }));
  await waitFor(() => {
    expect(screen.queryByRole("dialog", { name: "确认生成" })).toBeNull();
    expect(
      screen.getByRole("button", { name: "检查费用" }).hasAttribute("disabled"),
    ).toBe(false);
  });
  fireEvent.paste(editor, {
    clipboardData: {
      files: [],
      getData: (type: string) => (type === "text/plain" ? "白色陶瓷杯" : ""),
    },
  });
  fireEvent.click(screen.getByRole("button", { name: "检查费用" }));
  await screen.findByRole("dialog", { name: "确认生成" });
  expect(api.quote).toHaveBeenCalledTimes(2);
  expect(api.quote.mock.calls[1][1].expected_submission_epoch).toBe(2);
  expect(
    JSON.stringify(api.save.mock.calls.at(-1)?.[1].draft.document),
  ).toContain("白色陶瓷杯");
  expect(api.confirm).not.toHaveBeenCalled();
});

it("未知确认先核对已接受的原提交，不重复执行确认", async () => {
  api.confirm.mockRejectedValueOnce(new RequestError("request_unconfirmed"));
  await open();
  fireEvent.click(screen.getByRole("button", { name: "检查费用" }));
  fireEvent.click(await screen.findByRole("button", { name: "确认生成" }));
  const retry = await screen.findByRole("button", { name: "核对状态" });
  current = {
    ...current,
    submission: { ...current.submission, state: "accepted" },
  };
  fireEvent.click(retry);
  await screen.findByRole("heading", { name: "已完成" });
  expect(api.confirm).toHaveBeenCalledOnce();
  expect(api.quote).toHaveBeenCalledOnce();
});

it("未知确认查询失败时保留原请求，不新建报价或盲目重试确认", async () => {
  api.confirm.mockRejectedValueOnce(new RequestError("request_unconfirmed"));
  await open();
  fireEvent.click(screen.getByRole("button", { name: "检查费用" }));
  fireEvent.click(await screen.findByRole("button", { name: "确认生成" }));
  const retry = await screen.findByRole("button", { name: "核对状态" });
  api.current.mockRejectedValueOnce(new RequestError("request_unconfirmed"));
  fireEvent.click(retry);
  await waitFor(() => expect(retry.hasAttribute("disabled")).toBe(false));
  expect(api.confirm).toHaveBeenCalledOnce();
  expect(api.quote).toHaveBeenCalledOnce();
  expect(screen.queryByRole("button", { name: "取消" })).toBeNull();
  expect(screen.getByLabelText("生成模型").hasAttribute("disabled")).toBe(true);
  fireEvent.click(retry);
  await screen.findByRole("heading", { name: "已完成" });
  expect(api.confirm.mock.calls[0]).toEqual(api.confirm.mock.calls[1]);
});

it("报价操作冲突后提供状态恢复入口，读回已接受任务时不重新扣费", async () => {
  await open();
  fireEvent.click(screen.getByRole("button", { name: "检查费用" }));
  await screen.findByRole("button", { name: "确认生成" });
  api.resolve.mockRejectedValueOnce(
    new RequestError("submission_already_accepted"),
  );
  fireEvent.click(screen.getByRole("button", { name: "取消" }));
  const recover = await screen.findByRole("button", { name: "核对状态" });
  current = {
    ...current,
    submission: { ...current.submission, state: "accepted" },
  };
  fireEvent.click(recover);
  await screen.findByRole("heading", { name: "已完成" });
  expect(api.quote).toHaveBeenCalledOnce();
  expect(api.confirm).not.toHaveBeenCalled();
});

it.each([true, false])("确认面板展示服务端冻结系数：%s", async (provided) => {
  if (provided)
    quote.quote_detail = {
      multipliers: { mode_text2img: 1.5, resolution_2k: 1.25 },
    };
  await compactWorkspace();
  fireEvent.click(screen.getByRole("button", { name: "检查费用" }));
  await screen.findByRole("button", { name: "确认生成" });
  fireEvent.click(screen.getByRole("button", { name: "费用说明" }));
  if (provided) {
    expect(screen.getByText("模式系数（text2img）")).toBeTruthy();
    expect(screen.getByText("分辨率系数（2k）")).toBeTruthy();
    expect(screen.getByText("1.5")).toBeTruthy();
    expect(screen.getByText("1.25")).toBeTruthy();
  } else expect(screen.queryByText(/模式系数|分辨率系数/)).toBeNull();
  expect(api.confirm).not.toHaveBeenCalled();
});

it("展开保留导演工具与编辑器，参考筛选不改草稿，Escape恢复原入口", async () => {
  const { props, canvas, rerender } = await compactWorkspace(true);
  rerender(
    <RichGeneration
      {...props}
      canvasToolbar={
        <div role="toolbar" aria-label="导演工具箱">
          <button>配色</button>
        </div>
      }
    />,
  );
  const editor = screen.getByRole("textbox", { name: "创作提示词" });
  const expand = screen.getByRole("button", { name: "展开编辑器" });
  expand.focus();
  fireEvent.click(expand);
  const dialog = await screen.findByRole("dialog", {
    name: "多参考创作编辑器",
  });
  expect(
    dialog.contains(screen.getByRole("toolbar", { name: "导演工具箱" })),
  ).toBe(true);
  const before = JSON.stringify(canvas.onChange.mock.calls.at(-1)?.[0]);
  fireEvent.pointerDown(editor, { button: 0, pointerType: "mouse" });
  expect(screen.getByRole("dialog", { name: "多参考创作编辑器" })).toBe(dialog);
  fireEvent.click(screen.getByRole("radio", { name: "图片" }));
  expect(JSON.stringify(canvas.onChange.mock.calls.at(-1)?.[0])).toBe(before);
  expect(screen.getByRole("textbox", { name: "创作提示词" })).toBe(editor);
  fireEvent.keyDown(editor, { key: "Escape" });
  await waitFor(() =>
    expect(
      screen.queryByRole("dialog", { name: "多参考创作编辑器" }),
    ).toBeNull(),
  );
  expect(screen.getByRole("textbox", { name: "创作提示词" })).toBe(editor);
  expect(document.activeElement).toBe(expand);
});
