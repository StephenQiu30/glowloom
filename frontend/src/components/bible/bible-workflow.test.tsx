import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { ApiError } from "@/lib/request";
import { BibleActionDialog } from "./bible-action-dialog";
import { BibleLookDialog } from "./bible-look-dialog";
import { BibleVoiceDialog, declaredVoiceParams } from "./bible-voice-dialog";
import { BibleReferencePicker } from "./bible-reference-picker";
import { BibleHistoryDialog } from "./bible-history";
import { BibleResultDialog } from "./bible-result-dialog";
import * as query from "./bible-queries";
import type { BibleDetail, BibleIdentity, BibleVersion } from "./bible-model";
import type { BibleWriter } from "./use-bible-writer";
import type { Model } from "@/components/catalog/queries";

vi.mock("./bible-queries", async (original) => ({
  ...(await original<typeof import("./bible-queries")>()),
  listBible: vi.fn(),
  getBibleDetail: vi.fn(),
  getBibleMediaCandidates: vi.fn(),
  getBibleFormalEpisodes: vi.fn(),
  getBibleFormalScenes: vi.fn(),
  getBibleVoices: vi.fn(),
  getBibleVoiceModel: vi.fn(),
  getBibleHistory: vi.fn(),
  getBibleSnapshot: vi.fn(),
  getBibleResults: vi.fn(),
  getBibleResult: vi.fn(),
}));
const identity: BibleIdentity = {
  origin: window.location.origin,
  actorId: "11111111-1111-4111-8111-111111111111",
  orgId: "22222222-2222-4222-8222-222222222222",
  projectId: "33333333-3333-4333-8333-333333333333",
};
const entryId = "44444444-4444-4444-8444-444444444444",
  versionId = "55555555-5555-4555-8555-555555555555",
  lookId = "66666666-6666-4666-8666-666666666666",
  targetId = "77777777-7777-4777-8777-777777777777",
  mediaId = "88888888-8888-4888-8888-888888888888",
  sha = "a".repeat(64),
  at = "2026-10-02T00:00:00Z";
function detail(id = entryId, revision = 2): BibleDetail {
  const current: BibleVersion = {
    id: versionId,
    entry_id: id,
    org_id: identity.orgId,
    project_id: identity.projectId,
    kind: "character",
    number: revision,
    actor_id: identity.actorId,
    created_at: at,
    origin: "manual",
    content_sha256: sha,
    character: {
      name: "角色原文😀",
      definition: { role: "完整剧情定位" },
      looks: [{ id: lookId, name: "默认原造型", default: true }],
    },
  };
  return {
    head: {
      id,
      org_id: identity.orgId,
      project_id: identity.projectId,
      kind: "character",
      revision,
      current_version_id: versionId,
      confirmed_version_id: versionId,
      deleted: false,
      created_at: at,
      updated_at: at,
    },
    current,
    resolved_id: id,
  };
}
function writer(overrides: Partial<BibleWriter> = {}): BibleWriter {
  return {
    ready: true,
    busy: false,
    intent: null,
    rejected: null,
    latest: null,
    error: undefined,
    storageError: undefined,
    locked: false,
    submit: vi.fn(),
    replay: vi.fn(),
    restoreStorage: vi.fn(),
    readLatest: vi.fn(),
    discardRejected: vi.fn().mockResolvedValue(true),
    ...overrides,
  };
}
function mount(children: React.ReactNode) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return render(
    <QueryClientProvider client={client}>{children}</QueryClientProvider>,
  );
}
const voiceModel: Model = {
  id: mediaId,
  key: "actual.tts",
  display_name: "实际模型",
  capability: "audio.tts",
  status: "active",
  provider_id: targetId,
  provider_name: "实际提供方",
  region: "local",
  provider_status: "active",
  credential_present: true,
  input_roles: [],
  credential_test_result: null,
  current_price: null,
  current_version: {
    id: versionId,
    version_no: 3,
    modes: ["tts"],
    limits: {},
    param_schema: [
      {
        field: "voice_id",
        label: "声音",
        type: "string",
        component: "voice",
        enum: ["actual.voice"],
      },
      {
        field: "speed",
        label: "正式语速",
        type: "number",
        component: "slider",
        min: 0.5,
        max: 2,
        step: 0.25,
        default: 1,
      },
      {
        field: "language",
        label: "正式语言",
        type: "string",
        component: "select",
        enum: ["zh", "en"],
        default: "zh",
      },
    ],
  },
};
beforeEach(() => {
  vi.resetAllMocks();
  vi.stubGlobal(
    "ResizeObserver",
    class {
      observe() {}
      unobserve() {}
      disconnect() {}
    },
  );
  vi.mocked(query.listBible).mockResolvedValue({
    current_actor_id: identity.actorId,
    current_org_id: identity.orgId,
    entries: [
      { head: detail(targetId, 3).head, name: "目标角色", content_sha256: sha },
    ],
  });
  vi.mocked(query.getBibleDetail).mockResolvedValue(detail(targetId, 3));
  vi.mocked(query.getBibleMediaCandidates).mockResolvedValue({
    next_cursor: null,
    items: [
      {
        id: mediaId,
        project_id: identity.projectId,
        kind: "image",
        file_name: "真实合成图.png",
        mime_type: "image/png",
        byte_size: 100,
        revision: 1,
      },
    ],
  });
  vi.mocked(query.getBibleFormalEpisodes).mockResolvedValue([]);
  vi.mocked(query.getBibleFormalScenes).mockResolvedValue([]);
  vi.mocked(query.getBibleVoices).mockResolvedValue({
    voices: [
      {
        model_key: "actual.tts",
        model_version: 3,
        voice_key: "actual.voice",
        display_name: "实际声音",
      },
    ],
  });
  vi.mocked(query.getBibleVoiceModel).mockResolvedValue(voiceModel);
});
afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

