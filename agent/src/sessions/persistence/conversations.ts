import type { Pool } from 'pg';
import { sql } from 'kysely';
import {
  database,
  queries,
  type DatabaseConnection,
} from '../../database/connection.js';
import type {
  HistoryInput,
  Identity,
  MessageInput,
  UpdateSessionInput,
} from '../types.js';
import { AgentError } from '../errors.js';
import { SessionStore, inTransaction, runRow, type RunRow } from './store.js';
import type { PageCursor } from './pagination.js';
import { readMessagePage } from './message-history.js';
function runSummaries(connection: DatabaseConnection) {
  return queries(connection)
    .selectFrom('service_runs')
    .leftJoinLateral(
      (qb) =>
        qb
          .selectFrom('runtime_events')
          .select('payload')
          .where(
            'runtime_events.session_id',
            '=',
            sql<string>`service_runs.session_id::text`,
          )
          .where(
            sql<string>`runtime_events.payload->>'root_run_id'`,
            '=',
            sql<string>`service_runs.id::text`,
          )
          .where('runtime_events.event_type', '=', 'agent.state')
          .orderBy('runtime_events.commit_seq', 'desc')
          .orderBy('runtime_events.event_ordinal', 'desc')
          .limit(1)
          .as('latest'),
      (join) => join.onTrue(),
    )
    .selectAll('service_runs')
    .select(
      sql<string>`coalesce(latest.payload->>'status', 'accepted')`.as(
        'public_status',
      ),
    );
}
const terminal = new Set(['completed', 'failed', 'stopped', 'rejected']);
export function acceptedRun(run: RunRow, status = 'accepted') {
  return {
    root_run_id: run.id,
    session_id: run.session_id,
    submission_id: run.submission_id,
    status,
    execution_revision: Number(run.accepted_revision),
  };
}

