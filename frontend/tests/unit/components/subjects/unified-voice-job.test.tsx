import { act, cleanup, renderHook } from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { useSubjectVoiceJob } from "@/components/subjects/unified-voice-job";
const api = vi.hoisted(() => ({
  options: vi.fn(),
  create: vi.fn(),
  get: vi.fn(),
  command: vi.fn(),
}));
vi.mock("@/api/subjects", () => ({
  listSubjectVoiceOptions: api.options,
  createSubjectVoiceJob: api.create,
  getSubjectVoiceJob: api.get,
  getSubjectVoiceCommand: api.command,
}));
const actor = "11111111-1111-4111-8111-111111111111",
  scope = "22222222-2222-4222-8222-222222222222",
  release = "33333333-3333-4333-8333-333333333333",
  model = "44444444-4444-4444-8444-444444444444",
  jobID = "55555555-5555-4555-8555-555555555555",
  asset = "66666666-6666-4666-8666-666666666666";
const option = {
  release_id: release,
  model_version_id: model,
  model_key: "doubao",
  display_name: "豆包语音",
  voices: [{ key: "speaker", name: "默认音色" }],
  default_voice_key: "speaker",
};
function job(body: API.CreateVoiceJobInput, status = "queued") {
  return {
    id: jobID,
    current_actor_id: actor,
    current_scope_id: scope,
    ...body,
    model_version_id: model,
    revision: 1,
    status,
    created_at: "2026-10-28T00:00:00Z",
    updated_at: "2026-10-28T00:00:00Z",
    ...(status === "succeeded"
      ? {
          result: {
            asset_id: asset,
            asset_revision: 1,
            kind: "audio",
            mime_type: "audio/mpeg",
            file_name: "配音.mp3",
            library_item_id: asset,
            library_source: "subject",
          },
        }
      : {}),
  };
}
beforeEach(() => {
  vi.useFakeTimers();
  vi.clearAllMocks();
  api.options.mockResolvedValue({
    current_actor_id: actor,
    current_scope_id: scope,
    available: true,
    models: [option],
  });
  api.create.mockImplementation(async (body) => job(body));
});
afterEach(() => {
  cleanup();
  vi.useRealTimers();
});
function setup() {
  const ready = vi.fn();
  const hook = renderHook(() =>
    useSubjectVoiceJob({ actorID: actor, scopeID: scope, onReady: ready }),
  );
  return { ...hook, ready };
}

