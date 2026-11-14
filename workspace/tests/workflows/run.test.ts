import { expect, it } from "vitest";
import { evolveRun, type Run } from "../../src/workflows/run";
import { buildContext } from "../../src/workflows/definition";
const context = buildContext({
  goal: "验证入口",
  project: "workspace",
  repoCommit: "abc",
  codeDigest: "code",
  documents: [],
  skillRevisions: {},
  workflowRevision: "flow",
  allowedActions: ["read", "verify"],
  expiresAt: 10000,
  acceptance: ["测试通过"],
});
const base: Run = {
  id: "run",
  actor: "local-owner",
  workflowId: "feature",
  context,
  status: "created",
  version: 1,
  attempt: 0,
  stage: 0,
  externalHandle: null,
  evidence: [],
  acceptedBy: null,
};
it("does not restart an unknown request or revive a canceled run with late completion", () => {
  expect(() =>
    evolveRun({ ...base, status: "unknown" }, { type: "start" }, 100, true),
  ).toThrow(/unknown/);
  const canceled = evolveRun(
    { ...base, status: "running", attempt: 1 },
    { type: "cancel" },
    100,
    true,
  );
  expect(
    evolveRun(
      canceled,
      { type: "external", handle: "original", result: "finished" },
      100,
      true,
    ).status,
  ).toBe("canceled");
});
it("rejects stale inputs and acceptance without a matching passed check", () => {
  expect(() => evolveRun(base, { type: "start" }, 100, false)).toThrow(/stale/);
  expect(() =>
    evolveRun(
      { ...base, status: "waiting_input" },
      { type: "accept", actor: "local-owner" },
      100,
      true,
    ),
  ).toThrow(/evidence/);
  const checked = evolveRun(
    { ...base, status: "waiting_input" },
    {
      type: "evidence",
      evidence: {
        kind: "command",
        status: "passed",
        inputDigest: context.digest,
        summary: "unit suite",
        artifactRef: "sha256:actual-log",
      },
    },
    100,
    true,
  );
  expect(
    evolveRun(checked, { type: "accept", actor: "local-owner" }, 100, true)
      .acceptedBy,
  ).toBe("local-owner");
});
it("pauses new dispatch while preserving the original attempt and handle", () => {
  const paused = evolveRun(
    { ...base, status: "running", attempt: 1, externalHandle: "thread:turn" },
    { type: "pause" },
    100,
    true,
  );
  expect(paused.status).toBe("paused");
  expect(paused.externalHandle).toBe("thread:turn");
  expect(() => evolveRun(paused, { type: "start" }, 100, true)).toThrow();
});
it("binds the original external handle without dispatching a second attempt", () => {
  const running = evolveRun(base, { type: "start" }, 100, true);
  const bound = evolveRun(
    running,
    { type: "bind", handle: "original-thread:turn" },
    100,
    true,
  );
  expect(bound.externalHandle).toBe("original-thread:turn");
  expect(bound.attempt).toBe(1);
  expect(bound.status).toBe("waiting_external");
});
it("requires fresh verification after an explicit context refresh", () => {
  const changed = { ...context, digest: "new-digest", codeDigest: "new-code" };
  const old = {
    ...base,
    status: "waiting_input" as const,
    acceptedBy: "local-owner",
    evidence: [
      {
        kind: "command" as const,
        status: "passed" as const,
        inputDigest: context.digest,
        summary: "old test",
        artifactRef: "old-log",
      },
    ],
  };
  const refreshed = evolveRun(
    old,
    { type: "refresh", context: changed },
    100,
    true,
  );
  expect(refreshed.acceptedBy).toBeNull();
  expect(refreshed.evidence[0].inputDigest).toBe(context.digest);
  expect(() =>
    evolveRun(refreshed, { type: "accept", actor: "local-owner" }, 100, true),
  ).toThrow(/evidence/);
});
it("allows explicit reacceptance after a writeback changes frozen inputs", () => {
  const c = { ...context, stageIds: ["validate", "accept", "writeback"] };
  let run: Run = {
    ...base,
    context: c,
    stage: 2,
    status: "waiting_input",
    acceptedBy: "local-owner",
  };
  run = evolveRun(
    run,
    { type: "refresh", context: { ...c, digest: "fresh" } },
    100,
    true,
  );
  run = evolveRun(
    run,
    {
      type: "evidence",
      evidence: {
        kind: "command",
        status: "passed",
        inputDigest: "fresh",
        summary: "fresh check",
        artifactRef: "log",
      },
    },
    100,
    true,
  );
  run = evolveRun(run, { type: "accept", actor: "local-owner" }, 100, true);
  expect(evolveRun(run, { type: "advance", stages: 3 }, 100, true).status).toBe(
    "succeeded",
  );
});
it("clears only a definitely unstarted check without accepting it", () => {
  const pending = {
    ...base,
    status: "waiting_external" as const,
    pendingCheck: { key: "k", commandId: "typo" },
  };
  const next = evolveRun(
    pending,
    { type: "checkRejected" } as never,
    100,
    true,
  );
  expect(next.pendingCheck).toBeNull();
  expect(next.status).toBe("waiting_input");
  expect(next.evidence).toHaveLength(0);
});
