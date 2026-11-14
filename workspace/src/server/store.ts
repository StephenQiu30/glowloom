import type { Pool, PoolClient } from "pg";
import { isDeepStrictEqual } from "node:util";
import { randomUUID } from "node:crypto";
import { digest } from "../content/parse";
import { evolveRun, type Run, type Action } from "../workflows/run";
import type { TaskContext } from "../workflows/definition";
export type Proposal = {
  id: string;
  actor: string;
  documentId: string;
  baseRevision: string;
  text: string;
  baseText?: string;
  receipt: Record<string, unknown> | null;
};
export class PgStore {
  constructor(private pool: Pool) {}
  private async transaction<T>(action: (c: PoolClient) => Promise<T>) {
    const c = await this.pool.connect();
    try {
      await c.query("BEGIN");
      const result = await action(c);
      await c.query("COMMIT");
      return result;
    } catch (error) {
      await c.query("ROLLBACK");
      throw error;
    } finally {
      c.release();
    }
  }
  async health() {
    const result = await this.pool.query(
      "SELECT has_schema_privilege(current_user,'dev_workspace','USAGE') AS own,has_schema_privilege(current_user,'workspace','USAGE') AS business",
    );
    if (!result.rows[0]?.own || result.rows[0]?.business)
      throw new Error("restricted_database_role_required");
    return true;
  }
  async createRun(
    actor: string,
    workflow: string,
    context: TaskContext,
    key: string,
  ): Promise<Run> {
    const semanticContext = Object.fromEntries(
      Object.entries(context).filter(
        ([key]) => !["expiresAt", "digest"].includes(key),
      ),
    );
    const inputDigest = digest(
      JSON.stringify([actor, workflow, semanticContext]),
    );
    return this.transaction(async (c) => {
      const run: Run = {
        id: randomUUID(),
        actor,
        workflowId: workflow,
        context,
        status: "created",
        version: 1,
        attempt: 0,
        stage: 0,
        externalHandle: null,
        evidence: [],
        acceptedBy: null,
      };
      const inserted = await c.query(
        "INSERT INTO dev_workspace.workflow_run(id,actor,idempotency_key,input_digest,record) VALUES($1,$2,$3,$4,$5) ON CONFLICT(actor,idempotency_key) DO NOTHING RETURNING record",
        [run.id, actor, key, inputDigest, run],
      );
      if (!inserted.rowCount) {
        const existing = await c.query(
          "SELECT record,input_digest FROM dev_workspace.workflow_run WHERE actor=$1 AND idempotency_key=$2",
          [actor, key],
        );
        if (existing.rows[0]?.input_digest !== inputDigest)
          throw new Error("idempotency_conflict");
        return existing.rows[0].record as Run;
      }
      await c.query(
        "INSERT INTO dev_workspace.run_event(run_id,seq,event_key,kind,payload) VALUES($1,1,$2,'created',$3)",
        [run.id, key, { inputDigest: context.digest }],
      );
      await c.query(
        "INSERT INTO dev_workspace.artifact_ref(id,run_id,actor,kind,source_revision,checksum,manifest) VALUES($1,$2,$3,'task-context',$4,$5,$6)",
        [
          randomUUID(),
          run.id,
          actor,
          context.digest,
          digest(JSON.stringify(context)),
          { context },
        ],
      );
      return run;
    });
  }
  async getRun(id: string, actor: string): Promise<Run | null> {
    const result = await this.pool.query(
      "SELECT record FROM dev_workspace.workflow_run WHERE id=$1 AND actor=$2",
      [id, actor],
    );
    return result.rows[0]?.record ?? null;
  }
  async listRuns(actor: string): Promise<Run[]> {
    return (
      await this.pool.query(
        "SELECT record FROM dev_workspace.workflow_run WHERE actor=$1 ORDER BY created_at DESC LIMIT 100",
        [actor],
      )
    ).rows.map((r) => r.record);
  }
  async transition(
    id: string,
    actor: string,
    version: number,
    key: string,
    action: Action,
    current: boolean,
  ): Promise<{ run: Run; replayed: boolean }> {
    return this.transaction(async (c) => {
      const row = (
        await c.query(
          "SELECT record FROM dev_workspace.workflow_run WHERE id=$1 AND actor=$2 FOR UPDATE",
          [id, actor],
        )
      ).rows[0];
      if (!row) throw new Error("run_not_found");
      const run = row.record as Run;
      const event = await c.query(
        "SELECT payload FROM dev_workspace.run_event WHERE run_id=$1 AND event_key=$2",
        [id, key],
      );
      const actionDigest = digest(JSON.stringify(action));
      if (event.rowCount) {
        if (event.rows[0].payload.actionDigest !== actionDigest)
          throw new Error("idempotency_conflict");
        return { run, replayed: true };
      }
      if (run.version !== version) throw new Error("version_conflict");
      const next = evolveRun(run, action, Date.now(), current);
      if (action.type === "bind" || action.type === "external") {
        const binding = await c.query(
          "SELECT external_handle FROM dev_workspace.executor_binding WHERE run_id=$1 AND attempt=$2",
          [run.id, run.attempt],
        );
        if (
          binding.rows[0] &&
          binding.rows[0].external_handle !== action.handle
        )
          throw new Error("handle_conflict");
        await c.query(
          "INSERT INTO dev_workspace.executor_binding(run_id,attempt,executor,external_handle) VALUES($1,$2,'codex',$3) ON CONFLICT DO NOTHING",
          [run.id, run.attempt, action.handle],
        );
      }
      if (action.type === "refresh")
        await c.query(
          "INSERT INTO dev_workspace.artifact_ref(id,run_id,actor,kind,source_revision,checksum,manifest) VALUES($1,$2,$3,'task-context',$4,$5,$6)",
          [
            randomUUID(),
            run.id,
            actor,
            next.context.digest,
            digest(JSON.stringify(next.context)),
            { context: next.context },
          ],
        );
      await c.query(
        "UPDATE dev_workspace.workflow_run SET record=$1,updated_at=now() WHERE id=$2",
        [next, id],
      );
      await c.query(
        "INSERT INTO dev_workspace.run_event(run_id,seq,event_key,kind,payload) SELECT $1,COALESCE(MAX(seq),0)+1,$2,$3,$4 FROM dev_workspace.run_event WHERE run_id=$1",
        [
          id,
          key,
          action.type,
          {
            actionDigest,
            version: next.version,
            status: next.status,
            inputDigest: next.context.digest,
          },
        ],
      );
      return { run: next, replayed: false };
    });
  }
  async events(
    id: string,
    actor: string,
  ): Promise<
    { seq: number; kind: string; payload: Record<string, unknown> }[]
  > {
    return (
      await this.pool.query(
        "SELECT e.seq::int,e.kind,e.payload FROM dev_workspace.run_event e JOIN dev_workspace.workflow_run r ON r.id=e.run_id WHERE r.id=$1 AND r.actor=$2 ORDER BY e.seq",
        [id, actor],
      )
    ).rows;
  }
  async createProposal(input: {
    actor: string;
    documentId: string;
    baseRevision: string;
    text: string;
    baseText?: string;
    idempotencyKey: string;
  }): Promise<Proposal> {
    if (
      Buffer.byteLength(input.text) + Buffer.byteLength(input.baseText ?? "") >
      2_000_000
    )
      throw new Error("proposal_too_large");
    if (
      input.baseText !== undefined &&
      digest(input.baseText) !== input.baseRevision
    )
      throw new Error("proposal_baseline_mismatch");
    const inputDigest = digest(
      JSON.stringify([
        input.documentId,
        input.baseRevision,
        input.text,
        input.baseText,
      ]),
    );
    return this.transaction(async (c) => {
      const proposal: Proposal = {
        id: randomUUID(),
        actor: input.actor,
        documentId: input.documentId,
        baseRevision: input.baseRevision,
        text: input.text,
        baseText: input.baseText,
        receipt: null,
      };
      const inserted = await c.query(
        "INSERT INTO dev_workspace.content_proposal(id,actor,idempotency_key,input_digest,record) VALUES($1,$2,$3,$4,$5) ON CONFLICT(actor,idempotency_key) DO NOTHING RETURNING record",
        [proposal.id, input.actor, input.idempotencyKey, inputDigest, proposal],
      );
      if (inserted.rowCount) return proposal;
      const previous = (
        await c.query(
          "SELECT record,input_digest FROM dev_workspace.content_proposal WHERE actor=$1 AND idempotency_key=$2",
          [input.actor, input.idempotencyKey],
        )
      ).rows[0];
      if (previous.input_digest !== inputDigest)
        throw new Error("idempotency_conflict");
      return previous.record;
    });
  }
  async saveProposalReceipt(
    id: string,
    actor: string,
    receipt: Record<string, unknown>,
  ): Promise<void> {
    await this.transaction(async (c) => {
      const row = (
        await c.query(
          "SELECT record FROM dev_workspace.content_proposal WHERE id=$1 AND actor=$2 FOR UPDATE",
          [id, actor],
        )
      ).rows[0];
      if (!row) throw new Error("proposal_not_found");
      const proposal = row.record as Proposal;
      if (proposal.receipt && !isDeepStrictEqual(proposal.receipt, receipt))
        throw new Error("receipt_conflict");
      proposal.receipt = receipt;
      await c.query(
        "UPDATE dev_workspace.content_proposal SET record=$1,updated_at=now() WHERE id=$2",
        [proposal, id],
      );
    });
  }
  async getProposal(id: string, actor: string): Promise<Proposal | null> {
    return (
      (
        await this.pool.query(
          "SELECT record FROM dev_workspace.content_proposal WHERE id=$1 AND actor=$2",
          [id, actor],
        )
      ).rows[0]?.record ?? null
    );
  }
  async listProposals(actor: string): Promise<Proposal[]> {
    return (
      await this.pool.query(
        "SELECT record FROM dev_workspace.content_proposal WHERE actor=$1 ORDER BY created_at DESC LIMIT 100",
        [actor],
      )
    ).rows.map((r) => r.record);
  }
  async recordArtifact(
    run: Run,
    evidence: import("../workflows/run").Evidence,
  ) {
    await this.pool.query(
      "INSERT INTO dev_workspace.artifact_ref(id,run_id,actor,kind,source_revision,checksum,manifest) VALUES($1,$2,$3,$4,$5,$6,$7)",
      [
        randomUUID(),
        run.id,
        run.actor,
        evidence.kind,
        run.context.digest,
        evidence.artifactRef.replace(/^sha256:/, ""),
        evidence,
      ],
    );
  }
}
