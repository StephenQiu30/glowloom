import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { UnifiedSubjectEditor } from "@/components/subjects/unified-editor";
import { RequestError } from "@/lib/request";
import type { UnifiedSubject } from "@/components/subjects/unified-contracts";
const api = vi.hoisted(() => ({
  create: vi.fn(),
  update: vi.fn(),
  get: vi.fn(),
  voiceOptions: vi.fn(),
  voiceCreate: vi.fn(),
  voiceGet: vi.fn(),
  voiceCommand: vi.fn(),
}));
vi.mock("@/api/subjects", () => ({
  createSubject: api.create,
  updateSubject: api.update,
  getSubject: api.get,
  listSubjectVoiceOptions: api.voiceOptions,
  createSubjectVoiceJob: api.voiceCreate,
  getSubjectVoiceJob: api.voiceGet,
  getSubjectVoiceCommand: api.voiceCommand,
}));
vi.mock("@/components/subjects/unified-media", () => ({
  UnifiedSubjectMedia: ({ label }: { label: string }) => <span>{label}</span>,
}));
vi.mock("@/components/subjects/unified-source-upload", () => ({
  UnifiedSubjectSourceUpload: ({
    initialFile,
    allowedKinds,
  }: {
    initialFile?: File;
    allowedKinds: string[];
  }) => (
    <div role="status">
      上传来源 {initialFile?.name} {allowedKinds.join("/")}
    </div>
  ),
}));
vi.mock("@/components/subjects/unified-source-picker", () => ({
  UnifiedSubjectSourcePicker: ({
    onSelect,
  }: {
    onSelect: (media: unknown) => void;
  }) => (
    <button
      onClick={() =>
        onSelect({
          asset_id: "66666666-6666-4666-8666-666666666666",
          asset_revision: 1,
          kind: "image",
          mime_type: "image/png",
          file_name: "参考.png",
        })
      }
    >
      采用测试资产
    </button>
  ),
}));
const actorID = "11111111-1111-4111-8111-111111111111",
  scopeID = "22222222-2222-4222-8222-222222222222";
const initial: UnifiedSubject = {
  id: "33333333-3333-4333-8333-333333333333",
  owner_scope_id: scopeID,
  revision: 1,
  version_id: "44444444-4444-4444-8444-444444444444",
  name: "海边香氛",
  description: "金色玻璃瓶",
  primary: {
    binding_id: "55555555-5555-4555-8555-555555555555",
    asset_id: "66666666-6666-4666-8666-666666666666",
    asset_revision: 1,
    kind: "image",
    mime_type: "image/png",
    file_name: "主参考.png",
  },
  references: [],
  favorite: false,
  created_at: "2026-10-28T00:00:00Z",
  updated_at: "2026-10-28T00:00:00Z",
};
beforeEach(() => {
  vi.clearAllMocks();
  api.voiceOptions.mockResolvedValue({
    current_actor_id: actorID,
    current_scope_id: scopeID,
    available: false,
    reason: "voice_generation_unavailable",
    models: [],
  });
});
afterEach(cleanup);
it("未知保存结果冻结原请求，重试同键并回读，不重复创建", async () => {
  const saved = { ...initial, revision: 2, name: "蓝色香氛" };
  api.update
    .mockRejectedValueOnce(new Error("network"))
    .mockResolvedValueOnce(saved);
  api.get.mockResolvedValue(saved);
  const onSaved = vi.fn();
  render(
    <UnifiedSubjectEditor
      actorID={actorID}
      scopeID={scopeID}
      initial={initial}
      onClose={vi.fn()}
      onSaved={onSaved}
    />,
  );
  fireEvent.change(screen.getByLabelText("名称"), {
    target: { value: "蓝色香氛" },
  });
  fireEvent.click(screen.getByRole("button", { name: "保存" }));
  const retry = await screen.findByRole("button", { name: "重试原保存" });
  expect(screen.getByLabelText("名称")).toHaveProperty("disabled", true);
  fireEvent.click(retry);
  await waitFor(() => expect(onSaved).toHaveBeenCalledWith(saved));
  expect(api.update.mock.calls[0][1]).toEqual(api.update.mock.calls[1][1]);
  expect(api.update.mock.calls[0][2].headers["Idempotency-Key"]).toBe(
    api.update.mock.calls[1][2].headers["Idempotency-Key"],
  );
  expect(api.create).not.toHaveBeenCalled();
});

