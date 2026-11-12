import type { Pool, PoolClient } from 'pg';
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
  constructor(private readonly pool: Pool | PoolClient) {}

  async failed(
    kind: Kind,
    subject: string,
    root: string,
    code: Code,
    terminal = false,
  ) {
    await this.pool.query(
      `INSERT INTO agent.recovery_backoff(kind,subject_id,root_run_id,attempts,code,next_attempt_at,parked_at)
       VALUES($1,$2,$3,1,$4,clock_timestamp()+$5*interval '1 millisecond',CASE WHEN $7 THEN clock_timestamp() END)
       ON CONFLICT(kind,subject_id) DO UPDATE SET
       attempts=agent.recovery_backoff.attempts+1,code=EXCLUDED.code,
       next_attempt_at=clock_timestamp()+least($6,$5*power(2,least(agent.recovery_backoff.attempts,16)))*interval '1 millisecond',
       parked_at=CASE WHEN $7 OR agent.recovery_backoff.attempts+1 >= $8 THEN coalesce(agent.recovery_backoff.parked_at,clock_timestamp()) ELSE NULL END`,
      [
        kind,
        subject,
        root,
        code,
        recoveryDefaults.pollMs,
        recoveryDefaults.maxBackoffMs,
        terminal,
        recoveryDefaults.maxAttempts,
      ],
    );
  }
  async succeeded(kind: Kind, subject: string) {
    await this.pool.query(
      'UPDATE agent.recovery_backoff SET attempts=0,code=NULL,parked_at=NULL,next_attempt_at=clock_timestamp() WHERE kind=$1 AND subject_id=$2',
      [kind, subject],
    );
  }
  async read(root: string): Promise<RecoveryRecord[]> {
    return (
      await this.pool.query<RecoveryRecord>(
        `SELECT kind,subject_id,attempts,code,
       to_char(next_attempt_at AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS.MS"Z"') next_attempt_at,
       to_char(parked_at AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS.MS"Z"') parked_at
       FROM agent.recovery_backoff WHERE root_run_id=$1 AND code IS NOT NULL ORDER BY kind,subject_id`,
        [root],
      )
    ).rows;
  }
}
