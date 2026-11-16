import { RecoveryStore } from '../recovery.js';
import { publicState } from '../../pi-durable/events.js';
import type { Pool } from 'pg';
import {
  and,
  desc,
  eq,
  getTableColumns,
  isNull,
  lte,
  ne,
  sql,
} from 'drizzle-orm';
import {
  database,
  withTransaction,
  type AgentTransaction,
  type DatabaseConnection,
} from '../../database/connection.js';
import {
  runtimeEvents,
  serviceControlInbox,
  serviceRuns,
  serviceSessions,
} from '../../database/schema.js';
import { randomUUID } from 'node:crypto';
import type {
  CreateSessionInput,
  CreateRunInput,
  ControlInput,
  Identity,
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
export type SessionRow = typeof serviceSessions.$inferSelect;
export type RunRow = typeof serviceRuns.$inferSelect;
export async function inTransaction<T>(
  pool: Pool,
  work: (client: AgentTransaction) => Promise<T>,
  readOnly = false,
) {
  return withTransaction(pool, work, readOnly);
}
export class SessionStore {
  readonly cursors = new ScopedCursors();
  constructor(readonly pool: Pool) {}
  async createSession(input: CreateSessionInput) {
    const hash = canonicalHash(input);
    const id = randomUUID();
    const [row] = await database(this.pool)
      .insert(serviceSessions)
      .values({
        id,
        actor_id: input.actor_id,
        org_id: input.org_id,
        scope_id: input.scope_id,
        project_id: input.project_id,
        canvas_id: input.canvas_id ?? null,
        title: input.title,
        request_id: input.request_id,
        content_hash: hash,
      })
      .onConflictDoUpdate({
        target: [
          serviceSessions.actor_id,
          serviceSessions.project_id,
          serviceSessions.request_id,
        ],
        set: { request_id: input.request_id },
      })
      .returning();
    if (!row) throw new Error('Session insert returned no row');
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
    client: DatabaseConnection = this.pool,
  ) {
    const [row] = await database(client)
      .select()
      .from(serviceSessions)
      .where(
        and(
          eq(serviceSessions.id, id),
          eq(serviceSessions.actor_id, who.actor_id),
          eq(serviceSessions.project_id, who.project_id),
          isNull(serviceSessions.archived_at),
        ),
      );
    return row;
  }
  async list(who: Identity) {
    const rows = await database(this.pool)
      .select()
      .from(serviceSessions)
      .where(
        and(
          eq(serviceSessions.actor_id, who.actor_id),
          eq(serviceSessions.project_id, who.project_id),
          isNull(serviceSessions.archived_at),
        ),
      )
      .orderBy(desc(serviceSessions.created_at), desc(serviceSessions.id))
      .limit(100);
    return rows.map((r) => this.sessionDTO(r));
  }
  async admitRun(
    id: string,
    input: CreateRunInput,
    manifest: Readonly<FrozenManifest>,
  ) {
    return inTransaction(this.pool, async (client) => {
      const db = database(client);
      const [session] = await db
        .select()
        .from(serviceSessions)
        .where(
          and(
            eq(serviceSessions.id, id),
            eq(serviceSessions.actor_id, input.actor_id),
            eq(serviceSessions.project_id, input.project_id),
            isNull(serviceSessions.archived_at),
          ),
        )
        .for('update');
      if (!session) throw new AgentError('session_not_found');
      const hash = canonicalHash(input);
      const [prior] = await db
        .select()
        .from(serviceRuns)
        .where(
          and(
            eq(serviceRuns.session_id, id),
            eq(serviceRuns.request_id, input.request_id),
          ),
        );
      if (prior) {
        if (prior.content_hash !== hash)
          throw new AgentError('request_identity_conflict');
        return prior;
      }
      const [last] = await db
        .select()
        .from(serviceRuns)
        .where(eq(serviceRuns.session_id, id))
        .orderBy(desc(serviceRuns.accepted_revision))
        .limit(1);
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
      const [row] = await db
        .insert(serviceRuns)
        .values({
          id: run,
          session_id: id,
          request_id: input.request_id,
          content_hash: hash,
          goal: input.goal,
          manifest,
          submission_id: submission,
          accepted_revision: String(revision + 1),
          input: input.input ?? null,
          retry_of_run_id: input.retry_of_run_id ?? null,
        })
        .returning();
      if (!row) throw new Error('Run insert returned no row');
      await db
        .update(serviceSessions)
        .set({
          execution_revision: String(revision + 1),
          last_input_at: sql`clock_timestamp()`,
        })
        .where(eq(serviceSessions.id, id));
      return row;
    });
  }
  async run(id: string, who: Identity, client: DatabaseConnection = this.pool) {
    const [row] = await database(client)
      .select(getTableColumns(serviceRuns))
      .from(serviceRuns)
      .innerJoin(
        serviceSessions,
        eq(serviceSessions.id, serviceRuns.session_id),
      )
      .where(
        and(
          eq(serviceRuns.id, id),
          eq(serviceSessions.actor_id, who.actor_id),
          eq(serviceSessions.project_id, who.project_id),
          isNull(serviceSessions.archived_at),
        ),
      );
    return row;
  }
  async snapshot(run: RunRow, client: DatabaseConnection = this.pool) {
    const db = database(client);
    const [state] = await db
      .select({ payload: runtimeEvents.payload })
      .from(runtimeEvents)
      .where(
        and(
          eq(runtimeEvents.session_id, run.session_id),
          eq(sql<string>`${runtimeEvents.payload}->>'root_run_id'`, run.id),
          eq(runtimeEvents.event_type, 'agent.state'),
        ),
      )
      .orderBy(
        desc(runtimeEvents.commit_seq),
        desc(runtimeEvents.event_ordinal),
      )
      .limit(1);
    const [cursor] = await db
      .select({
        commit_seq: runtimeEvents.commit_seq,
        event_ordinal: runtimeEvents.event_ordinal,
      })
      .from(runtimeEvents)
      .where(eq(runtimeEvents.session_id, run.session_id))
      .orderBy(
        desc(runtimeEvents.commit_seq),
        desc(runtimeEvents.event_ordinal),
      )
      .limit(1);
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
      state: state
        ? (publicState(state.payload) as Partial<AgentState> &
            Pick<AgentState, 'status' | 'execution_revision'>)
        : {
            status: 'accepted',
            execution_revision: Number(run.accepted_revision),
          },
      snapshot_cursor: cursor
        ? `${cursor.commit_seq}:${cursor.event_ordinal}`
        : '0:0',
    };
    const [who] = await db
      .select({
        actor_id: serviceSessions.actor_id,
        project_id: serviceSessions.project_id,
      })
      .from(serviceSessions)
      .where(eq(serviceSessions.id, run.session_id));
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
    const recent = db.$with('recent').as(
      db
        .select({
          event_type: runtimeEvents.event_type,
          payload: runtimeEvents.payload,
          commit_seq: runtimeEvents.commit_seq,
          event_ordinal: runtimeEvents.event_ordinal,
        })
        .from(runtimeEvents)
        .where(
          and(
            eq(runtimeEvents.session_id, run.session_id),
            eq(sql<string>`${runtimeEvents.payload}->>'root_run_id'`, run.id),
            ne(runtimeEvents.event_type, 'agent.state'),
          ),
        )
        .orderBy(
          desc(runtimeEvents.commit_seq),
          desc(runtimeEvents.event_ordinal),
        )
        .limit(100),
    );
    const sized = db.$with('bounded_events').as(
      db
        .select({
          event_type: recent.event_type,
          payload: recent.payload,
          commit_seq: recent.commit_seq,
          event_ordinal: recent.event_ordinal,
          bytes:
            sql<number>`sum(octet_length(${recent.payload}::text)+octet_length(${recent.event_type})+64)
        over (order by ${recent.commit_seq} desc, ${recent.event_ordinal} desc)`.as(
              'bytes',
            ),
        })
        .from(recent),
    );
    const events = await db
      .with(recent, sized)
      .select({ event_type: sized.event_type, payload: sized.payload })
      .from(sized)
      .where(lte(sized.bytes, budget))
      .orderBy(desc(sized.commit_seq), desc(sized.event_ordinal));
    return { ...base, messages_page, events: events.reverse() };
  }

  async acceptControl(input: ControlInput) {
    const hash = canonicalHash(input);
    const [row] = await database(this.pool)
      .insert(serviceControlInbox)
      .values({
        decision_id: input.decision_id,
        root_run_id: input.root_run_id,
        authorization_revision: String(input.authorization_revision),
        content_hash: hash,
        input,
      })
      .onConflictDoUpdate({
        target: serviceControlInbox.decision_id,
        set: { decision_id: input.decision_id },
      })
      .returning({
        content_hash: serviceControlInbox.content_hash,
        receipt: serviceControlInbox.receipt,
      });
    if (!row) throw new Error('Control insert returned no row');
    if (row.content_hash !== hash)
      throw new AgentError('decision_identity_conflict');
    return row.receipt;
  }
}