it("确认当前完整版本须人工审核，CAS与原身份均冻结且未知时不得再提交", async () => {
  const controller = writer();
  mount(
    <BibleActionDialog
      identity={identity}
      frame={{ action: "confirm", detail: detail() }}
      writer={controller}
      onClose={vi.fn()}
    />,
  );
  const button = screen.getByRole("button", { name: "确认当前设定" });
  expect(button).toHaveProperty("disabled", true);
  fireEvent.click(screen.getByRole("checkbox", { name: /我已审核/ }));
  fireEvent.click(button);
  expect(controller.submit).toHaveBeenCalledExactlyOnceWith({
    action: "confirm",
    kind: "character",
    id: entryId,
    body: { expected_revision: 2 },
  });
});

it("新读取已回收或重定向身份不允许普通确认/合并，恢复只能以原稳定身份明确操作", () => {
  const current = detail();
  current.head.deleted = true;
  const controller = writer();
  mount(
    <BibleActionDialog
      identity={identity}
      frame={{ action: "confirm", detail: current }}
      writer={controller}
      onClose={vi.fn()}
    />,
  );
  expect(screen.getByRole("checkbox", { name: /我已审核/ })).toHaveProperty(
    "disabled",
    true,
  );
  expect(screen.getByRole("button", { name: "确认当前设定" })).toHaveProperty(
    "disabled",
    true,
  );
  expect(controller.submit).not.toHaveBeenCalled();
});

it("合并409读取最新两边后明确新意图使用新CAS并重新审核，不沿旧目标版本", async () => {
  const controller = writer(),
    frame = { action: "merge" as const, detail: detail() };
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  const view = render(
    <QueryClientProvider client={client}>
      <BibleActionDialog
        identity={identity}
        frame={frame}
        writer={controller}
        onClose={vi.fn()}
      />
    </QueryClientProvider>,
  );
  await screen.findByRole("option", { name: /目标角色/ });
  fireEvent.change(screen.getByRole("combobox", { name: "合并到已确认角色" }), {
    target: { value: targetId },
  });
  await waitFor(() => expect(screen.getByText(/目标当前版本 3/)).toBeTruthy());
  const intent = {
    ...identity,
    version: 1 as const,
    key: mediaId,
    command: {
      action: "merge" as const,
      kind: "character" as const,
      id: entryId,
      body: {
        expected_revision: 2,
        target_id: targetId,
        expected_target_revision: 3,
      },
    },
  };
  view.rerender(
    <QueryClientProvider client={client}>
      <BibleActionDialog
        identity={identity}
        frame={frame}
        writer={writer({
          locked: true,
          intent,
          rejected: { intent, cause: new ApiError(409, "conflict") },
          latest: {
            scope: {
              current_actor_id: identity.actorId,
              current_org_id: identity.orgId,
              entries: [],
            },
            detail: detail(entryId, 6),
            target: detail(targetId, 8),
          },
          discardRejected: controller.discardRejected,
        })}
        onClose={vi.fn()}
      />
    </QueryClientProvider>,
  );
  fireEvent.click(
    screen.getByRole("button", { name: "明确保留草稿并准备新的修改意图" }),
  );
  await waitFor(() =>
    expect(screen.getByText(/当前版本 6 · 不变版本/)).toBeTruthy(),
  );
  view.rerender(
    <QueryClientProvider client={client}>
      <BibleActionDialog
        identity={identity}
        frame={frame}
        writer={controller}
        onClose={vi.fn()}
      />
    </QueryClientProvider>,
  );
  expect(screen.getByRole("button", { name: "合并角色身份" })).toHaveProperty(
    "disabled",
    true,
  );
  fireEvent.click(screen.getByRole("checkbox", { name: /我已审核/ }));
  fireEvent.click(screen.getByRole("button", { name: "合并角色身份" }));
  expect(controller.submit).toHaveBeenCalledWith({
    action: "merge",
    kind: "character",
    id: entryId,
    body: {
      expected_revision: 6,
      target_id: targetId,
      expected_target_revision: 8,
    },
  });
});

