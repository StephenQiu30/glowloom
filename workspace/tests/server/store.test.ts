import { afterAll, expect, it } from "vitest";
import { Pool } from "pg";
import { randomUUID } from "node:crypto";
import { PgStore } from "../../src/server/store";
import { buildContext } from "../../src/workflows/definition";
const dsn = process.env.GLOWLOOM_WORKSPACE_TEST_DB_DSN;
if (dsn && !new URL(dsn).pathname.startsWith("/glowloom_workspace_test_"))
  throw new Error("Refusing non-test database");
const pool = dsn ? new Pool({ connectionString: dsn }) : null;
afterAll(async () => {
  await pool?.end();
});
const context = () =>
  buildContext({
    goal: "检查知识入口",
    project: "workspace",
    repoCommit: "abc",
    codeDigest: "code",
    documents: [],
    skillRevisions: {},
    workflowRevision: "flow",
    allowedActions: ["read", "verify"],
    expiresAt: Date.now() + 600000,
    acceptance: ["测试通过"],
  });
it.skipIf(!pool)(
  "persists run idempotency and CAS stage transitions across store instances",
  async () => {
    const store = new PgStore(pool!);
    const key = randomUUID();
    const c = context();
    const first = await store.createRun("local-owner", "feature", c, key);
    expect(first).not.toBeNull();
    const same = await new PgStore(pool!).createRun(
      "local-owner",
      "feature",
      c,
      key,
    );
    expect(same.id).toBe(first.id);
    expect(
      (
        await store.createRun(
          "local-owner",
          "feature",
          { ...c, expiresAt: c.expiresAt + 5000, digest: "refrozen" },
          key,
        )
      ).id,
    ).toBe(first.id);
    const results = await Promise.allSettled([
      store.transition(
        first.id,
        "local-owner",
        1,
        randomUUID(),
        { type: "start" },
        true,
      ),
      store.transition(
        first.id,
        "local-owner",
        1,
        randomUUID(),
        { type: "start" },
        true,
      ),
    ]);
    expect(results.filter((r) => r.status === "fulfilled")).toHaveLength(1);
    expect((await store.getRun(first.id, "local-owner"))?.attempt).toBe(1);
    expect(
      (await store.events(first.id, "local-owner")).map((e) => e.seq),
    ).toEqual([1, 2]);
    await expect(
      store.createRun(
        "local-owner",
        "feature",
        { ...c, goal: "different" },
        key,
      ),
    ).rejects.toThrow(/idempotency/);
  },
);
it.skipIf(!pool)(
  "preserves proposal revisions and returns the original application receipt",
  async () => {
    const store = new PgStore(pool!);
    const proposal = await store.createProposal({
      actor: "local-owner",
      documentId: "design-43",
      baseRevision: "old",
      text: "new text",
      idempotencyKey: randomUUID(),
    });
    expect(proposal.baseRevision).toBe("old");
    const receipt = {
      status: "applied",
      revision: "after",
      receiptId: "receipt-1",
    };
    await store.saveProposalReceipt(proposal.id, "local-owner", receipt);
    await store.saveProposalReceipt(proposal.id, "local-owner", {
      receiptId: "receipt-1",
      status: "applied",
      revision: "after",
    });
    const reloaded = await new PgStore(pool!).getProposal(
      proposal.id,
      "local-owner",
    );
    expect(reloaded?.receipt).toEqual(receipt);
    expect(await store.getProposal(proposal.id, "other-owner")).toBeNull();
  },
);
