import { authorizedText } from "./helpers/conversation-input.js";
import { randomUUID } from "node:crypto";
import { BACKGROUND_CONTEXT as context } from "@earendil-works/chord/context";
import { expect, it, vi } from "vitest";
import {
  BusinessClient,
  type Grant,
  type CallReceipt,
} from "../src/business/client.js";
import { SessionRuntime } from "../src/sessions/runtime.js";
import { SessionService } from "../src/sessions/service.js";
import { SessionStore } from "../src/sessions/store.js";
import { freezeManifest } from "../src/sessions/manifest.js";
import { StateDoc } from "../src/harness/docs.js";
import { testPool } from "./storage/database.js";
import { recoveryPumpPool } from "./helpers/recovery-database.js";

const scope = () => ({
  actor_id: randomUUID(),
  org_id: randomUUID(),
  scope_id: randomUUID(),
  project_id: randomUUID(),
});
class Fixture extends BusinessClient {
  grants = 0;
  calls = 0;
  callsByRun = new Map<string, number>();
  constructor(readonly who: ReturnType<typeof scope>) {
    super("http://fixture.invalid", "fixture");
  }
  override async tools() {
    return [];
  }
  override async decisions() {
    return {
      current: null,
      authorization_revision: 0,
      barrier: false,
      terminal: false,
    };
  }
  override async grant(
    input: Parameters<BusinessClient["grant"]>[0],
  ): Promise<Grant> {
    this.grants++;
    return {
      ...input,
      grant_id: randomUUID(),
      available_micros: 1000000,
      expires_at: input.lease_until,
      scope: this.who,
    };
  }
  override async validate(grant: Grant) {
    return grant;
  }
  override async snapshot() {
    return { revision: 1 };
  }
  override async findCall() {
    return undefined;
  }
  override async execute(
    ...args: Parameters<BusinessClient["execute"]>
  ): Promise<CallReceipt> {
    this.calls++;
    const [, , grant, call] = args;
    this.callsByRun.set(
      grant.root_run_id,
      (this.callsByRun.get(grant.root_run_id) ?? 0) + 1,
    );
    return {
      receipt_id: randomUUID(),
      root_run_id: grant.root_run_id,
      submission_id: grant.submission_id,
      call,
      status: "completed",
      cost_micros: 0,
      result: {
        output: "Done",
        stop_reason: "stop",
        usage: { input_tokens: 1, output_tokens: 1 },
        model_key: "fixture",
      },
    };
  }
}
async function until(check: () => Promise<boolean>) {
  for (let i = 0; i < 250; i++) {
    if (await check()) return;
    await new Promise((r) => setTimeout(r, 10));
  }
  throw Error("Lifecycle state did not converge");
}

it("closes a real PG Harness immediately on StorageFailed and releases its owner without renewing grants", async () => {
  const pool = testPool(),
    who = scope(),
    business = new Fixture(who),
    store = new SessionStore(pool);
  const session = await store.createSession({
    ...who,
    request_id: randomUUID(),
    content_hash: "a".repeat(64),
    title: "Storage failure",
  });
  const run = await store.admitRun(
    session.id,
    {
      input: authorizedText("Test"),
      ...who,
      request_id: randomUUID(),
      content_hash: "b".repeat(64),
      expected_execution_revision: 0,
      goal: "Test",
      manifest: { model_key: "fixture" },
    },
    freezeManifest({ model_key: "fixture" }, []),
  );
  const runtime = await SessionRuntime.open(pool, business, run, 30000);
  const connect = pool.connect.bind(pool);
  let armed = false;
  const spy = vi.spyOn(pool, "connect").mockImplementation(((
    ...args: unknown[]
  ) => {
    if (args.length) return Reflect.apply(connect, pool, args);
    return (async () => {
      const client = await connect();
      const query = client.query.bind(client);
      return new Proxy(client, {
        get(target, key) {
          if (key === "query")
            return async (...args: unknown[]) => {
              if (
                armed &&
                String(args[0]).includes("INSERT INTO agent.runtime_events")
              ) {
                armed = false;
                throw Error("faultgate projection unavailable");
              }
              return Reflect.apply(query, target, args);
            };
          const value = Reflect.get(target, key);
          return typeof value === "function" ? value.bind(target) : value;
        },
      });
    })();
  }) as typeof pool.connect);
  try {
    await runtime.authorize();
    armed = true;
    await expect(
      runtime.harness.commit(async (tx) => {
        const state = await tx.doc(StateDoc, runtime.root.id);
        state.execution_revision++;
      }, context),
    ).rejects.toThrow();
    expect(runtime.isClosed).toBe(true);
    expect(() => runtime.grant()).toThrow();
    await until(async () =>
      Boolean(
        (
          await pool.query(
            "SELECT lease_until<=clock_timestamp() released FROM agent.runtime_sessions WHERE session_id=$1",
            [session.id],
          )
        ).rows[0]?.released,
      ),
    );
    const grants = business.grants;
    await runtime.renew();
    expect(business.grants).toBe(grants);
    await expect(runtime.assertActive()).rejects.toThrow();
  } finally {
    spy.mockRestore();
    await runtime.close();
    await pool.end();
  }
});

