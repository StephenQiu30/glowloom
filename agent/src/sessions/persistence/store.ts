import { RecoveryStore } from '../recovery.js';
import { publicState } from '../../pi-durable/events.js';
import type { Pool, PoolClient } from 'pg';
import { randomUUID } from 'node:crypto';
import type {
  CreateSessionInput,
  CreateRunInput,
  ControlInput,
  Identity,
  Scope,
  AuthorizedInput,
} from '../types.js';
import { AgentError } from '../errors.js';
import {
  canonicalHash,
  stableId,
  hasCurrentDefinitions,
  type FrozenManifest,
} from '../manifest.js';
import type { AgentState } from '../../pi-durable/docs.js';
import { ScopedCursors } from './pagination.js';
import { readMessagePage } from './message-history.js';
export interface SessionRow extends Scope {
  id: string;
  title: string;
  pinned: boolean;
  archived_at: Date | null;
  execution_revision: string;
  created_at: Date;
}
export interface RunRow {
  id: string;
  session_id: string;
  request_id: string;
  content_hash: string;
  goal: string;
  manifest: FrozenManifest;
  submission_id: string;
  accepted_revision: string;
  created_at: Date;
  retry_of_run_id?: string;
  input?: AuthorizedInput;
}
export async function inTransaction<T>(
  pool: Pool,
  work: (client: PoolClient) => Promise<T>,
  readOnly = false,
) {
  const client = await pool.connect();
  let committed = false;
  try {
    await client.query(
      readOnly ? 'BEGIN ISOLATION LEVEL REPEATABLE READ READ ONLY' : 'BEGIN',
    );
    const result = await work(client);
    await client.query('COMMIT');
    committed = true;
    return result;
  } finally {
    if (!committed) await client.query('ROLLBACK').catch(() => {});
    client.release();
  }
}
export class SessionStore {
  readonly cursors = new ScopedCursors();
  constructor(readonly pool: Pool) {}
  async createSession(input: CreateSessionInput) {
    const hash = canonicalHash(input);
    const id = randomUUID();
    const result = await this.pool.query<SessionRow & { content_hash: string }>(
      `INSERT INTO agent.service_sessions(id,actor_id,org_id,scope_id,project_id,canvas_id,title,request_id,content_hash)
   VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9) ON CONFLICT(actor_id,project_id,request_id) DO UPDATE SET request_id=EXCLUDED.request_id RETURNING *`,
      [
        id,
        input.actor_id,
        input.org_id,
        input.scope_id,
        input.project_id,
        input.canvas_id ?? null,
        input.title,
        input.request_id,
        hash,
      ],
    );
    const row = result.rows[0]!;
    if (row.content_hash !== hash)
      throw new AgentError('request_identity_conflict');
    if (row.archived_at) throw new AgentError('session_not_found');
    return this.sessionDTO(row);
  }
  sessionDTO(row: SessionRow) {
    return {
      id: row.id,
      actor_id: row.actor_id,
      org_id: row.org_id,
      scope_id: row.scope_id,
      project_id: row.project_id,
      title: row.title,
      pinned: row.pinned,
      created_at: row.created_at,
      ...(row.canvas_id
        ? { canvas_id: row.canvas_id }
        : { canvas_id: undefined }),
      execution_revision: Number(row.execution_revision),
    };
  }
  async session(
    id: string,
    who: Identity,
    client: Pool | PoolClient = this.pool,
  ) {
    return (
      await client.query<SessionRow>(
        'SELECT * FROM agent.service_sessions WHERE id=$1 AND actor_id=$2 AND project_id=$3 AND archived_at IS NULL',
        [id, who.actor_id, who.project_id],
      )
    ).rows[0];
  }
  async list(who: Identity) {
    return (
      await this.pool.query<SessionRow>(
        'SELECT * FROM agent.service_sessions WHERE actor_id=$1 AND project_id=$2 AND archived_at IS NULL ORDER BY created_at DESC,id DESC LIMIT 100',
        [who.actor_id, who.project_id],
      )
    ).rows.map((r) => this.sessionDTO(r));
  }
  async admitRun(
    id: string,
    input: CreateRunInput,
    manifest: Readonly<FrozenManifest>,
  ) {
    return inTransaction(this.pool, async (client) => {
      const session = (
        await client.query<SessionRow>(
          'SELECT * FROM agent.service_sessions WHERE id=$1 AND actor_id=$2 AND project_id=$3 AND archived_at IS NULL FOR UPDATE',
          [id, input.actor_id, input.project_id],
        )
      ).rows[0];
      if (!session) throw new AgentError('session_not_found');
      const hash = canonicalHash(input);
      const prior = (
        await client.query<RunRow>(
          'SELECT * FROM agent.service_runs WHERE session_id=$1 AND request_id=$2',
          [id, input.request_id],
        )
      ).rows[0];
      if (prior) {
        if (prior.content_hash !== hash)
          throw new AgentError('request_identity_conflict');
        return prior;
      }
      const last = (
        await client.query<RunRow>(
          'SELECT * FROM agent.service_runs WHERE session_id=$1 ORDER BY accepted_revision DESC LIMIT 1',
          [id],
        )
      ).rows[0];
      const view = last ? await this.snapshot(last, client) : undefined;
      if (
        last &&
        (!view ||
          !['completed', 'failed', 'stopped', 'rejected'].includes(
            String(view.state?.status),
          ))
      )
        throw new AgentError('session_has_active_run');
      const revision = Math.max(
        Number(session.execution_revision),
        Number(view?.state?.execution_revision ?? 0),
      );
      if (revision !== input.expected_execution_revision)
        throw new AgentError('execution_revision_conflict');
      if (input.retry_of_run_id) {
        const original = await this.run(input.retry_of_run_id, input, client);
        if (!original || original.session_id !== id)
          throw new AgentError('run_not_found');
        const previous = await this.snapshot(original, client);
        if (
          !original.input ||
          original.input.input_hash !== input.input?.input_hash ||
          canonicalHash(original.manifest) !== canonicalHash(manifest) ||
          !['completed', 'failed', 'stopped', 'rejected'].includes(
            previous.state.status,
          )
        )
          throw new AgentError('retry_input_changed');
      }
      const run = stableId(id, 'run:' + input.request_id);
      const submission = stableId(run, 'submission:initial');
      const row = (
        await client.query<RunRow>(
          'INSERT INTO agent.service_runs(id,session_id,request_id,content_hash,goal,manifest,submission_id,accepted_revision,input,retry_of_run_id) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10) RETURNING *',
          [
            run,
            id,
            input.request_id,
            hash,
            input.goal,
            manifest,
            submission,
            revision + 1,
            input.input ?? null,
            input.retry_of_run_id ?? null,
          ],
        )
      ).rows[0]!;
      await client.query(
        'UPDATE agent.service_sessions SET execution_revision=$2,last_input_at=clock_timestamp() WHERE id=$1',
        [id, revision + 1],
      );
      return row;
    });
  }
  async run(id: string, who: Identity, client: Pool | PoolClient = this.pool) {
    return (
      await client.query<RunRow>(
        'SELECT r.* FROM agent.service_runs r JOIN agent.service_sessions s ON s.id=r.session_id WHERE r.id=$1 AND s.actor_id=$2 AND s.project_id=$3 AND s.archived_at IS NULL',
        [id, who.actor_id, who.project_id],
      )
    ).rows[0];
  }
  async snapshot(run: RunRow, client: Pool | PoolClient = this.pool) {
    const rows = await client.query<{
      event_type: string;
      payload: Partial<AgentState> &
        Pick<AgentState, 'status' | 'execution_revision'>;
      commit_seq: string;
      event_ordinal: number;
    }>(
      "SELECT event_type,payload,commit_seq,event_ordinal FROM agent.runtime_events WHERE session_id=$1 AND payload->>'root_run_id'=$2 AND event_type='agent.state' ORDER BY commit_seq DESC,event_ordinal DESC LIMIT 1",
      [run.session_id, run.id],
    );
    const cursor = (
      await client.query<{ commit_seq: string; event_ordinal: number }>(
        'SELECT commit_seq,event_ordinal FROM agent.runtime_events WHERE session_id=$1 ORDER BY commit_seq DESC,event_ordinal DESC LIMIT 1',
        [run.session_id],
      )
    ).rows[0];
    const recovery = await new RecoveryStore(client).read(run.id);
    const base = {
      recovery,
      execution_available: hasCurrentDefinitions(run.manifest) && !!run.input,
      history_incomplete: !run.input,
      root_run_id: run.id,
      session_id: run.session_id,
      submission_id: run.submission_id,
      goal: run.goal,
      ...(run.input ? { input: run.input } : {}),
      ...(run.retry_of_run_id ? { retry_of_run_id: run.retry_of_run_id } : {}),
      manifest: run.manifest,
      state: rows.rows[0]
        ? (publicState(
            rows.rows[0]
              .payload as import('@earendil-works/pi-durable').JsonObject,
          ) as (typeof rows.rows)[0]['payload'])
        : {
            status: 'accepted',
            execution_revision: Number(run.accepted_revision),
          },
      snapshot_cursor: cursor
        ? `${cursor.commit_seq}:${cursor.event_ordinal}`
        : '0:0',
    };
    const who = (
      await client.query<{ actor_id: string; project_id: string }>(
        'SELECT actor_id,project_id FROM agent.service_sessions WHERE id=$1',
        [run.session_id],
      )
    ).rows[0];
    if (!who) throw new AgentError('session_not_found');
    const messages_page = await readMessagePage(client, this.cursors, run, {
      ...who,
      through: base.snapshot_cursor,
    });
    const baseBytes = Buffer.byteLength(
      JSON.stringify({ ...base, messages_page, events: [] }),
    );
    const budget =
      Math.min(
        8 * 1024 * 1024,
        Math.max(4 * 1024 * 1024, baseBytes + 1024 * 1024),
      ) -
      65536 -
      baseBytes;
    if (budget < 0) throw new AgentError('snapshot_state_exceeds_limit');
    // Bound bytes before transferring JSON from PostgreSQL; preserve the same current cursor/state.
    const events = await client.query<{ event_type: string; payload: unknown }>(
      `WITH recent AS (
        SELECT event_type,payload,commit_seq,event_ordinal FROM agent.runtime_events
        WHERE session_id=$1 AND payload->>'root_run_id'=$2 AND event_type<>'agent.state'
        ORDER BY commit_seq DESC,event_ordinal DESC LIMIT 100
      ), bounded AS (
        SELECT *,sum(octet_length(payload::text)+octet_length(event_type)+64)
          OVER (ORDER BY commit_seq DESC,event_ordinal DESC) AS bytes FROM recent
      ) SELECT event_type,payload FROM bounded WHERE bytes<=$3 ORDER BY commit_seq DESC,event_ordinal DESC`,
      [run.session_id, run.id, budget],
    );
    return { ...base, messages_page, events: events.rows.reverse() };
  }

  async acceptControl(input: ControlInput) {
    const hash = canonicalHash(input);
    const row = (
      await this.pool.query<{ content_hash: string; receipt: unknown }>(
        'INSERT INTO agent.service_control_inbox(decision_id,root_run_id,authorization_revision,content_hash,input) VALUES($1,$2,$3,$4,$5) ON CONFLICT(decision_id) DO UPDATE SET decision_id=EXCLUDED.decision_id RETURNING content_hash,receipt',
        [
          input.decision_id,
          input.root_run_id,
          input.authorization_revision,
          hash,
          input,
        ],
      )
    ).rows[0]!;
    if (row.content_hash !== hash)
      throw new AgentError('decision_identity_conflict');
    return row.receipt;
  }
}
