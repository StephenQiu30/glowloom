import { authorizedText } from "./helpers/conversation-input.js";
import { access } from "node:fs/promises";
import { randomUUID } from "node:crypto";
import { expect, it } from "vitest";
import { SessionService } from "../src/sessions/service.js";
import {
  BusinessClient,
  BusinessError,
  type Grant,
  type CallReceipt,
} from "../src/business/client.js";
import { testPool } from "./storage/database.js";
import { type BusinessTool } from "../src/sessions/manifest.js";
const scope = () => ({
  actor_id: randomUUID(),
  org_id: randomUUID(),
  scope_id: randomUUID(),
  project_id: randomUUID(),
});
class Fixture extends BusinessClient {
  calls: unknown[] = [];
  who = scope();
  constructor(
    readonly respond: (
      input: Record<string, unknown>,
      kind: string,
      name: string,
    ) => unknown,
    readonly descriptors: BusinessTool[] = [],
  ) {
    super("http://127.0.0.1:1", "fixture");
  }
  override async tools() {
    return this.descriptors;
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
    const [kind, name, grant, call, input] = args;
    this.calls.push(input);
    const result = this.respond(input as Record<string, unknown>, kind, name);
    return {
      receipt_id: randomUUID(),
      root_run_id: grant.root_run_id,
      submission_id: grant.submission_id,
      call,
      status: "completed",
      cost_micros: 1,
      result:
        kind === "model"
          ? {
              ...(result as object),
              usage: { input_tokens: 1, output_tokens: 1 },
              model_key: "test",
            }
          : result,
    };
  }
}
const answer = (output: string, tool_calls?: unknown[]) => ({
  output,
  stop_reason: tool_calls ? "tool_use" : "stop",
  ...(tool_calls ? { tool_calls } : {}),
});
async function run(
  business: Fixture,
  manifest: Record<string, unknown> = { model_key: "test" },
) {
  const pool = testPool();
  const service = new SessionService(pool, business, { leaseMs: 30000 });
  const session = await service.createSession({
    ...business.who,
    request_id: randomUUID(),
    content_hash: "a".repeat(64),
    title: "能力",
  });
  const created = await service.createRun(session.id, {
    ...business.who,
    request_id: randomUUID(),
    content_hash: "b".repeat(64),
    goal: "目标与来源",
    input: authorizedText("目标与来源"),
    expected_execution_revision: 0,
    manifest,
  });
  await service.waitForRun(created.root_run_id);
  return {
    pool,
    service,
    session,
    created,
    snapshot: await service.readRun(created.root_run_id, business.who),
    close: async () => {
      await service.close();
      await pool.end();
    },
  };
}
it("runs two actual owned children, preserves a local failure and adopts only the completed committed source", async () => {
  let model = 0;
  let adopted: string | undefined;
  const business = new Fixture(
    (input) => {
      model++;
      if (model === 1)
        return answer("专业任务", [
          {
            id: "script",
            name: "delegate",
            arguments: {
              role: "script",
              task: "分析剧本",
              budget: { model_steps: 1, tool_steps: 0 },
            },
          },
          {
            id: "art",
            name: "delegate",
            arguments: {
              role: "art",
              task: "分析美术",
              budget: { model_steps: 2, tool_steps: 1 },
            },
          },
        ]);
      if (model === 2) throw new Error("script provider failed");
      if (model === 3) {
        expect(JSON.stringify(input)).toContain("art_only");
        expect(JSON.stringify(input)).not.toContain("script_only");
        return answer("art result, user source");
      }
      if (model === 4) {
        const messages = input.messages as { role: string; content: string }[];
        const result = JSON.parse(
          messages.filter((m) => m.role === "tool").at(-1)!.content,
        );
        adopted = result.task_id;
        return answer("采用美术", [
          {
            id: "adopt",
            name: "plan_update",
            arguments: {
              items: [],
              adopt: [
                {
                  delegation_id: result.task_id,
                  source_refs: result.source_refs,
                  artifacts: [],
                },
              ],
            },
          },
        ]);
      }
      return answer("采用完成");
    },
    [
      {
        name: "script_only",
        description: "script only",
        parameters: { type: "object" },
        contract_version: "1",
        replay: "safe",
        effect: "read",
        roles: ["script"],
      },
      {
        name: "art_only",
        description: "art only",
        parameters: { type: "object" },
        contract_version: "1",
        replay: "safe",
        effect: "read",
        roles: ["art"],
      },
      {
        name: "unassigned_read",
        description: "must not inherit",
        parameters: { type: "object" },
        contract_version: "1",
        replay: "safe",
        effect: "read",
      },
    ],
  );
  const r = await run(business);
  try {
    expect(r.snapshot?.state.delegations).toHaveLength(2);
    expect(r.snapshot?.state.delegations?.map((d) => d.status)).toEqual([
      "failed",
      "done",
    ]);
    expect(r.snapshot?.state.adopted?.[0]?.delegation_id).toBe(adopted);
    expect(r.snapshot?.state.tool_steps).toBe(3);
    expect(r.snapshot?.state.model_steps).toBe(5);
    const children = await r.pool.query(
      "SELECT body FROM agent.runtime_records WHERE session_id=$1 AND record_type='conversation' AND owner_task_id IS NOT NULL",
      [r.session.id],
    );
    expect(children.rowCount).toBe(2);
  } finally {
    await r.close();
  }
});
it("bridges authorized Go image content into actual Pi tool output and the next model request", async () => {
  const data = "iVBORw0KGgo=";
  let model = 0;
  let seen: unknown;
  const business = new Fixture(
    (input, kind) => {
      if (kind === "tool")
        return {
          content: [
            { type: "text", text: "Authorized frame 1" },
            { type: "image", data, mimeType: "image/png" },
          ],
          source: "asset:authorized",
        };
      model++;
      if (model === 1)
        return answer("读取帧", [
          { id: "frame", name: "read_frame", arguments: {} },
        ]);
      seen = input;
      return answer("已检查帧");
    },
    [
      {
        name: "read_frame",
        description: "read authorized frame",
        parameters: { type: "object" },
        contract_version: "1",
        replay: "safe",
        effect: "read",
        roles: ["qa"],
      },
    ],
  );
  const r = await run(business);
  try {
    const entries = await r.pool.query(
      "SELECT body FROM agent.runtime_records WHERE session_id=$1 AND record_type='entry' AND body::text LIKE '%image/png%'",
      [r.session.id],
    );
    expect(entries.rowCount).toBeGreaterThan(0);
    expect(JSON.stringify(seen)).toContain(`data:image/png;base64,${data}`);
    expect(JSON.stringify(r.snapshot?.events)).not.toContain(data);
  } finally {
    await r.close();
  }
});
it("loads only frozen authorized Skill content, persists it for later requests, and counts its core step", async () => {
  let model = 0;
  class Skills extends Fixture {
    override async skill(...args: Parameters<BusinessClient["skill"]>) {
      expect(args[1]).toBe("s1");
      return {
        skill_id: "s1",
        version: "v1",
        resource_hash: "hash1",
        content: "Pinned Skill: no host shell; source v1",
      };
    }
  }
  const business = new Skills((input) => {
    model++;
    if (model === 1)
      return answer("读Skill", [
        { id: "load", name: "skill_load", arguments: { skill_id: "s1" } },
      ]);
    expect(JSON.stringify(input)).toContain("Pinned Skill: no host shell");
    return answer("约束已读");
  });
  const r = await run(business, {
    model_key: "test",
    skills: [{ skill_id: "s1", version: "v1", resource_hash: "hash1" }],
  });
  try {
    const docs = await r.pool.query(
      "SELECT v.content FROM agent.runtime_records r JOIN agent.runtime_document_versions v ON v.session_id=r.session_id AND v.document_id=r.id WHERE r.session_id=$1 AND r.record_type='document' AND r.kind_key=$2 ORDER BY v.commit_seq DESC LIMIT 1",
      [r.session.id, JSON.stringify("glowloom.agent")],
    );
    expect(JSON.stringify(docs.rows)).toContain("Pinned Skill: no host shell");
    expect(r.snapshot?.state.tool_steps).toBe(1);
  } finally {
    await r.close();
  }
});

