import { render, findFeedback } from "../../../feedback";
import { cleanup, fireEvent, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { GenerationDialog } from "@/components/generation/generation-dialog";
import { RequestError } from "@/lib/request";
import { RichGeneration } from "@/components/generation/rich-generation";
import { emptyPromptDraft } from "@/components/generation/prompt-document";
import { mediaBinding } from "@/components/generation/reference-controls";
import type { CanvasReferenceSource } from "@/components/generation/reference-source";

const api = vi.hoisted(() => ({
  models: vi.fn(),
  quote: vi.fn(),
  confirm: vi.fn(),
  get: vi.fn(),
  list: vi.fn(),
  cancel: vi.fn(),
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
  confirmOperationQuote: api.confirm,
  getOperation: api.get,
  listProjectOperations: api.list,
  cancelOperation: api.cancel,
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

it("画布紧凑呈现保持真正富文本内联，先保存新节点且重渲染不重新初始化", async () => {
  const { props, canvas, rerender } = await compactWorkspace();
  const editor = screen.getByRole("textbox", { name: "创作提示词" });
  expect(editor.getAttribute("contenteditable")).toBe("true");
  expect(editor.getAttribute("data-placeholder")).toBe(
    "上传参考图、输入文字或 @ 主体，描述你想生成的图片",
  );
  expect(editor.closest('[data-slot="input-group-control"]')).toBeTruthy();
  expect(
    screen.getByRole("button", { name: "生成模型" }).textContent,
  ).toContain("真实模型");
  expect(
    screen.getByRole("combobox", { name: "生成模式" }).textContent,
  ).toContain("文生图");
  expect(screen.queryByText("全能参考")).toBeNull();
  expect(canvas.flush).toHaveBeenCalledOnce();
  expect(canvas.flush.mock.invocationCallOrder[0]).toBeLessThan(
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
  expect(canvas.flush).toHaveBeenCalledOnce();
});

it("紧凑参数使用模型schema与候选上限，同一画布来源报价后才允许确认", async () => {
  const { canvas, pointer } = await compactWorkspace();
  fireEvent.click(screen.getByRole("button", { name: "生成参数" }));
  expect(screen.getByLabelText("画幅")).toBeTruthy();
  const count = screen.getByLabelText("候选数量");
  expect(count.getAttribute("max")).toBe("4");
  fireEvent.change(count, { target: { value: "2" } });
  await waitFor(() =>
    expect(canvas.onChange).toHaveBeenLastCalledWith(
      expect.objectContaining({
        params: expect.objectContaining({ output_count: 2 }),
      }),
    ),
  );
  fireEvent.keyDown(document, { key: "Escape" });
  fireEvent.click(screen.getByRole("button", { name: "检查费用" }));
  await screen.findByRole("button", { name: "确认费用并生成" });
  expect(api.quote.mock.calls[0][1].input_source).toEqual(pointer);
  expect(api.confirm).not.toHaveBeenCalled();
  fireEvent.click(screen.getByRole("button", { name: "确认费用并生成" }));
  await waitFor(() => expect(api.confirm).toHaveBeenCalledOnce());
});

it("全能参考模式来自真实目录合同，切换不重建正文或触发生成", async () => {
  const models = await api.models();
  models.items[0].current_version.modes.push("omni_reference");
  api.models.mockResolvedValue(models);
  const { canvas } = await compactWorkspace();
  const editor = screen.getByRole("textbox", { name: "创作提示词" });
  fireEvent.keyDown(screen.getByRole("combobox", { name: "生成模式" }), {
    key: "ArrowDown",
  });
  fireEvent.click(await screen.findByRole("option", { name: "全能参考" }));
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
  fireEvent.click(await screen.findByRole("option", { name: "图片" }));
  expect(screen.queryByRole("option", { name: /未连接海报/ })).toBeNull();
  fireEvent.click(screen.getByRole("option", { name: /已连接海报/ }));
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
  fireEvent.click(screen.getByRole("option", { name: /待核验海报/ }));
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
  expect(
    screen
      .getByRole("button", { name: "检查费用", hidden: true })
      .hasAttribute("disabled"),
  ).toBe(true);
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
  api.models.mockRejectedValueOnce(
    new RequestError("dependency_unavailable", 503),
  );
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
  expect(await screen.findByText("模型目录暂不可用")).toBeTruthy();
  expect(
    screen.getByRole("button", { name: "检查费用" }).hasAttribute("disabled"),
  ).toBe(true);
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
  await waitFor(() => expect(canvas.flush).toHaveBeenCalledTimes(2));
  const edited = canvas.onChange.mock.calls.at(-1)?.[0];
  const identityCalls = api.identity.mock.calls.length;
  const submissionCalls = api.current.mock.calls.length;
  fireEvent.click(screen.getByRole("button", { name: "重试模型目录" }));
  await waitFor(() =>
    expect(screen.queryByText("模型目录暂不可用")).toBeNull(),
  );
  expect(
    screen.getByRole("button", { name: "生成模型" }).textContent,
  ).toContain("真实模型");
  expect(screen.getByRole("textbox", { name: "创作提示词" })).toBe(editor);
  expect(canvas.onChange.mock.calls.at(-1)?.[0]).toEqual(edited);
  expect(edited.references[0]).toEqual(binding);
  expect(api.identity).toHaveBeenCalledTimes(identityCalls);
  expect(api.current).toHaveBeenCalledTimes(submissionCalls);
  expect(canvas.flush).toHaveBeenCalledTimes(2);
  expect(api.quote).not.toHaveBeenCalled();
});

it("未确认的报价重试复用保存后的草稿指针和幂等键，确认前不提交生成", async () => {
  api.quote.mockRejectedValueOnce(new RequestError("request_unconfirmed"));
  await open();
  fireEvent.click(screen.getByRole("button", { name: "检查费用" }));
  await findFeedback();
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

it("模式查询发现当前模型停用后保留输入并禁用报价", async () => {
  const page = await api.models();
  page.items[0].current_version.modes = ["text_to_image", "image_to_image"];
  await open();
  api.models.mockResolvedValue({
    items: [{ ...page.items[0], status: "disabled" }],
  });
  fireEvent.change(screen.getByLabelText("生成模式"), {
    target: { value: "image_to_image" },
  });
  await screen.findByText("该模型当前不可用");
  expect(
    screen.getByRole("button", { name: "检查费用" }).hasAttribute("disabled"),
  ).toBe(true);
  expect((screen.getByLabelText("生成模式") as HTMLSelectElement).value).toBe(
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
  const confirm = await screen.findByRole("button", { name: "确认费用并生成" });
  expect(confirm.hasAttribute("disabled")).toBe(true);
  expect(api.confirm).not.toHaveBeenCalled();
});

it("画布共享富文本不读写本地恢复分支", async () => {
  await compactWorkspace();
  expect(api.branchList).not.toHaveBeenCalled();
  expect(api.branchWrite).not.toHaveBeenCalled();
  expect(screen.queryByText(/恢复.*分支/)).toBeNull();
});