it("同revision和version的回读事实不一致不能报告保存成功", async () => {
  const saved = { ...initial, revision: 2, name: "新名称" };
  api.update.mockResolvedValue(saved);
  api.get.mockResolvedValue({
    ...saved,
    primary: {
      ...saved.primary,
      binding_id: "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa",
    },
  });
  const done = vi.fn();
  render(
    <UnifiedSubjectEditor
      actorID={actorID}
      scopeID={scopeID}
      initial={initial}
      onClose={vi.fn()}
      onSaved={done}
    />,
  );
  fireEvent.change(screen.getByLabelText("名称"), {
    target: { value: "新名称" },
  });
  fireEvent.click(screen.getByRole("button", { name: "保存" }));
  await screen.findByRole("button", { name: "重试原保存" });
  expect(done).not.toHaveBeenCalled();
  expect(screen.getByRole("alert").textContent).toContain("结果尚未确认");
});
it("并发冲突保留输入，核对最新revision后再原子保存", async () => {
  api.update.mockRejectedValueOnce(new RequestError("revision_conflict", 409));
  api.get.mockResolvedValue({ ...initial, revision: 3 });
  render(
    <UnifiedSubjectEditor
      actorID={actorID}
      scopeID={scopeID}
      initial={initial}
      onClose={vi.fn()}
      onSaved={vi.fn()}
    />,
  );
  fireEvent.change(screen.getByLabelText("名称"), {
    target: { value: "保留的新名称" },
  });
  fireEvent.click(screen.getByRole("button", { name: "保存" }));
  fireEvent.click(
    await screen.findByRole("button", { name: "读取最新版本并保留输入" }),
  );
  await waitFor(() =>
    expect(screen.getByRole("button", { name: "保存" })).toHaveProperty(
      "disabled",
      false,
    ),
  );
  expect(screen.getByLabelText("名称")).toHaveProperty("value", "保留的新名称");
  api.update.mockResolvedValue({
    ...initial,
    revision: 4,
    name: "保留的新名称",
  });
  api.get.mockResolvedValue({ ...initial, revision: 4, name: "保留的新名称" });
  fireEvent.click(screen.getByRole("button", { name: "保存" }));
  await waitFor(() => expect(api.update).toHaveBeenCalledTimes(2));
  expect(api.update.mock.calls[1][1].expected_revision).toBe(3);
});
it("主参考移除后保存不可用，名称20计数无独立详情步骤", () => {
  render(
    <UnifiedSubjectEditor
      actorID={actorID}
      scopeID={scopeID}
      initial={initial}
      onClose={vi.fn()}
      onSaved={vi.fn()}
    />,
  );
  expect(screen.getByRole("heading", { name: "设置主体" })).toBeTruthy();
  expect(screen.getByLabelText("名称").getAttribute("maxlength")).toBe("20");
  fireEvent.click(screen.getByRole("button", { name: "移除主参考" }));
  expect(screen.getByRole("button", { name: "保存" })).toHaveProperty(
    "disabled",
    true,
  );
});

it.each([
  [" ", true],
  ["主".repeat(20), false],
  ["主".repeat(21), true],
  ["😀".repeat(10), false],
  ["😀".repeat(11), true],
])("名称预算按浏览器UTF-16合同判断：%s", (name, disabled) => {
  render(
    <UnifiedSubjectEditor
      actorID={actorID}
      scopeID={scopeID}
      initial={initial}
      onClose={vi.fn()}
      onSaved={vi.fn()}
    />,
  );
  fireEvent.change(screen.getByLabelText("名称"), { target: { value: name } });
  expect(screen.getByRole("button", { name: "保存" })).toHaveProperty(
    "disabled",
    disabled,
  );
});

it("不选择项目或类别就以完整设置原子创建，成功后回读", async () => {
  const created = { ...initial, description: "" };
  api.create.mockResolvedValue(created);
  api.get.mockResolvedValue(created);
  const onSaved = vi.fn();
  render(
    <UnifiedSubjectEditor
      actorID={actorID}
      scopeID={scopeID}
      onClose={vi.fn()}
      onSaved={onSaved}
    />,
  );
  fireEvent.change(screen.getByLabelText("名称"), {
    target: { value: "海边香氛" },
  });
  expect(screen.getByRole("button", { name: "保存" })).toHaveProperty(
    "disabled",
    true,
  );
  fireEvent.click(screen.getByRole("button", { name: "从资产添加" }));
  fireEvent.click(
    await screen.findByRole("button", { name: "采用测试资产", hidden: true }),
  );
  fireEvent.click(screen.getByRole("button", { name: "保存" }));
  await waitFor(() => expect(onSaved).toHaveBeenCalledWith(created));
  expect(api.create.mock.calls[0][0]).toEqual({
    expected_revision: 0,
    name: "海边香氛",
    description: "",
    primary: { asset_id: initial.primary.asset_id },
    references: [],
  });
  expect(api.get).toHaveBeenCalledWith({ id: initial.id }, expect.any(Object));
  expect(api.update).not.toHaveBeenCalled();
});

it("主参考本地拖入进入独立上传流程，上传结束前保存禁用", async () => {
  const view = render(
    <UnifiedSubjectEditor
      actorID={actorID}
      scopeID={scopeID}
      initial={initial}
      onClose={vi.fn()}
      onSaved={vi.fn()}
    />,
  );
  const file = new File(["image"], "本地参考.png", { type: "image/png" });
  fireEvent.drop(view.baseElement.querySelector(".unified-subject-main")!, {
    dataTransfer: { files: [file] },
  });
  expect(
    await screen.findByText("上传来源 本地参考.png image/video"),
  ).toBeTruthy();
  expect(screen.getByRole("button", { name: "保存" })).toHaveProperty(
    "disabled",
    true,
  );
});

