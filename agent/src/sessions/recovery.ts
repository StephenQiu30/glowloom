import { and, asc, eq, isNotNull, sql } from 'drizzle-orm';
import { database, type DatabaseConnection } from '../database/connection.js';
import { recoveryBackoff } from '../database/schema.js';
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
    await database(this.pool)
      .insert(recoveryBackoff)
      .values({
        kind,
        subject_id: subject,
        root_run_id: root,
        attempts: 1,
        code,
        next_attempt_at: sql`clock_timestamp()+${recoveryDefaults.pollMs}*interval '1 millisecond'`,
        parked_at: terminal ? sql`clock_timestamp()` : null,
      })
      .onConflictDoUpdate({
        target: [recoveryBackoff.kind, recoveryBackoff.subject_id],
        set: {
          attempts: sql`${recoveryBackoff.attempts}+1`,
          code,
          next_attempt_at: sql`clock_timestamp()+least(${recoveryDefaults.maxBackoffMs}, ${recoveryDefaults.pollMs}*power(2,least(${recoveryBackoff.attempts},16)))*interval '1 millisecond'`,
          parked_at: sql`case when ${terminal} or ${recoveryBackoff.attempts}+1 >= ${recoveryDefaults.maxAttempts} then coalesce(${recoveryBackoff.parked_at},clock_timestamp()) else null end`,
        },
      });
  }
  async succeeded(kind: Kind, subject: string) {
    await database(this.pool)
      .update(recoveryBackoff)
      .set({
        attempts: 0,
        code: null,
        parked_at: null,
        next_attempt_at: sql`clock_timestamp()`,
      })
      .where(
        and(
          eq(recoveryBackoff.kind, kind),
          eq(recoveryBackoff.subject_id, subject),
        ),
      );
  }
  async read(root: string): Promise<RecoveryRecord[]> {
    const rows = await database(this.pool)
      .select({
        kind: recoveryBackoff.kind,
        subject_id: recoveryBackoff.subject_id,
        attempts: recoveryBackoff.attempts,
        code: recoveryBackoff.code,
        next_attempt_at: recoveryBackoff.next_attempt_at,
        parked_at: recoveryBackoff.parked_at,
      })
      .from(recoveryBackoff)
      .where(
        and(
          eq(recoveryBackoff.root_run_id, root),
          isNotNull(recoveryBackoff.code),
        ),
      )
      .orderBy(asc(recoveryBackoff.kind), asc(recoveryBackoff.subject_id));
    return rows.map((row) => ({
      ...row,
      kind: row.kind as Kind,
      code: row.code as Code,
      next_attempt_at: row.next_attempt_at.toISOString(),
      parked_at: row.parked_at?.toISOString() ?? null,
    }));
  }
}