it("waits for an in-flight real PG runtime opening during close and relinquishes its lease", async () => {
  const pool = testPool(),
    who = scope(),
    business = new Fixture(who),
    service = new SessionService(pool, business, { leaseMs: 30000 });
  let released!: () => void, opened!: () => void;
  const gate = new Promise<void>((r) => (released = r)),
    ready = new Promise<void>((r) => (opened = r));
  const open = SessionRuntime.open.bind(SessionRuntime);
  const spy = vi
    .spyOn(SessionRuntime, "open")
    .mockImplementation(async (...args) => {
      const runtime = await open(...args);
      opened();
      await gate;
      return runtime;
    });
  try {
    const session = await service.createSession({
      ...who,
      request_id: randomUUID(),
      content_hash: "a".repeat(64),
      title: "Opening close race",
    });
    await service.createRun(session.id, {
      input: authorizedText("Test"),
      ...who,
      request_id: randomUUID(),
      content_hash: "b".repeat(64),
      expected_execution_revision: 0,
      goal: "Test",
      manifest: { model_key: "fixture" },
    });
    await ready;
    let closed = false;
    const closing = service.close().then(() => {
      closed = true;
    });
    await new Promise((r) => setImmediate(r));
    expect(closed).toBe(false);
    released();
    await closing;
    expect(business.calls).toBe(0);
    expect(
      (
        await pool.query(
          "SELECT lease_until<=clock_timestamp() released FROM agent.runtime_sessions WHERE session_id=$1",
          [session.id],
        )
      ).rows[0]?.released,
    ).toBe(true);
  } finally {
    released();
    spy.mockRestore();
    await service.close();
    await pool.end();
  }
});

it("recovers an unresolved run beyond 33 earlier completed sessions using real PG projections", async () => {
  const pool = recoveryPumpPool(),
    who = scope(),
    business = new Fixture(who),
    service = new SessionService(pool, business, { leaseMs: 30000 });
  let recovery: SessionService | undefined;
  try {
    for (let i = 0; i < 33; i++) {
      const session = await service.createSession({
        ...who,
        request_id: randomUUID(),
        content_hash: "a".repeat(64),
        title: `Completed ${i}`,
      });
      const run = await service.createRun(session.id, {
        input: authorizedText("Test"),
        ...who,
        request_id: randomUUID(),
        content_hash: "b".repeat(64),
        expected_execution_revision: 0,
        goal: "Test",
        manifest: { model_key: "fixture" },
      });
      await service.waitForRun(run.root_run_id);
      expect((await service.readRun(run.root_run_id, who))?.state.status).toBe(
        "completed",
      );
    }
    await service.close();
    const session = await service.store.createSession({
      ...who,
      request_id: randomUUID(),
      content_hash: "a".repeat(64),
      title: "Unresolved beyond batch",
    });
    const run = await service.store.admitRun(
      session.id,
      {
        input: authorizedText("Test"),
        ...who,
        request_id: randomUUID(),
        content_hash: "b".repeat(64),
        expected_execution_revision: 0,
        goal: "Test",
        manifest: { model_key: "fixture" },
      },
      freezeManifest({ model_key: "fixture" }, []),
    );
    recovery = new SessionService(pool, business, { leaseMs: 30000 });
    recovery.startRecovery();
    await until(
      async () =>
        (await recovery!.readRun(run.id, who))?.state.status === "completed",
    );
    expect(business.callsByRun.get(run.id)).toBe(1);
  } finally {
    await recovery?.close();
    await service.close();
    await pool.end();
  }
}, 30000);
