import { RequestError } from "@/lib/request";
import { expect, it, vi } from "vitest";
import { ProjectCreationController } from "@/components/projects/project-creation-controller";
import {
  ProjectIntentStore,
  type IntentDatabase,
  type IntentTransactions,
} from "@/components/projects/project-intent-store";
import { type ProjectCreationContext } from "@/components/projects/project-creation-state";
class Memory implements IntentTransactions {
  fail = false;
  state: IntentDatabase = { fence: 0, intents: [] };
  async transact<T>(_write: boolean, run: (state: IntentDatabase) => T) {
    if (this.fail) throw new Error("creation_storage_failed");
    return run(this.state);
  }
}
const context: ProjectCreationContext = {
  actorId: "00000000-0000-4000-8000-000000000001",
  scopeId: "00000000-0000-4000-8000-000000000002",
  sessionGeneration: 1,
  scopeGeneration: 1,
  fenceEpoch: 0,
};
const target = {
  projectId: "00000000-0000-4000-8000-000000000003",
  canvasId: "00000000-0000-4000-8000-000000000004",
};
function fixture() {
  const backend = new Memory();
  const intents = new ProjectIntentStore(backend, () => ({
    sessionGeneration: 1,
    scopeGeneration: 1,
  }));
  const recovery = vi.fn().mockResolvedValue({
    state: "unrecorded",
    project_id: null,
    canvas_id: null,
    project_status: null,
  });
  const create = vi
    .fn()
    .mockResolvedValue({ id: target.projectId, canvas_id: target.canvasId });
  const read = vi.fn().mockResolvedValue(undefined),
    navigate = vi.fn();
  const ports = {
    intents,
    context: async () => {
      await intents.activate(context);
      return context;
    },
    isCurrent: () => true,
    resolve: recovery,
    create,
    readTarget: read,
    navigate,
    allowedScopes: () => [context.scopeId],
  };
  return {
    controller: new ProjectCreationController(ports),
    ports,
    intents,
    recovery,
    create,
    read,
    navigate,
    backend,
  };
}
it("initial creation requires a readable recovery contract and sends frozen defaults once", async () => {
  const f = fixture();
  await f.controller.start();
  expect(f.create).toHaveBeenCalledTimes(1);
  expect(f.create.mock.calls[0][0]).toEqual({
    name: "未命名画布",
    description: "",
    aspect_ratio: "16:9",
    style_type: "realistic",
  });
  expect(f.navigate).toHaveBeenCalledWith(target);
  expect(f.controller.snapshot().phase).toBe("ready");
  const broken = fixture();
  broken.recovery.mockRejectedValue(new Error("service unavailable"));
  await broken.controller.start();
  expect(broken.create).not.toHaveBeenCalled();
  expect(broken.controller.snapshot().phase).toBe("preflight_failed");
});
it("refresh and unrecorded never automatically retry an uncertain POST", async () => {
  const f = fixture();
  f.create.mockRejectedValue(new Error("connection lost"));
  await f.controller.start();
  const key = f.create.mock.calls[0][1];
  expect(f.controller.snapshot().phase).toBe("unknown");
  const restored = new ProjectCreationController(f.ports);
  await restored.restore();
  expect(f.create).toHaveBeenCalledTimes(1);
  expect(restored.snapshot().phase).toBe("unrecorded");
  f.create.mockResolvedValue({
    id: target.projectId,
    canvas_id: target.canvasId,
  });
  await restored.continueOriginal();
  expect(f.create.mock.calls[1][1]).toBe(key);
  expect(f.navigate).toHaveBeenCalledWith(target);
});
it("a created target that cannot open is retried through reads only", async () => {
  const f = fixture();
  f.read.mockRejectedValueOnce(new Error("read unavailable"));
  await f.controller.start();
  expect(f.controller.snapshot().phase).toBe("open_failed");
  await f.controller.reopen();
  expect(f.create).toHaveBeenCalledTimes(1);
  expect(f.read).toHaveBeenCalledTimes(2);
  expect(f.controller.snapshot().phase).toBe("ready");
});
it("a malformed recovery success cannot dispatch or clear the journal", async () => {
  const f = fixture();
  f.recovery.mockResolvedValue({ state: "unrecorded" });
  await f.controller.start();
  expect(f.create).not.toHaveBeenCalled();
  expect(f.controller.snapshot().phase).toBe("preflight_failed");
  expect(await f.intents.readForActor(context)).toHaveLength(1);
});

it("storage failure after dispatch cannot be reported as a preflight failure", async () => {
  const f = fixture();
  f.create.mockImplementation(async () => {
    f.backend.fail = true;
    throw new Error("response lost");
  });
  await f.controller.start();
  expect(f.create).toHaveBeenCalledTimes(1);
  expect(f.controller.snapshot().phase).toBe("unknown");
});