it("声音参数仅按实际安全schema显式输入，保留零值且拒越界/失效声明", () => {
  expect(declaredVoiceParams(voiceModel, {})).toEqual({});
  expect(
    declaredVoiceParams(voiceModel, { speed: "0.5", language: "zh" }),
  ).toEqual({ speed: 0.5, language: "zh" });
  const bad: Record<string, string>[] = [
    { speed: "0.6" },
    { speed: "Infinity" },
    { language: "fake" },
    { pitch: "0" },
  ];
  for (const entries of bad)
    expect(() => declaredVoiceParams(voiceModel, entries)).toThrow();
});

it("声音参数需存在完整兼容模式，不能组合两个互斥模式或遗漏无默认必填参数", () => {
  const model: Model = {
    ...voiceModel,
    current_version: {
      ...voiceModel.current_version!,
      modes: ["tts", "dialogue"],
      param_schema: [
        voiceModel.current_version!.param_schema[0],
        {
          field: "speed",
          label: "语速",
          type: "number",
          component: "input",
          for_modes: ["tts"],
        },
        {
          field: "emotion",
          label: "情绪",
          type: "string",
          component: "input",
          for_modes: ["dialogue"],
          required: true,
        },
      ],
    },
  };
  expect(declaredVoiceParams(model, { speed: "0" })).toEqual({ speed: 0 });
  expect(() =>
    declaredVoiceParams(model, { speed: "0", emotion: "平静" }),
  ).toThrow();
  expect(() =>
    declaredVoiceParams(
      {
        ...model,
        current_version: { ...model.current_version!, modes: ["dialogue"] },
      },
      {},
    ),
  ).toThrow();
});

it("正式声音列表没有静态种子，选定安全版本后也不会隐式提供模型默认值", async () => {
  const controller = writer();
  mount(
    <BibleVoiceDialog
      identity={identity}
      id={entryId}
      revision={2}
      writer={controller}
      onClose={vi.fn()}
    />,
  );
  fireEvent.change(screen.getByRole("combobox", { name: "声音来源" }), {
    target: { value: "catalog" },
  });
  const choice = JSON.stringify(["actual.tts", 3, "actual.voice"]);
  await screen.findByRole("option", { name: "实际声音 · actual.tts · v3" });
  fireEvent.change(screen.getByRole("combobox", { name: "当前正式声音" }), {
    target: { value: choice },
  });
  await screen.findByRole("spinbutton", { name: "正式语速" });
  fireEvent.click(screen.getByRole("button", { name: "明确保存声音绑定" }));
  expect(controller.submit).toHaveBeenCalledWith({
    action: "voice_bind",
    kind: "character",
    id: entryId,
    body: {
      expected_revision: 2,
      voice: {
        kind: "catalog",
        catalog: {
          model_key: "actual.tts",
          expected_model_version: 3,
          voice_key: "actual.voice",
          params: {},
        },
      },
    },
  });
});

it("造型保存保留精确正式分集与场景UUID，读取失败不把原范围悄悄清空", async () => {
  vi.mocked(query.getBibleFormalEpisodes).mockRejectedValue(
    new ApiError(403, "forbidden"),
  );
  const controller = writer(),
    look = {
      id: lookId,
      name: " 原造型😀\n",
      default: true,
      applies_to: [{ episode_id: targetId, scene_key: mediaId }],
    };
  mount(
    <BibleLookDialog
      identity={identity}
      frame={{ action: "look_update", id: entryId, revision: 2, look }}
      writer={controller}
      onClose={vi.fn()}
    />,
  );
  await screen.findAllByRole("alert");
  expect(
    screen.getByRole("combobox", { name: "适用范围1正式分集" }),
  ).toHaveProperty("value", targetId);
  expect(
    screen.getByRole("combobox", { name: "适用范围1正式场景" }),
  ).toHaveProperty("value", mediaId);
  fireEvent.click(screen.getByRole("button", { name: "保存完整造型" }));
  await waitFor(() =>
    expect(controller.submit).toHaveBeenCalledWith({
      action: "look_update",
      kind: "character",
      id: entryId,
      lookId,
      body: {
        expected_revision: 2,
        look: { name: look.name, default: true, applies_to: look.applies_to },
      },
    }),
  );
});

