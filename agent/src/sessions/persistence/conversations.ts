import type { Pool, PoolClient } from 'pg';
import type {
  HistoryInput,
  Identity,
  MessageInput,
  UpdateSessionInput,
} from '../../../contracts/agent.js';
import { AgentError } from '../errors.js';
import { SessionStore, inTransaction, type RunRow } from './store.js';

type Database = Pool | PoolClient;
import type { PageCursor } from './pagination.js';
import { readMessagePage } from './message-history.js';
const RUN_SUMMARIES = `SELECT r.*,coalesce(e.payload->>'status','accepted') AS public_status FROM agent.service_runs r
 LEFT JOIN LATERAL (SELECT payload FROM agent.runtime_events WHERE session_id=r.session_id::text AND payload->>'root_run_id'=r.id::text AND event_type='agent.state' ORDER BY commit_seq DESC,event_ordinal DESC LIMIT 1) e ON true`;
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
    const rows = (
      await this.pool.query<
        Parameters<SessionStore['sessionDTO']>[0] & {
          last_input_at: Date;
          last_input_cursor: string;
          snapshot_at: string;
        }
      >(
        `
      WITH bound AS (SELECT coalesce($3::timestamptz,statement_timestamp()) AS upper), owned AS (
        SELECT s.id,s.actor_id,s.org_id,s.scope_id,s.project_id,s.canvas_id,s.title,s.pinned,s.execution_revision,s.created_at, max(r.created_at) AS last_input_at FROM agent.service_sessions s
        JOIN agent.service_runs r ON r.session_id=s.id CROSS JOIN bound
        WHERE s.actor_id=$1 AND s.project_id=$2 AND s.archived_at IS NULL AND r.created_at<=bound.upper GROUP BY s.id
      ) SELECT owned.*,bound.upper::text AS snapshot_at,last_input_at::text AS last_input_cursor
        FROM owned CROSS JOIN bound
        WHERE ($4::timestamptz IS NULL OR (pinned,last_input_at,id)<($7::boolean,$4::timestamptz,$5::uuid))
        ORDER BY pinned DESC,last_input_at DESC,id DESC LIMIT $6`,
        [
          query.actor_id,
          query.project_id,
          cursor?.upper ?? null,
          cursor?.before ?? null,
          cursor?.id ?? null,
          (query.limit ?? 20) + 1,
          cursor?.pinned ?? false,
        ],
      )
    ).rows;
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
      type Row = Parameters<SessionStore['sessionDTO']>[0];
      const session = (
        await db.query<Row>(
          'SELECT * FROM agent.service_sessions WHERE id=$1 AND actor_id=$2 AND project_id=$3 AND org_id=$4 AND scope_id=$5 FOR UPDATE',
          [id, input.actor_id, input.project_id, input.org_id, input.scope_id],
        )
      ).rows[0];
      if (!session || (session.archived_at && input.action !== 'delete'))
        throw new AgentError('session_not_found');
      if (session.archived_at) return this.store.sessionDTO(session);

      if (input.action === 'delete') {
        const last = (
          await db.query<RunRow & { public_status: string }>(
            RUN_SUMMARIES +
              ' WHERE r.session_id=$1 ORDER BY r.accepted_revision DESC LIMIT 1',
            [id],
          )
        ).rows[0];
        if (last && !terminal.has(last.public_status))
          throw new AgentError('session_has_active_run');
      }

      const row = (
        await db.query<Row>(
          `UPDATE agent.service_sessions
           SET title=coalesce($2,title), pinned=coalesce($3,pinned),
             archived_at=CASE WHEN $4 THEN clock_timestamp() ELSE archived_at END
           WHERE id=$1 RETURNING *`,
          [
            id,
            title ?? null,
            input.action === 'pin'
              ? true
              : input.action === 'unpin'
                ? false
                : null,
            input.action === 'delete',
          ],
        )
      ).rows[0]!;
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
        const upper =
          cursor?.upper ??
          String(
            (
              await db.query<{ revision: string }>(
                'SELECT coalesce(max(accepted_revision),0) AS revision FROM agent.service_runs WHERE session_id=$1',
                [id],
              )
            ).rows[0]!.revision,
          );
        if (!/^\d+$/.test(upper) || (cursor && !/^\d+$/.test(cursor.before)))
          throw new AgentError('invalid_cursor');
        const rows = (
          await db.query<RunRow & { public_status?: string }>(
            RUN_SUMMARIES +
              ' WHERE r.session_id=$1 AND r.accepted_revision<=$2 AND ($3::bigint IS NULL OR r.accepted_revision<$3) ORDER BY r.accepted_revision DESC LIMIT $4',
            [id, upper, cursor?.before ?? null, (query.limit ?? 20) + 1],
          )
        ).rows;
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
        const run = (
          await db.query<RunRow & { public_status?: string }>(
            'SELECT * FROM agent.service_runs WHERE session_id=$1 AND request_id=$2',
            [id, request],
          )
        ).rows[0];
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
        const run = (
          await db.query<RunRow & { public_status?: string }>(
            RUN_SUMMARIES +
              ' WHERE r.session_id=$1 ORDER BY r.accepted_revision DESC LIMIT 1',
            [id],
          )
        ).rows[0];
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
        const rows = (
          await db.query<{
            message_id: string;
            run_id: string;
            conversation_id: number;
            sequence: string;
            part_index: number;
            kind: string;
            content: string;
          }>(
            'SELECT * FROM agent.public_messages WHERE session_id=$1 AND run_id=$2 AND message_id=$3 ORDER BY part_index',
            [run.session_id, run.id, messageId],
          )
        ).rows;
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
