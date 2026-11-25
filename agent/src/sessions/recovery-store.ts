import { sql } from 'kysely';
import {
  database,
  queries,
  type DatabaseConnection,
} from '../database/connection.js';
import { recoveryDefaults } from '../config.js';
import { RunStatus } from '../harness/state-doc.js';

export const RecoveryKind = { Run: 'run', Control: 'control' } as const;
export const RecoveryCode = {
  ScanUnavailable: 'recovery_scan_unavailable',
  ExecutionUnavailable: 'execution_unavailable',
  ControlUnavailable: 'control_delivery_unavailable',
  DefinitionsUnavailable: 'pinned_definitions_unavailable',
  CloseFailed: 'execution_close_failed',
} as const;
export type RecoveryKind = (typeof RecoveryKind)[keyof typeof RecoveryKind];
export type RecoveryCode = (typeof RecoveryCode)[keyof typeof RecoveryCode];
export interface RecoveryRecord {
  kind: RecoveryKind;
  subject_id: string;
  attempts: number;
  code: RecoveryCode;
  next_attempt_at: string;
  parked_at: string | null;
}
export interface RecoveryEvent {
  kind: RecoveryKind | 'scan';
  code: RecoveryCode;
  root_run_id?: string;
  subject_id?: string;
  consecutive_failures?: number;
}
/** Reads recovery candidates and records retry diagnostics for existing runs and controls. */
export class RecoveryStore {
  constructor(private readonly pool: DatabaseConnection) {}

  async failed(
    kind: RecoveryKind,
    subject: string,
    root: string,
    code: RecoveryCode,
    terminal = false,
  ) {
    await queries(this.pool)
      .insertInto('recovery_backoff')
      .values({
        kind,
        subject_id: subject,
        root_run_id: root,
        attempts: 1,
        code,
        next_attempt_at: sql<Date>`clock_timestamp()+${recoveryDefaults.pollMs}*interval '1 millisecond'`,
        parked_at: terminal ? sql<Date>`clock_timestamp()` : null,
      })
      .onConflict((conflict) =>
        conflict.columns(['kind', 'subject_id']).doUpdateSet((expression) => ({
          attempts: expression('recovery_backoff.attempts', '+', 1),
          code,
          next_attempt_at: sql<Date>`clock_timestamp()+least(${recoveryDefaults.maxBackoffMs}, ${recoveryDefaults.pollMs}*power(2,least(${expression.ref('recovery_backoff.attempts')},16)))*interval '1 millisecond'`,
          parked_at: sql<Date | null>`case when ${terminal} or ${expression.ref('recovery_backoff.attempts')}+1 >= ${recoveryDefaults.maxAttempts} then coalesce(${expression.ref('recovery_backoff.parked_at')},clock_timestamp()) else null end`,
        })),
      )
      .execute();
  }
  async succeeded(kind: RecoveryKind, subject: string) {
    await queries(this.pool)
      .updateTable('recovery_backoff')
      .set({
        attempts: 0,
        code: null,
        parked_at: null,
        next_attempt_at: sql<Date>`clock_timestamp()`,
      })
      .where('kind', '=', kind)
      .where('subject_id', '=', subject)
      .execute();
  }
  async read(root: string): Promise<RecoveryRecord[]> {
    const rows = await database(this.pool).recoveryBackoff.findMany({
      select: {
        kind: true,
        subject_id: true,
        attempts: true,
        code: true,
        next_attempt_at: true,
        parked_at: true,
      },
      where: { root_run_id: root, code: { not: null } },
      orderBy: [{ kind: 'asc' }, { subject_id: 'asc' }],
    });
    return rows.map((row) => ({
      ...row,
      kind: row.kind as RecoveryKind,
      code: row.code as RecoveryCode,
      next_attempt_at: row.next_attempt_at.toISOString(),
      parked_at: row.parked_at?.toISOString() ?? null,
    }));
  }