/** Read projections are scoped again in PostgreSQL; cursors never grant access. */
export class ConversationStore {
  constructor(
    private readonly pool: Pool,
    private readonly store: SessionStore,
  ) {}
  private scope(kind: string, who: Identity, id = '') {
    return `${kind}:${who.actor_id}:${who.project_id}:${id}`;
  }
  private encode(cursor: PageCursor) {
    return this.store.cursors.encode(cursor);
  }
  private decode(value: string | undefined, scope: string) {
    return this.store.cursors.decode(value, scope);
  }
  async sessions(query: HistoryInput) {
    const scope = this.scope('session-history', query);
    const cursor = this.decode(query.cursor, scope);
    // Timestamps used by cursors stay as PostgreSQL text, preserving microseconds.
    let statement = queries(this.pool)
      .with('bound', (qb) =>
        qb.selectNoFrom(
          sql<string>`coalesce(${cursor?.upper ?? null}::timestamptz,statement_timestamp())::text`.as(
            'upper',
          ),
        ),
      )
      .with('owned', (qb) =>
        qb
          .selectFrom('service_sessions')
          .innerJoin(
            'service_runs',
            'service_runs.session_id',
            'service_sessions.id',
          )
          .crossJoin('bound')
          .selectAll('service_sessions')
          .select([
            sql<Date>`max(service_runs.created_at)`.as(
              'computed_last_input_at',
            ),
            sql<string>`max(service_runs.created_at)::text`.as(
              'last_input_cursor',
            ),
          ])
          .where('service_sessions.actor_id', '=', query.actor_id)
          .where('service_sessions.project_id', '=', query.project_id)
          .where('service_sessions.archived_at', 'is', null)
          .where(
            'service_runs.created_at',
            '<=',
            sql<Date>`bound.upper::timestamptz`,
          )
          .groupBy('service_sessions.id'),
      )
      .selectFrom('owned')
      .crossJoin('bound')
      .selectAll('owned')
      .select('bound.upper as snapshot_at');
    if (cursor)
      statement = statement.where(
        sql<boolean>`(owned.pinned, owned.computed_last_input_at, owned.id) < (${cursor.pinned ?? false}::boolean, ${cursor.before}::timestamptz, ${cursor.id}::uuid)`,
      );
    const rows = await statement
      .orderBy('owned.pinned', 'desc')
      .orderBy('owned.computed_last_input_at', 'desc')
      .orderBy('owned.id', 'desc')
      .limit((query.limit ?? 20) + 1)
      .execute();
    const more = rows.length > (query.limit ?? 20);
    const items = rows.slice(0, query.limit ?? 20),
      last = items.at(-1);
    return {
      items: items.map((r) => ({
        ...this.store.sessionDTO(r),
        last_input_at: r.computed_last_input_at.toISOString(),
      })),
      ...(more && last
        ? {
            next_cursor: this.encode({
              scope,
              upper: last.snapshot_at,
              before: last.last_input_cursor,
              id: last.id,
              pinned: last.pinned,
            }),
          }
        : {}),
    };
  }
  async updateSession(id: string, input: UpdateSessionInput) {
    const title = input.title?.trim();
    if (input.action === 'rename' ? !title : input.title !== undefined)
      throw new AgentError('invalid_session_update');
    return inTransaction(this.pool, async (connection) => {
      const session = await queries(connection)
        .selectFrom('service_sessions')
        .selectAll()
        .where('id', '=', id)
        .where('actor_id', '=', input.actor_id)
        .where('project_id', '=', input.project_id)
        .where('org_id', '=', input.org_id)
        .where('scope_id', '=', input.scope_id)
        .forUpdate()
        .executeTakeFirst();
      if (!session || (session.archived_at && input.action !== 'delete'))
        throw new AgentError('session_not_found');
      if (session.archived_at) return this.store.sessionDTO(session);
      if (input.action === 'delete') {
        const last = await runSummaries(connection)
          .where('service_runs.session_id', '=', id)
          .orderBy('service_runs.accepted_revision', 'desc')
          .limit(1)
          .executeTakeFirst();
        if (last && !terminal.has(last.public_status))
          throw new AgentError('session_has_active_run');
      }
      const row =
        input.action === 'delete'
          ? await queries(connection)
              .updateTable('service_sessions')
              .set({ archived_at: sql<Date>`clock_timestamp()` })
              .where('id', '=', id)
              .returningAll()
              .executeTakeFirstOrThrow()
          : await database(connection).serviceSession.update({
              where: { id },
              data: {
                ...(title !== undefined ? { title } : {}),
                ...(input.action === 'pin'
                  ? { pinned: true }
                  : input.action === 'unpin'
                    ? { pinned: false }
                    : {}),
              },
            });
      return this.store.sessionDTO(row);
    });
  }
  private async requireSession(
    id: string,
    who: Identity,
    db: DatabaseConnection,
  ) {
    if (!(await this.store.session(id, who, db)))
      throw new AgentError('session_not_found');
  }
  private summary(run: RunRow & { public_status?: string }) {
    return {
      root_run_id: run.id,
      session_id: run.session_id,
      accepted_revision: Number(run.accepted_revision),
      created_at: run.created_at.toISOString(),
      status: run.public_status ?? 'accepted',
      input_excerpt: Array.from(run.input ? run.input.plain_text : run.goal)
        .slice(0, 240)
        .join(''),
      ...(run.retry_of_run_id ? { retry_of_run_id: run.retry_of_run_id } : {}),
    };
  }
  async runs(id: string, query: HistoryInput) {
    return inTransaction(
      this.pool,
      async (connection) => {
        await this.requireSession(id, query, connection);
        const scope = this.scope('runs', query, id),
          cursor = this.decode(query.cursor, scope);
        const maximum = cursor
          ? undefined
          : await database(connection).serviceRun.aggregate({
              _max: { accepted_revision: true },
              where: { session_id: id },
            });
        const upper =
          cursor?.upper ?? String(maximum?._max.accepted_revision ?? 0);
        if (!/^\d+$/.test(upper) || (cursor && !/^\d+$/.test(cursor.before)))
          throw new AgentError('invalid_cursor');
        let statement = runSummaries(connection)
          .where('service_runs.session_id', '=', id)
          .where('service_runs.accepted_revision', '<=', BigInt(upper));
        if (cursor)
          statement = statement.where(
            'service_runs.accepted_revision',
            '<',
            BigInt(cursor.before),
          );
        const rows = await statement
          .orderBy('service_runs.accepted_revision', 'desc')
          .limit((query.limit ?? 20) + 1)
          .execute();
        const more = rows.length > (query.limit ?? 20),
          page = rows.slice(0, query.limit ?? 20),
          last = page.at(-1);
        return {
          items: page
            .toReversed()
            .map((r) =>
              this.summary({ ...runRow(r), public_status: r.public_status }),
            ),
          ...(more && last
            ? {
                next_cursor: this.encode({
                  scope,
                  upper,
                  before: String(last.accepted_revision),
                }),
              }
            : {}),
        };
      },
      true,
    );
  }
  async submission(id: string, request: string, who: Identity) {
    return inTransaction(
      this.pool,
      async (connection) => {
        await this.requireSession(id, who, connection);
        const run = await database(connection).serviceRun.findUnique({
          where: {
            session_id_request_id: { session_id: id, request_id: request },
          },
        });
        return run ? acceptedRun(runRow(run)) : undefined;
      },
      true,
    );
  }
  async current(id: string, who: Identity) {
    return inTransaction(
      this.pool,
      async (connection) => {
        await this.requireSession(id, who, connection);
        const run = await runSummaries(connection)
          .where('service_runs.session_id', '=', id)
          .orderBy('service_runs.accepted_revision', 'desc')
          .limit(1)
          .executeTakeFirst();
        if (!run) return {};
        const decoded = runRow(run),
          summary = this.summary({
            ...decoded,
            public_status: run.public_status,
          });
        return {
          latest_accepted: acceptedRun(decoded, summary.status),
          ...(!terminal.has(summary.status) ? { active_run: summary } : {}),
          unresolved_run_ids:
            summary.status === 'needs_reconciliation' ? [run.id] : [],
        };
      },
      true,
    );
  }
  async messages(id: string, query: MessageInput) {
    return inTransaction(
      this.pool,
      async (db) => {
        const run = await this.store.run(id, query, db);
        if (!run) throw new AgentError('run_not_found');
        return this.messagePage(run, query, db);
      },
      true,
    );
  }
  async messagePage(run: RunRow, query: MessageInput, db: DatabaseConnection) {
    return readMessagePage(db, this.store.cursors, run, query);
  }
  async message(id: string, messageId: string, who: Identity) {
    return inTransaction(
      this.pool,
      async (connection) => {
        const run = await this.store.run(id, who, connection);
        if (!run) throw new AgentError('run_not_found');
        const size = await queries(connection)
          .selectFrom('public_messages')
          .select(
            sql<bigint>`coalesce(sum(octet_length(content)),0)`.as('bytes'),
          )
          .where('session_id', '=', run.session_id)
          .where('run_id', '=', run.id)
          .where('message_id', '=', messageId)
          .executeTakeFirst();
        if (size && Number(size.bytes) > 65536)
          throw new AgentError('quoted_message_exceeds_limit');
        const rows = await database(connection).publicMessage.findMany({
          where: {
            session_id: run.session_id,
            run_id: run.id,
            message_id: messageId,
          },
          orderBy: { part_index: 'asc' },
        });
        if (!rows.length) return;
        const content = rows.map((r) => r.content).join('');
        if (Buffer.byteLength(content) > 65536)
          throw new AgentError('quoted_message_exceeds_limit');
        const first = rows[0]!;
        return {
          message_id: first.message_id,
          run_id: first.run_id,
          conversation_id: Number(first.conversation_id),
          sequence: first.sequence,
          part_index: 0,
          kind: first.kind,
          content,
        };
      },
      true,
    );
  }
}