it("添加音色菜单读取真实配置，不可用时说明接入原因", async () => {
  render(
    <UnifiedSubjectEditor
      actorID={actorID}
      scopeID={scopeID}
      initial={initial}
      onClose={vi.fn()}
      onSaved={vi.fn()}
    />,
  );
  fireEvent.pointerDown(screen.getByRole("button", { name: "添加音色" }), {
    button: 0,
    ctrlKey: false,
  });
  expect(
    await screen.findByRole("menuitem", { name: "上传音色" }),
  ).toBeTruthy();
  expect(screen.queryByRole("menuitem", { name: "从资产添加" })).toBeNull();
  fireEvent.click(screen.getByRole("menuitem", { name: "智能配音" }));
  expect(
    await screen.findByText("请先接入火山引擎豆包语音模型并配置可用音色。"),
  ).toBeTruthy();
  expect(api.voiceOptions).toHaveBeenCalledOnce();
  expect(api.voiceCreate).not.toHaveBeenCalled();
  expect(api.create).not.toHaveBeenCalled();
  expect(api.update).not.toHaveBeenCalled();
});

it("智能配音中保留原音色与表单并禁保存，正式音频成功后才替换且随主体原子保存", async () => {
  const release = "77777777-7777-4777-8777-777777777777",
    model = "88888888-8888-4888-8888-888888888888",
    asset = "99999999-9999-4999-8999-999999999999";
  api.voiceOptions.mockResolvedValue({
    current_actor_id: actorID,
    current_scope_id: scopeID,
    available: true,
    models: [
      {
        release_id: release,
        model_version_id: model,
        model_key: "doubao",
        display_name: "豆包",
        voices: [{ key: "speaker", name: "默认音色" }],
        default_voice_key: "speaker",
      },
    ],
  });
  let resolve!: (value: unknown) => void;
  api.voiceCreate.mockImplementation(
    () =>
      new Promise((r) => {
        resolve = r;
      }),
  );
  const oldVoice = {
    name: "原音色",
    media: {
      ...initial.primary,
      asset_id: "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa",
      binding_id: "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb",
      kind: "audio" as const,
      mime_type: "audio/mpeg",
      file_name: "原音色.mp3",
    },
  };
  render(
    <UnifiedSubjectEditor
      actorID={actorID}
      scopeID={scopeID}
      initial={{ ...initial, voice: oldVoice }}
      onClose={vi.fn()}
      onSaved={vi.fn()}
    />,
  );
  fireEvent.change(screen.getByLabelText("名称"), {
    target: { value: "保留的主体名称" },
  });
  fireEvent.pointerDown(screen.getByRole("button", { name: "添加音色" }), {
    button: 0,
    ctrlKey: false,
  });
  fireEvent.click(await screen.findByRole("menuitem", { name: "智能配音" }));
  await waitFor(() => expect(api.voiceCreate).toHaveBeenCalledOnce());
  expect(screen.getByText("原音色")).toBeTruthy();
  expect(screen.queryByRole("button", { name: "移除音色" })).toBeNull();
  expect(screen.getByRole("button", { name: "保存" })).toHaveProperty(
    "disabled",
    true,
  );
  expect(screen.getByRole("button", { name: "移除主参考" })).toHaveProperty(
    "disabled",
    true,
  );
  expect(screen.getByLabelText("名称")).toHaveProperty(
    "value",
    "保留的主体名称",
  );
  expect(screen.getByText("关闭设置不会取消已提交的配音任务。")).toBeTruthy();
  const body = api.voiceCreate.mock.calls[0][0];
  resolve({
    ...body,
    id: "cccccccc-cccc-4ccc-8ccc-cccccccccccc",
    current_actor_id: actorID,
    current_scope_id: scopeID,
    model_version_id: model,
    revision: 1,
    status: "succeeded",
    created_at: initial.created_at,
    updated_at: initial.updated_at,
    result: {
      asset_id: asset,
      asset_revision: 1,
      kind: "audio",
      mime_type: "audio/mpeg",
      file_name: "新配音.mp3",
      library_item_id: asset,
      library_source: "subject",
    },
  });
  await screen.findByText("智能配音1");
  expect(screen.queryByText("原音色")).toBeNull();
  expect(screen.getByRole("button", { name: "移除音色" })).toHaveProperty(
    "disabled",
    false,
  );
  const saved = {
    ...initial,
    revision: 2,
    name: "保留的主体名称",
    voice: { name: "智能配音1", media: { ...oldVoice.media, asset_id: asset } },
  };
  api.update.mockResolvedValue(saved);
  api.get.mockResolvedValue(saved);
  fireEvent.click(screen.getByRole("button", { name: "保存" }));
  await waitFor(() => expect(api.update).toHaveBeenCalledOnce());
  expect(api.update.mock.calls[0][1].voice).toEqual({
    name: "智能配音1",
    asset_id: asset,
  });
});