it("an explicit start during an empty background restore is not lost", async () => {
  const f = fixture();
  let release!: () => void;
  const wait = new Promise<void>((resolve) => {
    release = resolve;
  });
  const original = f.ports.context;
  f.ports.context = async () => {
    await wait;
    return original();
  };
  const restoring = f.controller.restore();
  const starting = f.controller.start();
  release();
  await restoring;
  await starting;
  expect(f.create).toHaveBeenCalledTimes(1);
});
it("a definite initial 422 ends only the initial attempt, while unknown then 422 retains its key", async () => {
  const f = fixture();
  f.create.mockRejectedValue(new RequestError("invalid_request", 422));
  await f.controller.start();
  expect(f.controller.snapshot().phase).toBe("rejected");
  expect(await f.intents.readForActor(context)).toHaveLength(0);
  const g = fixture();
  g.create
    .mockRejectedValueOnce(new Error("lost"))
    .mockRejectedValueOnce(new RequestError("invalid_request", 422));
  await g.controller.start();
  await g.controller.resolve();
  await g.controller.continueOriginal();
  expect(g.controller.snapshot().phase).toBe("unknown");
  expect(g.create.mock.calls[1][1]).toBe(g.create.mock.calls[0][1]);
  expect(await g.intents.readForActor(context)).toHaveLength(1);
});
it("acknowledgement storage failure prevents navigation and a new intent", async () => {
  const f = fixture();
  f.read.mockImplementation(async () => {
    f.backend.fail = true;
  });
  await f.controller.start();
  expect(f.controller.snapshot().phase).toBe("ack_failed");
  expect(f.navigate).not.toHaveBeenCalled();
  f.backend.fail = false;
  f.read.mockResolvedValue(undefined);
  await f.controller.start();
  expect(f.create).toHaveBeenCalledTimes(1);
  await f.controller.reopen();
  expect(f.navigate).toHaveBeenCalledWith(target);
});
it("dispose during submission ignores a late receipt and never navigates", async () => {
  const f = fixture();
  let release!: (value: unknown) => void;
  f.create.mockImplementation(
    () =>
      new Promise((resolve) => {
        release = resolve;
      }),
  );
  const starting = f.controller.start();
  await vi.waitFor(() => expect(f.create).toHaveBeenCalledTimes(1));
  f.controller.dispose();
  release({ id: target.projectId, canvas_id: target.canvasId });
  await starting;
  expect(f.navigate).not.toHaveBeenCalled();
});
it("mount restore and an existing prepared intent never automatically dispatch", async () => {
  const f = fixture();
  await f.controller.restore();
  expect(f.create).not.toHaveBeenCalled();
  await f.intents.beginOrResume(context, {
    name: "冻结名称",
    description: "",
    aspect_ratio: "1:1",
    style_type: "realistic",
  });
  await f.controller.start();
  expect(f.controller.snapshot().phase).toBe("unrecorded");
  expect(f.create).not.toHaveBeenCalled();
  await f.controller.continueOriginal();
  expect(f.create.mock.calls[0][0].name).toBe("冻结名称");
  expect(f.create.mock.calls[0][0].aspect_ratio).toBe("1:1");
});
it("reopening an old target cannot consume a successor intent from another tab", async () => {
  const f = fixture();
  f.read.mockRejectedValueOnce(new Error("offline"));
  await f.controller.start();
  expect(f.controller.snapshot().phase).toBe("open_failed");
  const second = new ProjectCreationController(f.ports);
  await second.restore();
  await second.reopen();
  let release!: (value: unknown) => void;
  f.create.mockImplementation(
    () =>
      new Promise((resolve) => {
        release = resolve;
      }),
  );
  const next = second.start();
  await vi.waitFor(() => expect(f.create).toHaveBeenCalledTimes(2));
  const before = structuredClone(f.backend.state.intents);
  await f.controller.reopen();
  expect(f.backend.state.intents).toEqual(before);
  expect(f.controller.snapshot().phase).toBe("ready");
  release({ id: target.projectId, canvas_id: target.canvasId });
  await next;
});
it.each(["success", "failure"])(
  "late %s after another tab acknowledged stops busy state without touching a successor",
  async (result) => {
    const f = fixture();
    const attempts: Array<{
      resolve: (value: unknown) => void;
      reject: (reason: unknown) => void;
    }> = [];
    f.create.mockImplementation(
      () =>
        new Promise((resolve, reject) => attempts.push({ resolve, reject })),
    );
    const first = f.controller.start();
    await vi.waitFor(() => expect(attempts).toHaveLength(1));
    const second = new ProjectCreationController(f.ports);
    await second.restore();
    const continuing = second.continueOriginal();
    await vi.waitFor(() => expect(attempts).toHaveLength(2));
    attempts[0].resolve({ id: target.projectId, canvas_id: target.canvasId });
    await first;
    if (result === "success")
      attempts[1].resolve({ id: target.projectId, canvas_id: target.canvasId });
    else attempts[1].reject(new Error("lost"));
    await continuing;
    expect(second.snapshot().phase).not.toBe("submitting");
    expect(f.backend.state.intents).toHaveLength(0);
    expect(f.navigate).toHaveBeenCalledTimes(1);
  },
);
it("a passive restore of a known target offers explicit reopen without navigating or acknowledging", async () => {
  const f = fixture();
  f.read.mockRejectedValueOnce(new Error("offline"));
  await f.controller.start();
  const second = new ProjectCreationController(f.ports);
  f.recovery.mockResolvedValue({
    state: "created",
    project_id: target.projectId,
    canvas_id: target.canvasId,
    project_status: "active",
  });
  await second.restore();
  expect(second.snapshot().phase).toBe("open_failed");
  expect(f.navigate).not.toHaveBeenCalled();
  expect(f.backend.state.intents).toHaveLength(1);
  await second.reopen();
  expect(f.navigate).toHaveBeenCalledWith(target);
});
it("logout intent invalidates navigation even after acknowledgement committed but before server logout returns", async () => {
  const f = fixture();
  let release!: () => void;
  const wait = new Promise<void>((resolve) => {
    release = resolve;
  });
  const acknowledge = f.intents.acknowledge.bind(f.intents);
  let committed = false;
  vi.spyOn(f.intents, "acknowledge").mockImplementation(async (...args) => {
    await acknowledge(...args);
    committed = true;
    await wait;
  });
  const pending = f.controller.start();
  await vi.waitFor(() => expect(committed).toBe(true));
  await f.intents.invalidate("passive_expiry");
  release();
  await pending;
  expect(f.navigate).not.toHaveBeenCalled();
});