it("唯一模型与明确默认音色一键创建真实任务，轮询正式audio后才采用", async () => {
  const { result, ready } = setup();
  await act(async () => {
    await result.current.begin();
  });
  expect(result.current.active).toBe(true);
  expect(ready).not.toHaveBeenCalled();
  const body = api.create.mock.calls[0][0];
  expect(body.release_id).toBe(release);
  expect(body.voice_key).toBe("speaker");
  expect(body).not.toHaveProperty("project_id");
  api.get.mockResolvedValue({ ...job(body, "succeeded"), revision: 2 });
  await act(async () => {
    await vi.advanceTimersByTimeAsync(1500);
  });
  expect(ready).toHaveBeenCalledWith(
    "智能配音1",
    expect.objectContaining({
      asset_id: asset,
      kind: "audio",
      library_source: "subject",
    }),
  );
  expect(result.current.active).toBe(false);
});
it.each(["missing", "ambiguous"])(
  "默认配音配置%s时不弹技术配置、不猜测或提交",
  async (kind) => {
    api.options.mockResolvedValue({
      current_actor_id: actor,
      current_scope_id: scope,
      available: true,
      models:
        kind === "missing"
          ? [{ ...option, default_voice_key: undefined }]
          : [
              option,
              { ...option, release_id: "77777777-7777-4777-8777-777777777777" },
            ],
    });
    const { result } = setup();
    await act(async () => {
      await result.current.begin();
    });
    expect(result.current.phase).toBe("unavailable");
    expect(result.current.active).toBe(false);
    expect(result.current.error).toBe("智能配音暂不可用，请稍后重试。");
    expect(api.create).not.toHaveBeenCalled();
  },
);
it("未知创建结果只查原command，recorded后恢复同job而不再收费", async () => {
  api.create.mockRejectedValueOnce(new Error("network"));
  const { result } = setup();
  await act(async () => {
    await result.current.begin();
  });
  expect(result.current.phase).toBe("unknown");
  const body = api.create.mock.calls[0][0];
  api.command.mockResolvedValue({ state: "recorded", job: job(body) });
  await act(async () => {
    await result.current.recover();
  });
  expect(api.command).toHaveBeenCalledWith(
    { key: api.create.mock.calls[0][1].headers["Idempotency-Key"] },
    expect.anything(),
  );
  expect(api.create).toHaveBeenCalledOnce();
  expect(result.current.active).toBe(true);
});
it("unrecorded并非失败证明，仅显式同键重试原创建，不换key", async () => {
  api.create.mockRejectedValueOnce(new Error("network"));
  const { result } = setup();
  await act(async () => {
    await result.current.begin();
  });
  api.command.mockResolvedValue({ state: "unrecorded" });
  await act(async () => {
    await result.current.recover();
  });
  expect(result.current.phase).toBe("unrecorded");
  expect(api.create).toHaveBeenCalledOnce();
  await act(async () => {
    await result.current.retryOriginal();
  });
  expect(api.create.mock.calls[0][0]).toEqual(api.create.mock.calls[1][0]);
  expect(api.create.mock.calls[0][1].headers["Idempotency-Key"]).toBe(
    api.create.mock.calls[1][1].headers["Idempotency-Key"],
  );
});
it("供应商unknown只恢复读取，不能通过重新生成自动换key", async () => {
  api.create.mockImplementation(async (body) => job(body, "unknown"));
  const { result } = setup();
  await act(async () => {
    await result.current.begin();
  });
  expect(result.current.active).toBe(true);
  expect(result.current.phase).toBe("unknown");
  await act(async () => {
    await result.current.begin();
  });
  expect(api.create).toHaveBeenCalledOnce();
  api.get.mockResolvedValue(job(api.create.mock.calls[0][0], "unknown"));
  await act(async () => {
    await result.current.recover();
  });
  expect(api.get).toHaveBeenCalledWith({ job_id: jobID }, expect.anything());
  expect(api.create).toHaveBeenCalledOnce();
});
it("终态失败保留可读状态，明确重新生成才建立新意图与key", async () => {
  api.create.mockImplementation(async (body) => ({
    ...job(body, "failed"),
    error_code: "provider_failed",
  }));
  const { result } = setup();
  await act(async () => {
    await result.current.begin();
  });
  expect(result.current.phase).toBe("failed");
  expect(result.current.active).toBe(false);
  await act(async () => {
    await result.current.begin();
  });
  expect(api.create).toHaveBeenCalledTimes(2);
  expect(api.create.mock.calls[0][1].headers["Idempotency-Key"]).not.toBe(
    api.create.mock.calls[1][1].headers["Idempotency-Key"],
  );
});
it("错误空间或伪成功媒体不被采纳，不能变成静态ready", async () => {
  api.create.mockImplementation(async (body) => ({
    ...job(body, "succeeded"),
    result: {
      asset_id: asset,
      asset_revision: 1,
      kind: "image",
      mime_type: "image/png",
      file_name: "假音频.png",
      library_item_id: asset,
      library_source: "subject",
    },
  }));
  const { result, ready } = setup();
  await act(async () => {
    await result.current.begin();
  });
  expect(ready).not.toHaveBeenCalled();
  expect(result.current.phase).toBe("unknown");
});
it("卸载取消轮询，晚响应不把音色写回另一份草稿", async () => {
  let resolve!: (value: unknown) => void;
  api.create.mockImplementation(
    () =>
      new Promise((r) => {
        resolve = r;
      }),
  );
  const { result, unmount, ready } = setup();
  let request!: Promise<void>;
  await act(async () => {
    request = result.current.begin();
    await Promise.resolve();
  });
  const body = api.create.mock.calls[0][0];
  unmount();
  await act(async () => {
    resolve(job(body, "succeeded"));
    await request;
  });
  expect(ready).not.toHaveBeenCalled();
  expect(api.get).not.toHaveBeenCalled();
});