  pendingControls() {
    return queries(this.pool)
      .selectFrom('service_control_inbox as controls')
      .leftJoin('recovery_backoff as backoff', (join) =>
        join
          .on('backoff.kind', '=', RecoveryKind.Control)
          .onRef('backoff.subject_id', '=', 'controls.decision_id'),
      )
      .select('controls.input')
      .where('controls.receipt', 'is', null)
      .where('backoff.parked_at', 'is', null)
      .where((expression) =>
        expression.or([
          expression('backoff.next_attempt_at', 'is', null),
          expression(
            'backoff.next_attempt_at',
            '<=',
            sql<Date>`clock_timestamp()`,
          ),
        ]),
      )
      .orderBy((expression) =>
        expression.fn.coalesce(
          'backoff.next_attempt_at',
          'controls.created_at',
        ),
      )
      .orderBy('controls.decision_id')
      .limit(recoveryDefaults.controlBatch)
      .execute();
  }

  pendingRuns() {
    return queries(this.pool)
      .with('latest', (query) =>
        query
          .selectFrom('service_runs')
          .distinctOn('session_id')
          .selectAll()
          .orderBy('session_id')
          .orderBy('accepted_revision', 'desc'),
      )
      .selectFrom('latest')
      .leftJoinLateral(
        (expression) =>
          expression
            .selectFrom('runtime_events as events')
            .select((event) => [
              sql<string | null>`${event.ref('events.payload')}->>'status'`.as(
                'status',
              ),
              sql<
                string | null
              >`${event.ref('events.payload')}->'delivery_wait'->>'next_wakeup_at'`.as(
                'wakeup',
              ),
            ])
            .whereRef(
              'events.session_id',
              '=',
              sql<string>`${expression.ref('latest.session_id')}::text`,
            )
            .where(
              (event) =>
                sql<string>`${event.ref('events.payload')}->>'root_run_id'`,
              '=',
              sql<string>`${expression.ref('latest.id')}::text`,
            )
            .where('events.event_type', '=', 'agent.state')
            .orderBy('events.commit_seq', 'desc')
            .orderBy('events.event_ordinal', 'desc')
            .limit(1)
            .as('state'),
        (join) => join.onTrue(),
      )
      .leftJoin('runtime_sessions as owner', (join) =>
        join.onRef(
          'owner.session_id',
          '=',
          sql<string>`${sql.ref('latest.session_id')}::text`,
        ),
      )
      .leftJoin('recovery_backoff as backoff', (join) =>
        join
          .on('backoff.kind', '=', RecoveryKind.Run)
          .onRef('backoff.subject_id', '=', 'latest.id'),
      )
      .selectAll('latest')
      .where((expression) =>
        expression.or([
          expression(
            expression.fn.coalesce('state.status', expression.val('accepted')),
            'in',
            [RunStatus.Accepted, RunStatus.Running],
          ),
          expression.and([
            expression('state.status', '=', RunStatus.WaitingDelivery),
            expression(
              expression.fn.coalesce('state.wakeup', expression.val('')),
              '<=',
              sql<string>`to_char(clock_timestamp() AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS.MS"Z"')`,
            ),
          ]),
        ]),
      )
      .where((expression) =>
        expression.or([
          expression('owner.session_id', 'is', null),
          expression('owner.lease_until', '<=', sql<Date>`clock_timestamp()`),
        ]),
      )
      .where('backoff.parked_at', 'is', null)
      .where((expression) =>
        expression.or([
          expression('backoff.next_attempt_at', 'is', null),
          expression(
            'backoff.next_attempt_at',
            '<=',
            sql<Date>`clock_timestamp()`,
          ),
        ]),
      )
      .orderBy(
        (expression) =>
          sql<Date>`coalesce(${expression.ref('backoff.next_attempt_at')}, CASE WHEN ${expression.ref('state.status')}=${RunStatus.WaitingDelivery} THEN ${expression.ref('state.wakeup')}::timestamptz ELSE ${expression.ref('latest.created_at')} END)`,
      )
      .orderBy('latest.id')
      .limit(recoveryDefaults.runBatch)
      .execute();
  }
}
