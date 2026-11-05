import { randomUUID } from "node:crypto";
import { expect, it } from "vitest";
import { BACKGROUND_CONTEXT as context } from "@earendil-works/chord/context";
import { Harness, ROOT_CONVERSATION_ID } from "@earendil-works/pi-durable";
import {
  BusinessClient,
  type Grant,
  type CallReceipt,
} from "../src/business/client.js";
import { buildRegistry, type HarnessGate } from "../src/harness/registry.js";
import { StateDoc } from "../src/harness/docs.js";
import { createBusinessModels } from "../src/harness/model-provider.js";
import { PhaseTelemetry } from "../src/harness/telemetry.js";
import { freezeManifest } from "../src/sessions/manifest.js";
import {
  claimSessionOwner,
  releaseSessionOwner,
} from "../src/storage/ownership.js";
import { openPostgresStorage } from "../src/storage/postgres.js";
import { testPool } from "./storage/database.js";

it("uses actual official CompactionTask through the business model gate then reinjects typed goal, manifest, Skill, plan and adoption constraints", async () => {
  const pool = testPool();
  const session = randomUUID();
  const owner = await claimSessionOwner(pool, session, randomUUID(), 30000);
  let harness: Harness | undefined;
  let calls = 0;
  let last: Record<string, unknown> | undefined;
  const grant: Grant = {
    grant_id: randomUUID(),
    root_run_id: randomUUID(),
    submission_id: randomUUID(),
    owner_epoch: Number(owner.epoch),
    authorization_revision: 0,
    available_micros: 1000000,
    expires_at: owner.leaseUntil.toISOString(),
    scope: {
      actor_id: randomUUID(),
      org_id: randomUUID(),
      scope_id: randomUUID(),
      project_id: randomUUID(),
    },
  };
  class Fixture extends BusinessClient {
    constructor() {
      super("http://127.0.0.1:1", "fixture");
    }
    override async validate(value: Grant) {
      return value;
    }
    override async snapshot() {
      return { revision: 1 };
    }
    override async findCall() {
      return undefined;
    }
    override async skill() {
      return {
        skill_id: "script",
        version: "v1",
        resource_hash: "hash",
        content: "Skill constraint retained: cite real sources.",
      };
    }
    override async execute(
      ...args: Parameters<BusinessClient["execute"]>
    ): Promise<CallReceipt> {
      calls++;
      const [, , g, call, input] = args;
      last = input as Record<string, unknown>;
      return {
        receipt_id: randomUUID(),
        root_run_id: g.root_run_id,
        submission_id: g.submission_id,
        call,
        status: "completed",
        cost_micros: 1,
        result: {
          output: "Official model response " + calls,
          usage: { input_tokens: 1, output_tokens: 1 },
          stop_reason: "stop",
          model_key: "test",
        },
      };
    }
  }
  const business = new Fixture();
  const manifest = freezeManifest(
    {
      model_key: "test",
      skills: [{ skill_id: "script", version: "v1", resource_hash: "hash" }],
    },
    [],
  );
  const gate: HarnessGate = {
    manifest,
    harness: () => harness!,
    grant: () => grant,
    rootId: () => ROOT_CONVERSATION_ID,
    assertSources: async () => {},
    assertActive: async () => {
      await business.validate(grant);
    },
    proposal: async () => {
      throw new Error("unexpected proposal");
    },
    reserve: async (call, kind) => {
      await harness!.commit(async (tx) => {
        const s = await tx.doc(StateDoc, ROOT_CONVERSATION_ID);
        s[kind === "model" ? "model_steps" : "tool_steps"]++;
        s.reserved_calls.push({ call_id: call.call_id, task_id: call.task_id });
      }, context);
    },
  };
  try {
    const registry = buildRegistry(business, gate);
    const { models } = createBusinessModels(business, manifest, gate);
    harness = await Harness.open(
      await openPostgresStorage(pool, session, owner),
      {
        models,
        registry: registry.registry,
        telemetry: new PhaseTelemetry(),
        settings: {
          retry: { maxRetries: 0 },
          compaction: {
            enabled: true,
            backgroundTokens: 0,
            keepRecentTokens: 32,
            reserveTokens: 512,
          },
        },
      },
      context,
    );
    const root = await harness.root(context, {
      agent: {
        model: { provider: "glowloom", modelId: "test" },
        extensions: registry.rootExtensions,
        tools: registry.rootTools,
      },
    });
    await harness.commit(async (tx) => {
      const s = await tx.doc(StateDoc, root.id);
      Object.assign(s, {
        root_run_id: grant.root_run_id,
        segment_id: grant.submission_id,
        status: "running",
        goal: "Permanent creative goal",
        manifest_hash: manifest.sha256,
        skills: manifest.skills,
        plan: [{ id: "p1", title: "Keep this plan" }],
        adopted: [
          { delegation_id: "adopted-task", source_refs: ["asset:real-source"] },
        ],
        loaded_skills: [
          {
            skill_id: "script",
            version: "v1",
            resource_hash: "hash",
            content: "Skill constraint retained: cite real sources.",
          },
        ],
      });
    }, context);
    await (
      await root.submit(
        {
          type: "input",
          content: "Old source history ".repeat(300),
          requestId: randomUUID(),
        },
        context,
      )
    ).wait(context);
    await (
      await root.submit(
        {
          type: "input",
          content: "Recent source ".repeat(80),
          requestId: randomUUID(),
        },
        context,
      )
    ).wait(context);
    const before = calls;
    const task = await root.compact(undefined, context);
    const settled = await harness.waitForTask(task, context);
    await root.waitForIdle(context);
    expect(settled.state.outcome.status).toBe("completed");
    expect(calls).toBe(before + 1);
    const persisted = await harness.getTask(task, context);
    expect(persisted?.kind).toBe("pi.compaction");
    expect(settled.state.outcome.result).toBeDefined();
    await (
      await root.submit(
        {
          type: "input",
          content: "Continue after compaction",
          requestId: randomUUID(),
        },
        context,
      )
    ).wait(context);
    for (const retained of [
      "Permanent creative goal",
      manifest.sha256,
      "Keep this plan",
      "asset:real-source",
      "Skill constraint retained",
      "Only explicitly adopted",
    ])
      expect(JSON.stringify(last)).toContain(retained);
    expect(
      (await harness.snapshot(StateDoc, root.id, context))?.model_steps,
    ).toBe(calls);
  } finally {
    await harness?.close(context);
    await releaseSessionOwner(pool, session, owner);
    await pool.end();
  }
});