it("读取中断保留同job，恢复成功不再创建任务", async () => {
  const { result, ready } = setup();
  await act(async () => {
    await result.current.begin();
  });
  api.get.mockRejectedValueOnce(new Error("network"));
  await act(async () => {
    await vi.advanceTimersByTimeAsync(1500);
  });
  expect(result.current.phase).toBe("read_error");
  expect(result.current.active).toBe(true);
  const body = api.create.mock.calls[0][0];
  api.get.mockResolvedValue({ ...job(body, "succeeded"), revision: 2 });
  await act(async () => {
    await result.current.recover();
  });
  expect(ready).toHaveBeenCalledOnce();
  expect(api.create).toHaveBeenCalledOnce();
  expect(api.get).toHaveBeenCalledTimes(2);
});

it("配置回执不属于当前空间时禁止开始生成", async () => {
  api.options.mockResolvedValue({
    current_actor_id: actor,
    current_scope_id: actor,
    available: true,
    models: [option],
  });
  const { result } = setup();
  await act(async () => {
    await result.current.begin();
  });
  expect(result.current.phase).toBe("unavailable");
  expect(api.create).not.toHaveBeenCalled();
});

it("轮询回执job身份和revision不能回退或替换", async () => {
  const { result, ready } = setup();
  await act(async () => {
    await result.current.begin();
  });
  api.get.mockResolvedValue({
    ...job(api.create.mock.calls[0][0], "succeeded"),
    id: asset,
    revision: 2,
  });
  await act(async () => {
    await vi.advanceTimersByTimeAsync(1500);
  });
  expect(result.current.phase).toBe("read_error");
  expect(ready).not.toHaveBeenCalled();
});

it("连续点击开始只读取一次配置并提交一次任务", async () => {
  const { result } = setup();
  await act(async () => {
    await Promise.all([
      result.current.begin(),
      result.current.begin(),
      result.current.begin(),
    ]);
  });
  expect(api.options).toHaveBeenCalledOnce();
  expect(api.create).toHaveBeenCalledOnce();
});

it("媒体导入待重试仅以原永久key和冻结请求恢复保存音频", async () => {
  api.create.mockImplementationOnce(async (body) => ({
    ...job(body, "ingesting"),
    error_code: "media_import_pending",
  }));
  const { result, ready } = setup();
  await act(async () => {
    await result.current.begin();
  });
  expect(result.current.phase).toBe("import_pending");
  expect(result.current.active).toBe(true);
  await act(async () => {
    await vi.advanceTimersByTimeAsync(3000);
  });
  expect(api.get).not.toHaveBeenCalled();
  const body = api.create.mock.calls[0][0];
  api.create.mockResolvedValue({ ...job(body, "succeeded"), revision: 2 });
  await act(async () => {
    await result.current.retryOriginal();
  });
  expect(api.create.mock.calls[1][0]).toEqual(body);
  expect(api.create.mock.calls[1][1].headers["Idempotency-Key"]).toBe(
    api.create.mock.calls[0][1].headers["Idempotency-Key"],
  );
  expect(ready).toHaveBeenCalledOnce();
  expect(result.current.phase).toBe("ready");
});

it("存储尚未就绪时说明TOS配置与迁移前置，并且不提交收费任务", async () => {
  api.options.mockResolvedValue({
    current_actor_id: actor,
    current_scope_id: scope,
    available: false,
    reason: "voice_storage_unavailable",
    models: [],
  });
  const { result } = setup();
  await act(async () => {
    await result.current.begin();
  });
  expect(result.current.phase).toBe("unavailable");
  expect(result.current.error).toContain("TOS配置与存储迁移");
  expect(api.create).not.toHaveBeenCalled();
});
