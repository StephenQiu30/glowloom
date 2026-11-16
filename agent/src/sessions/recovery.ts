import { sql } from 'kysely';
import {
  database,
  queries,
  type DatabaseConnection,
} from '../database/connection.js';
import { recoveryDefaults } from '../config.js';

export const RecoveryKind = { Run: 'run', Control: 'control' } as const;
export const RecoveryCode = {
  ScanUnavailable: 'recovery_scan_unavailable',
  ExecutionUnavailable: 'execution_unavailable',
  ControlUnavailable: 'control_delivery_unavailable',
  DefinitionsUnavailable: 'pinned_definitions_unavailable',
  CloseFailed: 'execution_close_failed',
} as const;
type Kind = (typeof RecoveryKind)[keyof typeof RecoveryKind];
type Code = (typeof RecoveryCode)[keyof typeof RecoveryCode];
export interface RecoveryRecord {
  kind: Kind;
  subject_id: string;
  attempts: number;
  code: Code;
  next_attempt_at: string;
  parked_at: string | null;
}
export interface RecoveryEvent {
  kind: Kind | 'scan';
  code: Code;
  root_run_id?: string;
  subject_id?: string;
  consecutive_failures?: number;
}
/** Backoff metadata for existing runs/controls, never another task executor. */
export class RecoveryStore {
  constructor(private readonly pool: DatabaseConnection) {}

  async failed(
    kind: Kind,
    subject: string,
    root: string,
    code: Code,
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
  async succeeded(kind: Kind, subject: string) {
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
      kind: row.kind as Kind,
      code: row.code as Code,
      next_attempt_at: row.next_attempt_at.toISOString(),
      parked_at: row.parked_at?.toISOString() ?? null,
    }));
  }
}