it("大量正式范围按25行读取与交互，分页不会截断原完整造型提交", async () => {
  const controller = writer(),
    applies_to = Array.from({ length: 26 }, (_, index) => ({
      episode_id: `90000000-0000-4000-8000-${String(index + 1).padStart(12, "0")}`,
    }));
  mount(
    <BibleLookDialog
      identity={identity}
      frame={{
        action: "look_update",
        id: entryId,
        revision: 2,
        look: { id: lookId, name: "完整多集造型", default: true, applies_to },
      }}
      writer={controller}
      onClose={vi.fn()}
    />,
  );
  expect(screen.getAllByRole("combobox")).toHaveLength(50);
  fireEvent.click(screen.getByRole("button", { name: "下一页适用范围" }));
  expect(screen.getAllByRole("combobox")).toHaveLength(2);
  fireEvent.click(screen.getByRole("button", { name: "保存完整造型" }));
  await waitFor(() =>
    expect(controller.submit).toHaveBeenCalledWith({
      action: "look_update",
      kind: "character",
      id: entryId,
      lookId,
      body: {
        expected_revision: 2,
        look: { name: "完整多集造型", default: true, applies_to },
      },
    }),
  );
});

it("六图选择仅提交原assetUUID与原用途，unknown锁定且不主动获取签名预览", async () => {
  const save = vi.fn();
  mount(
    <BibleReferencePicker
      identity={identity}
      initial={[]}
      locked={false}
      onDirty={vi.fn()}
      onSave={save}
    />,
  );
  const selectors = await screen.findAllByRole("combobox");
  await waitFor(() => expect(selectors[0]).toHaveProperty("disabled", false));
  fireEvent.change(selectors[0], { target: { value: mediaId } });
  fireEvent.click(screen.getByRole("button", { name: "保存六类参考图" }));
  expect(save).toHaveBeenCalledWith([{ role: "primary", asset_id: mediaId }]);
  expect(query.getBibleMediaCandidates).toHaveBeenCalledTimes(1);
});

it("历史正文按指定不可变版本懒读，翻页失败不把缓存或最新正文当旧历史", async () => {
  const old = detail().current;
  vi.mocked(query.getBibleHistory).mockResolvedValue({
    versions: [old],
    next_cursor: "old-cursor",
  });
  vi.mocked(query.getBibleSnapshot).mockResolvedValue(old);
  const onVersion = vi.fn();
  mount(
    <BibleHistoryDialog
      identity={identity}
      kind="character"
      id={entryId}
      onVersion={onVersion}
      onClose={vi.fn()}
    />,
  );
  fireEvent.click(await screen.findByRole("button", { name: /查看 v2/ }));
  expect(onVersion).toHaveBeenCalledWith(versionId);
  expect(query.getBibleSnapshot).not.toHaveBeenCalled();
  vi.mocked(query.getBibleHistory).mockRejectedValueOnce(
    new ApiError(403, "forbidden"),
  );
  fireEvent.click(screen.getByRole("button", { name: "读取更多不可变版本" }));
  await screen.findByRole("alert");
  expect(query.getBibleSnapshot).not.toHaveBeenCalled();
});

it("普通完成文本仅为待核验候选，审核后发送正式operation/output身份而不发送任意正文", async () => {
  vi.mocked(query.getBibleResults).mockResolvedValue({
    next_cursor: null,
    items: [{ id: targetId, model_name: "实际文本模型", finished_at: at }],
  } as Awaited<ReturnType<typeof query.getBibleResults>>);
  vi.mocked(query.getBibleResult).mockResolvedValue({
    outputs: [
      { id: mediaId, kind: "text", moderation_status: "passed", sequence: 1 },
      { id: lookId, kind: "text", moderation_status: "rejected", sequence: 2 },
    ],
  } as Awaited<ReturnType<typeof query.getBibleResult>>);
  const controller = writer();
  mount(
    <BibleResultDialog
      identity={identity}
      kind="prop"
      revision={0}
      writer={controller}
      onClose={vi.fn()}
    />,
  );
  expect(screen.getByText(/待后端核验的已完成文本结果候选/)).toBeTruthy();
  await screen.findByRole("option", { name: /实际文本模型/ });
  fireEvent.change(
    screen.getByRole("combobox", { name: "待核验的已完成任务" }),
    { target: { value: targetId } },
  );
  const output = screen.getByRole("combobox", { name: "通过审核的文本候选" });
  await waitFor(() =>
    expect(within(output).getAllByRole("option")).toHaveLength(2),
  );
  fireEvent.change(output, { target: { value: mediaId } });
  fireEvent.click(screen.getByRole("checkbox"));
  fireEvent.click(screen.getByRole("button", { name: "明确采纳选中正式结果" }));
  expect(controller.submit).toHaveBeenCalledWith({
    action: "create_result",
    kind: "prop",
    body: { expected_revision: 0, operation_id: targetId, output_id: mediaId },
  });
});