it("keeps a Skill script behind the formal Go isolation command and refuses missing isolation without a host fallback", async () => {
  const marker = "/tmp/glowloom-never-execute-" + randomUUID();
  let model = 0;
  let denied = false;
  const business = new Fixture(
    (input, kind) => {
      if (kind === "tool") {
        denied = true;
        throw new BusinessError(503, "skill_isolation_unavailable");
      }
      model++;
      if (model === 1)
        return answer("执行获准Skill", [
          {
            id: "script",
            name: "skill_execute",
            arguments: { skill_id: "script", script: "touch " + marker },
          },
        ]);
      expect(JSON.stringify(input)).toContain("skill_isolation_unavailable");
      return answer("隔离设施缺失，执行失败");
    },
    [
      {
        name: "skill_execute",
        description: "Go sandbox only",
        parameters: {
          type: "object",
          properties: {
            skill_id: { type: "string" },
            script: { type: "string" },
          },
          required: ["skill_id", "script"],
        },
        contract_version: "1",
        replay: "unsafe",
        effect: "write",
        roles: ["script"],
      },
    ],
  );
  const r = await run(business);
  try {
    expect(denied).toBe(true);
    expect(r.snapshot?.state.tool_steps).toBe(1);
    await expect(access(marker)).rejects.toThrow();
  } finally {
    await r.close();
  }
});
it("retries only the failed professional task in a new owned child using an explicit redo reference", async () => {
  let model = 0;
  let failedTask = "";
  const business = new Fixture((input) => {
    model++;
    if (model === 1)
      return answer("委派", [
        {
          id: "initial",
          name: "delegate",
          arguments: {
            role: "script",
            task: "分析一段",
            budget: { model_steps: 1, tool_steps: 0 },
          },
        },
      ]);
    if (model === 2) throw new Error("local script failure");
    if (model === 3) {
      const result = JSON.parse(
        (input.messages as { role: string; content: string }[])
          .filter((x) => x.role === "tool")
          .at(-1)!.content,
      );
      failedTask = result.task_id;
      return answer("仅重做失败项", [
        {
          id: "redo",
          name: "delegate",
          arguments: {
            role: "script",
            task: "仅重做原失败项",
            redo_of: failedTask,
            budget: { model_steps: 1, tool_steps: 0 },
          },
        },
      ]);
    }
    return answer(model === 4 ? "重做结果，来自用户目标" : "结果等待显式采用");
  });
  const r = await run(business);
  try {
    expect(r.snapshot?.state.delegations?.map((x) => x.status)).toEqual([
      "failed",
      "done",
    ]);
    expect(r.snapshot?.state.delegations?.[1]?.redo_of).toBe(failedTask);
    expect(r.snapshot?.state.delegations?.[1]?.conversation_id).not.toBe(
      r.snapshot?.state.delegations?.[0]?.conversation_id,
    );
    expect(r.snapshot?.state.adopted).toEqual([]);
  } finally {
    await r.close();
  }
});
it("does not allow a specialist to delegate recursively or mutate the producer plan through hidden tools", async () => {
  let model = 0;
  const business = new Fixture((input) => {
    model++;
    if (model === 1)
      return answer("委派", [
        {
          id: "initial",
          name: "delegate",
          arguments: {
            role: "script",
            task: "有限分析",
            budget: { model_steps: 3, tool_steps: 1 },
          },
        },
      ]);
    if (model === 2) {
      expect((input.tools as { name: string }[]).map((x) => x.name)).toEqual([
        "skill_load",
      ]);
      return answer("试图扩权", [
        {
          id: "recursive",
          name: "delegate",
          arguments: { role: "art", task: "不能运行" },
        },
        {
          id: "plan",
          name: "plan_update",
          arguments: {
            items: [
              {
                id: "bad",
                title: "无权计划",
                status: "completed",
                depends_on: [],
                artifacts: [],
              },
            ],
          },
        },
      ]);
    }
    return answer("受限分析");
  });
  const r = await run(business);
  try {
    expect(r.snapshot?.state.delegations).toHaveLength(1);
    expect(r.snapshot?.state.plan).toEqual([]);
    expect(r.snapshot?.state.tool_steps).toBe(1);
  } finally {
    await r.close();
  }
});
it("enforces the allocated child tool allowance and retains one shared root budget", async () => {
  let model = 0;
  let tool = 0;
  const business = new Fixture(
    (_input, kind) => {
      if (kind === "tool") {
        tool++;
        return { observed: "receipt" };
      }
      model++;
      if (model === 1)
        return answer("委派小额度", [
          {
            id: "child",
            name: "delegate",
            arguments: {
              role: "qa",
              task: "仅一次读取",
              budget: { model_steps: 2, tool_steps: 1 },
            },
          },
        ]);
      if (model === 2)
        return answer("两次读取", [
          { id: "a", name: "read_frame", arguments: {} },
          { id: "b", name: "read_frame", arguments: {} },
        ]);
      return answer("有限结果");
    },
    [
      {
        name: "read_frame",
        description: "authorized frame",
        parameters: { type: "object" },
        contract_version: "1",
        replay: "safe",
        effect: "read",
        roles: ["qa"],
      },
    ],
  );
  const r = await run(business);
  try {
    expect(tool).toBe(1);
    expect(r.snapshot?.state.tool_steps).toBe(2);
    expect(r.snapshot?.state.delegations?.[0]?.tool_used).toBe(1);
    expect(r.snapshot?.state.delegations?.[0]?.model_used).toBe(2);
    expect(r.snapshot?.state.delegations?.[0]?.tool_budget).toBe(1);
  } finally {
    await r.close();
  }
});
it("stops a real parent-child ownership tree and releases an interrupted specialist allocation", async () => {
  const pool = testPool();
  const who = scope();
  let releaseStarted: () => void = () => {};
  const started = new Promise<void>((resolve) => {
    releaseStarted = resolve;
  });
  let models = 0;
  let aborted = false;
  class AbortFixture extends BusinessClient {
    current: import("../src/business/client.js").BusinessDecision | null = null;
    constructor() {
      super("http://127.0.0.1:1", "fixture");
    }
    override async tools() {
      return [];
    }
    override async decisions() {
      return {
        current: this.current,
        authorization_revision: this.current ? 1 : 0,
        barrier: !!this.current,
        terminal: !!this.current,
      };
    }
    override async grant(
      input: Parameters<BusinessClient["grant"]>[0],
    ): Promise<Grant> {
      return {
        ...input,
        grant_id: randomUUID(),
        available_micros: 1000000,
        expires_at: input.lease_until,
        scope: who,
      };
    }
    override async validate(g: Grant) {
      if (this.current) throw new BusinessError(409, "stopped");
      return g;
    }
    override async snapshot() {
      return { revision: 1 };
    }
    override async findCall() {
      return undefined;
    }
    override async receipt() {
      return { accepted: true };
    }
    override async execute(
      ...args: Parameters<BusinessClient["execute"]>
    ): Promise<CallReceipt> {
      models++;
      const [, , g, call, , , ,] = args;
      if (models === 2) {
        releaseStarted();
        const signal = args[6];
        await new Promise<void>((_resolve, reject) => {
          const abort = () => {
            aborted = true;
            reject(new Error("cancelled child model"));
          };
          if (signal?.aborted) abort();
          else signal?.addEventListener("abort", abort, { once: true });
        });
      }
      return {
        receipt_id: randomUUID(),
        root_run_id: g.root_run_id,
        submission_id: g.submission_id,
        call,
        status: "completed",
        cost_micros: 1,
        result: {
          ...answer("委派", [
            {
              id: "child",
              name: "delegate",
              arguments: { role: "script", task: "取消中的专业任务" },
            },
          ]),
          usage: { input_tokens: 1, output_tokens: 1 },
          model_key: "test",
        },
      };
    }
  }
  const fixture = new AbortFixture();
  const service = new SessionService(pool, fixture, { leaseMs: 30000 });
  try {
    const session = await service.createSession({
      ...who,
      request_id: randomUUID(),
      content_hash: "a".repeat(64),
      title: "取消父子",
    });
    const created = await service.createRun(session.id, {
      input: authorizedText("取消树"),
      ...who,
      request_id: randomUUID(),
      content_hash: "b".repeat(64),
      goal: "取消树",
      expected_execution_revision: 0,
      manifest: { model_key: "test" },
    });
    await started;
    fixture.current = {
      ...who,
      decision_id: randomUUID(),
      root_run_id: created.root_run_id,
      session_id: session.id,
      authorization_revision: 1,
      action: "stop",
      content_hash: "c".repeat(64),
    };
    await service.command(created.root_run_id, fixture.current);
    await service.waitForRun(created.root_run_id);
    const snapshot = await service.readRun(created.root_run_id, who);
    expect(snapshot?.state.status).toBe("stopped");
    expect(aborted).toBe(true);
    expect(models).toBe(2);
    expect(snapshot?.state.delegations).toHaveLength(1);
    expect(snapshot?.state.delegations?.[0]?.status).toBe("failed");
    const tasks = await pool.query<{ body: string }>(
      "SELECT body FROM agent.runtime_records WHERE session_id=$1 AND record_type='task'",
      [session.id],
    );
    expect(
      tasks.rows.every((x) => JSON.parse(x.body).state.status === "terminal"),
    ).toBe(true);
  } finally {
    await service.close();
    await pool.end();
  }
});
