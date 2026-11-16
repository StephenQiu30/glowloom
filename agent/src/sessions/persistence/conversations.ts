import type { Pool } from 'pg';
import {
  and,
  desc,
  eq,
  getTableColumns,
  isNull,
  lt,
  lte,
  sql,
} from 'drizzle-orm';
import {
  database,
  type DatabaseConnection,
} from '../../database/connection.js';
import {
  publicMessages,
  runtimeEvents,
  serviceRuns,
  serviceSessions,
} from '../../database/schema.js';
import type {
  HistoryInput,
  Identity,
  MessageInput,
  UpdateSessionInput,
} from '../types.js';
import { AgentError } from '../errors.js';
import { SessionStore, inTransaction, type RunRow } from './store.js';

type Database = DatabaseConnection;
import type { PageCursor } from './pagination.js';
import { readMessagePage } from './message-history.js';
function runSummaries(connection: DatabaseConnection) {
  const db = database(connection);
  const latest = db
    .select({ payload: runtimeEvents.payload })
    .from(runtimeEvents)
    .where(
      and(
        eq(runtimeEvents.session_id, sql`${serviceRuns.session_id}::text`),
        eq(
          sql<string>`${runtimeEvents.payload}->>'root_run_id'`,
          sql`${serviceRuns.id}::text`,
        ),
        eq(runtimeEvents.event_type, 'agent.state'),
      ),
    )
    .orderBy(desc(runtimeEvents.commit_seq), desc(runtimeEvents.event_ordinal))
    .limit(1)
    .as('latest');
  return db
    .select({
      ...getTableColumns(serviceRuns),
      public_status:
        sql<string>`coalesce(${latest.payload}->>'status', 'accepted')`.as(
          'public_status',
        ),
    })
    .from(serviceRuns)
    .leftJoinLateral(latest, sql`true`)
    .$dynamic();
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

/** Read projections are scoped again in SQL; cursors never grant access. */
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
    const db = database(this.pool);
    // Keep the snapshot and keyset timestamps in PostgreSQL text form so
    // microseconds survive the cursor round trip through JavaScript.
    const bound = db.$with('bound').as(
      db
        .select({
          upper:
            sql<string>`coalesce(${cursor?.upper ?? null}::timestamptz,statement_timestamp())::text`.as(
              'upper',
            ),
        })
        .from(sql`(values (1)) as snapshot_seed(value)`),
    );
    const owned = db.$with('owned').as(
      db
        .select({
          ...getTableColumns(serviceSessions),
          last_input_at: sql<Date>`max(${serviceRuns.created_at})`
            .mapWith(serviceRuns.created_at)
            .as('last_input_at'),
          last_input_cursor:
            sql<string>`max(${serviceRuns.created_at})::text`.as(
              'last_input_cursor',
            ),
        })
        .from(serviceSessions)
        .innerJoin(serviceRuns, eq(serviceRuns.session_id, serviceSessions.id))
        .crossJoin(bound)
        .where(
          and(
            eq(serviceSessions.actor_id, query.actor_id),
            eq(serviceSessions.project_id, query.project_id),
            isNull(serviceSessions.archived_at),
            lte(serviceRuns.created_at, sql`${bound.upper}::timestamptz`),
          ),
        )
        .groupBy(serviceSessions.id),
    );
    const result = await db
      .with(bound, owned)
      .select()
      .from(owned)
      .crossJoin(bound)
      .where(
        cursor
          ? lt(
              sql`(${owned.pinned}, ${owned.last_input_at}, ${owned.id})`,
              sql`(${cursor.pinned ?? false}::boolean, ${cursor.before}::timestamptz, ${cursor.id}::uuid)`,
            )
          : undefined,
      )
      .orderBy(desc(owned.pinned), desc(owned.last_input_at), desc(owned.id))
      .limit((query.limit ?? 20) + 1);
    const rows = result.map(({ owned: row, bound: snapshot }) => ({
      ...row,
      snapshot_at: snapshot.upper,
    }));
    const more = rows.length > (query.limit ?? 20);
    const items = rows.slice(0, query.limit ?? 20);
    const last = items.at(-1);
    return {
      items: items.map((r) => ({
        ...this.store.sessionDTO(r),
        last_input_at: r.last_input_at.toISOString(),
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
    return inTransaction(this.pool, async (db) => {
      const [session] = await db
        .select()
        .from(serviceSessions)
        .where(
          and(
            eq(serviceSessions.id, id),
            eq(serviceSessions.actor_id, input.actor_id),
            eq(serviceSessions.project_id, input.project_id),
            eq(serviceSessions.org_id, input.org_id),
            eq(serviceSessions.scope_id, input.scope_id),
          ),
        )
        .for('update');
      if (!session || (session.archived_at && input.action !== 'delete'))
        throw new AgentError('session_not_found');
      if (session.archived_at) return this.store.sessionDTO(session);

      if (input.action === 'delete') {
        const [last] = await runSummaries(db)
          .where(eq(serviceRuns.session_id, id))
          .orderBy(desc(serviceRuns.accepted_revision))
          .limit(1);
        if (last && !terminal.has(last.public_status))
          throw new AgentError('session_has_active_run');
      }

      const [row] = await db
        .update(serviceSessions)
        .set({
          ...(title !== undefined ? { title } : {}),
          ...(input.action === 'pin'
            ? { pinned: true }
            : input.action === 'unpin'
              ? { pinned: false }
              : {}),
          ...(input.action === 'delete'
            ? { archived_at: sql`clock_timestamp()` }
            : {}),
        })
        .where(eq(serviceSessions.id, id))
        .returning();
      if (!row) throw new Error('Session update returned no row');
      return this.store.sessionDTO(row);
    });
  }
  private async requireSession(id: string, who: Identity, db: Database) {
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
      async (db) => {
        await this.requireSession(id, query, db);
        const scope = this.scope('runs', query, id);
        const cursor = this.decode(query.cursor, scope);
        const [maximum] = cursor
          ? []
          : await db
              .select({
                revision:
                  sql<string>`coalesce(max(${serviceRuns.accepted_revision}),0)::text`.as(
                    'revision',
                  ),
              })
              .from(serviceRuns)
              .where(eq(serviceRuns.session_id, id));
        const upper = cursor?.upper ?? maximum?.revision ?? '0';
        if (!/^\d+$/.test(upper) || (cursor && !/^\d+$/.test(cursor.before)))
          throw new AgentError('invalid_cursor');
        const rows = await runSummaries(db)
          .where(
            and(
              eq(serviceRuns.session_id, id),
              lte(serviceRuns.accepted_revision, upper),
              cursor
                ? lt(serviceRuns.accepted_revision, cursor.before)
                : undefined,
            ),
          )
          .orderBy(desc(serviceRuns.accepted_revision))
          .limit((query.limit ?? 20) + 1);
        const more = rows.length > (query.limit ?? 20),
          page = rows.slice(0, query.limit ?? 20),
          last = page.at(-1);
        const items = page.toReversed().map((r) => this.summary(r));
        return {
          items,
          ...(more && last
            ? {
                next_cursor: this.encode({
                  scope,
                  upper,
                  before: last.accepted_revision,
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
      async (db) => {
        await this.requireSession(id, who, db);
        const [run] = await db
          .select()
          .from(serviceRuns)
          .where(
            and(
              eq(serviceRuns.session_id, id),
              eq(serviceRuns.request_id, request),
            ),
          );
        return run ? acceptedRun(run) : undefined;
      },
      true,
    );
  }
  async current(id: string, who: Identity) {
    return inTransaction(
      this.pool,
      async (db) => {
        await this.requireSession(id, who, db);
        const [run] = await runSummaries(db)
          .where(eq(serviceRuns.session_id, id))
          .orderBy(desc(serviceRuns.accepted_revision))
          .limit(1);
        if (!run) return {};
        const summary = this.summary(run);
        return {
          latest_accepted: acceptedRun(run, summary.status),
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
  async messagePage(run: RunRow, query: MessageInput, db: Database) {
    return readMessagePage(db, this.store.cursors, run, query);
  }
  async message(id: string, messageId: string, who: Identity) {
    return inTransaction(
      this.pool,
      async (db) => {
        const run = await this.store.run(id, who, db);
        if (!run) throw new AgentError('run_not_found');
        const messageScope = and(
          eq(publicMessages.session_id, run.session_id),
          eq(publicMessages.run_id, run.id),
          eq(publicMessages.message_id, messageId),
        );
        const [size] = await db
          .select({
            bytes:
              sql<number>`coalesce(sum(octet_length(${publicMessages.content})),0)`.mapWith(
                Number,
              ),
          })
          .from(publicMessages)
          .where(messageScope);
        if (size && size.bytes > 65536)
          throw new AgentError('quoted_message_exceeds_limit');
        const rows = await db
          .select()
          .from(publicMessages)
          .where(messageScope)
          .orderBy(publicMessages.part_index);
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
