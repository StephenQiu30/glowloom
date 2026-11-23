import { RecoveryStore } from '../recovery/store.js';
import { publicState } from '../../pi-durable/events.js';
import type { Pool } from 'pg';
import { sql } from 'kysely';
import {
  database,
  queries,
  withTransaction,
  type AgentTransaction,
  type DatabaseConnection,
} from '../../database/connection.js';
import type {
  ServiceSession,
  ServiceRun,
  Prisma,
} from '../../database/generated/client.js';
import type { JsonObject } from '@earendil-works/pi-durable';
import { randomUUID } from 'node:crypto';
import type {
  CreateSessionInput,
  CreateRunInput,
  ControlInput,
  Identity,
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
export type SessionRow = ServiceSession;
export type RunRow = Omit<ServiceRun, 'manifest' | 'input'> & {
  manifest: FrozenManifest;
  input: AuthorizedInput | null;
};
/** These JSON documents are written and validated at the session input boundary. */
export function runRow(row: ServiceRun | RunRow): RunRow {
  return {
    ...row,
    manifest: row.manifest as unknown as FrozenManifest,
    input: row.input as unknown as AuthorizedInput | null,
  };
}
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
    const row = await database(this.pool).serviceSession.upsert({
      where: {
        actor_id_project_id_request_id: {
          actor_id: input.actor_id,
          project_id: input.project_id,
          request_id: input.request_id,
        },
      },
      create: {
        id: randomUUID(),
        actor_id: input.actor_id,
        org_id: input.org_id,
        scope_id: input.scope_id,
        project_id: input.project_id,
        canvas_id: input.canvas_id ?? null,
        title: input.title,
        request_id: input.request_id,
        content_hash: hash,
      },
      update: { request_id: input.request_id },
    });
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
    return (
      (await database(client).serviceSession.findFirst({
        where: {
          id,
          actor_id: who.actor_id,
          project_id: who.project_id,
          archived_at: null,
        },
      })) ?? undefined
    );
  }
  async list(who: Identity) {
    const rows = await database(this.pool).serviceSession.findMany({
      where: {
        actor_id: who.actor_id,
        project_id: who.project_id,
        archived_at: null,
      },
      orderBy: [{ created_at: 'desc' }, { id: 'desc' }],
      take: 100,
    });
    return rows.map((r) => this.sessionDTO(r));
  }
  async admitRun(
    id: string,
    input: CreateRunInput,
    manifest: Readonly<FrozenManifest>,
  ) {
    return inTransaction(this.pool, async (client) => {
      const db = database(client);
      const session = await queries(client)
        .selectFrom('service_sessions')
        .selectAll()
        .where('id', '=', id)
        .where('actor_id', '=', input.actor_id)
        .where('project_id', '=', input.project_id)
        .where('archived_at', 'is', null)
        .forUpdate()
        .executeTakeFirst();
      if (!session) throw new AgentError('session_not_found');
      let draft = input.input;
      // Storyboard ordinals resolve at acceptance. The same request can be
      // delivered again after a reorder and must still return its first run.
      const hash = canonicalHash({
        ...input,
        input: { ...draft, storyboard_targets: undefined },
      });
      const prior = await db.serviceRun.findUnique({
        where: {
          session_id_request_id: {
            session_id: id,
            request_id: input.request_id,
          },
        },
      });
      if (prior) {
        if (prior.content_hash !== hash)
          throw new AgentError('request_identity_conflict');
        return runRow(prior);
      }
      const previousRun = await db.serviceRun.findFirst({
        where: { session_id: id },
        orderBy: { accepted_revision: 'desc' },
      });
      const last = previousRun ? runRow(previousRun) : undefined;
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
        const targets = original.input.storyboard_targets;
        draft = { ...draft };
        if (targets) draft.storyboard_targets = targets;
        else delete draft.storyboard_targets;
      }
      if (
        draft.storyboard_targets?.length &&
        (!session.canvas_id || session.canvas_id !== input.canvas_id)
      )
        throw new AgentError('session_not_found');
      const run = stableId(id, 'run:' + input.request_id);
      const submission = stableId(run, 'submission:initial');
      const row = await db.serviceRun.create({
        data: {
          id: run,
          session_id: id,
          request_id: input.request_id,
          content_hash: hash,
          goal: input.goal,
          manifest: manifest as unknown as Prisma.InputJsonObject,
          submission_id: submission,
          accepted_revision: BigInt(revision + 1),
          input: draft as unknown as Prisma.InputJsonObject,
          retry_of_run_id: input.retry_of_run_id ?? null,
        },
      });
      await queries(client)
        .updateTable('service_sessions')
        .set({
          execution_revision: BigInt(revision + 1),
          last_input_at: sql<Date>`clock_timestamp()`,
        })
        .where('id', '=', id)
        .execute();
      return runRow(row);
    });
  }
  async run(id: string, who: Identity, client: DatabaseConnection = this.pool) {
    const row = await queries(client)
      .selectFrom('service_runs')
      .innerJoin(
        'service_sessions',
        'service_sessions.id',
        'service_runs.session_id',
      )
      .selectAll('service_runs')
      .where('service_runs.id', '=', id)
      .where('service_sessions.actor_id', '=', who.actor_id)
      .where('service_sessions.project_id', '=', who.project_id)
      .where('service_sessions.archived_at', 'is', null)
      .executeTakeFirst();
    return row ? runRow(row) : undefined;
  }
  async snapshot(run: RunRow, client: DatabaseConnection = this.pool) {
    const db = database(client);
    const state = await db.runtimeEvent.findFirst({
      select: { payload: true },
      where: {
        session_id: run.session_id,
        payload: { path: ['root_run_id'], equals: run.id },
        event_type: 'agent.state',
      },
      orderBy: [{ commit_seq: 'desc' }, { event_ordinal: 'desc' }],
    });
    const cursor = await db.runtimeEvent.findFirst({
      select: { commit_seq: true, event_ordinal: true },
      where: { session_id: run.session_id },
      orderBy: [{ commit_seq: 'desc' }, { event_ordinal: 'desc' }],
    });
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
        ? (publicState(state.payload as JsonObject) as Partial<AgentState> &
            Pick<AgentState, 'status' | 'execution_revision'>)
        : {
            status: 'accepted',
            execution_revision: Number(run.accepted_revision),
          },
      snapshot_cursor: cursor
        ? `${cursor.commit_seq}:${cursor.event_ordinal}`
        : '0:0',
    };
    const who = await db.serviceSession.findUnique({
      select: { actor_id: true, project_id: true },
      where: { id: run.session_id },
    });
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
    // Apply the byte budget in PostgreSQL before transferring any event JSON.
    const events = await queries(client)
      .with('recent', (qb) =>
        qb
          .selectFrom('runtime_events')
          .select(['event_type', 'payload', 'commit_seq', 'event_ordinal'])
          .where('session_id', '=', run.session_id)
          .where(sql<string>`payload->>'root_run_id'`, '=', run.id)
          .where('event_type', '!=', 'agent.state')
          .orderBy('commit_seq', 'desc')
          .orderBy('event_ordinal', 'desc')
          .limit(100),
      )
      .with('bounded_events', (qb) =>
        qb
          .selectFrom('recent')
          .selectAll()
          .select(
            sql<bigint>`sum(octet_length(payload::text)+octet_length(event_type)+64) over (order by commit_seq desc, event_ordinal desc)`.as(
              'bytes',
            ),
          ),
      )
      .selectFrom('bounded_events')
      .select(['event_type', 'payload'])
      .where('bytes', '<=', BigInt(budget))
      .orderBy('commit_seq', 'desc')
      .orderBy('event_ordinal', 'desc')
      .execute();
    return { ...base, messages_page, events: events.reverse() };
  }

  async recordControlReceipt(
    decisionId: string,
    receipt: Record<string, unknown>,
  ) {
    await database(this.pool).serviceControlInbox.updateMany({
      where: { decision_id: decisionId },
      data: { receipt: receipt as Prisma.InputJsonObject },
    });
  }

  async acceptControl(input: ControlInput) {
    const hash = canonicalHash(input);
    const row = await database(this.pool).serviceControlInbox.upsert({
      where: { decision_id: input.decision_id },
      create: {
        decision_id: input.decision_id,
        root_run_id: input.root_run_id,
        authorization_revision: BigInt(input.authorization_revision),
        content_hash: hash,
        input: input as unknown as Prisma.InputJsonObject,
      },
      update: { decision_id: input.decision_id },
      select: { content_hash: true, receipt: true },
    });
    if (row.content_hash !== hash)
      throw new AgentError('decision_identity_conflict');
    return row.receipt as JsonObject | null;
  }
}
