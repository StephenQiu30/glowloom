import { expect, it } from "vitest";
import {
  ProjectIntentStore,
  type IntentDatabase,
  type IntentTransactions,
} from "@/components/projects/project-intent-store";
import {
  blankProjectInput,
  type ProjectCreationContext,
} from "@/components/projects/project-creation-state";

class MemoryTransactions implements IntentTransactions {
  state: IntentDatabase = { fence: 0, intents: [] };
  private queue = Promise.resolve();
  transact<T>(_write: boolean, run: (state: IntentDatabase) => T): Promise<T> {
    const next = this.queue.then(() => {
      const state = structuredClone(this.state);
      const result = run(state);
      this.state = state;
      return structuredClone(result);
    });
    this.queue = next.then(
      () => {},
      () => {},
    );
    return next;
  }
}
const context = (): ProjectCreationContext => ({
  actorId: "00000000-0000-4000-8000-000000000001",
  scopeId: "00000000-0000-4000-8000-000000000002",
  sessionGeneration: 1,
  scopeGeneration: 1,
  fenceEpoch: 0,
});
const store = (backend: MemoryTransactions) =>
  new ProjectIntentStore(backend, () => ({
    sessionGeneration: 1,
    scopeGeneration: 1,
  }));

it("two clients share an intent and only one initial dispatch wins", async () => {
  const backend = new MemoryTransactions(),
    a = store(backend),
    b = store(backend),
    ctx = context();
  await a.activate(ctx);
  await b.activate(ctx);
  const [first, second] = await Promise.all([
    a.beginOrResume(ctx, blankProjectInput()),
    b.beginOrResume(ctx, { ...blankProjectInput(), name: "之后的默认名" }),
  ]);
  expect(first.disposition).toBe("new");
  expect(second.disposition).toBe("existing");
  expect(first.intent.key).toBe(second.intent.key);
  expect(second.intent.body.name).toBe("未命名画布");
  const claim = await Promise.allSettled([
    a.beginDispatch(ctx, first.intent.key, 1, "initial"),
    b.beginDispatch(ctx, first.intent.key, 1, "initial"),
  ]);
  expect(claim.filter((v) => v.status === "fulfilled")).toHaveLength(1);
  expect((await a.readForActor(ctx))[0].body).toEqual({
    name: "未命名画布",
    description: "",
    aspect_ratio: "16:9",
    style_type: "realistic",
  });
});

it("a late timeout cannot regress a created target or delete a later intent", async () => {
  const backend = new MemoryTransactions(),
    a = store(backend),
    ctx = context();
  await a.activate(ctx);
  const { intent } = await a.beginOrResume(ctx, blankProjectInput());
  const sent = await a.beginDispatch(ctx, intent.key, 1, "initial");
  const created = await a.transition(
    ctx,
    intent.key,
    sent.localRevision,
    "created",
    {
      projectId: "00000000-0000-4000-8000-000000000003",
      canvasId: "00000000-0000-4000-8000-000000000004",
    },
  );
  await expect(
    a.transition(ctx, intent.key, sent.localRevision, "unknown"),
  ).rejects.toThrow();
  await a.acknowledge(ctx, intent.key, created.localRevision, "opened");
  const next = await a.beginOrResume(ctx, blankProjectInput());
  await expect(
    a.acknowledge(ctx, intent.key, created.localRevision, "opened"),
  ).rejects.toThrow();
  expect((await a.readForActor(ctx))[0].key).toBe(next.intent.key);
});

it("logout fences old writers and different actors cannot recover the old intent", async () => {
  const backend = new MemoryTransactions(),
    a = store(backend),
    b = store(backend),
    ctx = context();
  await a.activate(ctx);
  await b.activate(ctx);
  const { intent } = await a.beginOrResume(ctx, blankProjectInput());
  await b.invalidate("active_logout");
  await expect(
    a.beginDispatch(ctx, intent.key, 1, "initial"),
  ).rejects.toThrow();
  const other = {
    ...ctx,
    actorId: "00000000-0000-4000-8000-000000000005",
    fenceEpoch: 1,
  };
  await b.activate(other);
  expect(await b.readForActor(other)).toEqual([]);
});

it("an uncertain retry cannot be terminated as validation rejected", async () => {
  const backend = new MemoryTransactions(),
    a = store(backend),
    ctx = context();
  await a.activate(ctx);
  const { intent } = await a.beginOrResume(ctx, blankProjectInput());
  const sent = await a.beginDispatch(ctx, intent.key, 1, "initial");
  const unknown = await a.transition(
    ctx,
    intent.key,
    sent.localRevision,
    "unknown",
  );
  const retry = await a.beginDispatch(
    ctx,
    intent.key,
    unknown.localRevision,
    "explicit_continuation",
  );
  await expect(
    a.transition(ctx, intent.key, retry.localRevision, "rejected"),
  ).rejects.toThrow();
  expect((await a.readForActor(ctx))[0].hasUncertainDispatch).toBe(true);
});

it("unsupported persisted contract versions block instead of being repaired", async () => {
  const backend = new MemoryTransactions(),
    a = store(backend),
    ctx = context();
  await a.activate(ctx);
  await a.beginOrResume(ctx, blankProjectInput());
  (backend.state.intents[0] as { contractVersion: number }).contractVersion =
    99;
  await expect(a.beginOrResume(ctx, blankProjectInput())).rejects.toThrow();
  expect(backend.state.intents).toHaveLength(1);
});

it("passive invalidation cannot be undone by an older activation callback", async () => {
  const backend = new MemoryTransactions(),
    a = store(backend),
    ctx = context();
  const activating = a.activate(ctx);
  await a.invalidate("passive_expiry");
  await expect(activating).rejects.toThrow();
  await expect(a.beginOrResume(ctx, blankProjectInput())).rejects.toThrow();
});

it("a successful strong clear remains successful when passive expiry overlaps", async () => {
  const backend = new MemoryTransactions(),
    a = store(backend);
  const clearing = a.invalidate("identity_change");
  await a.invalidate("passive_expiry");
  await clearing;
  const ctx = { ...context(), fenceEpoch: 1 };
  await a.activate(ctx);
  expect((await a.beginOrResume(ctx, blankProjectInput())).disposition).toBe(
    "new",
  );
});
