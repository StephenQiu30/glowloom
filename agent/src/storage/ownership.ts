import { sql } from 'kysely';
import type { Pool } from 'pg';
import { queries, type DatabaseConnection } from '../database/connection.js';

export interface SessionOwner {
  readonly id: string;
  readonly epoch: string;
  readonly leaseUntil: Date;
}

function validateLease(ms: number) {
  if (!Number.isSafeInteger(ms) || ms < 1 || ms > 300_000)
    throw new RangeError('Invalid owner lease length');
}

interface LeaseRow {
  owner_id: string;
  owner_epoch: bigint;
  lease_until: Date;
}
function ownerFrom(row?: LeaseRow): SessionOwner {
  if (!row) throw new Error('Session ownership is unavailable or expired');
  return {
    id: row.owner_id,
    epoch: row.owner_epoch.toString(),
    leaseUntil: row.lease_until,
  };
}

const leaseColumns = ['owner_id', 'owner_epoch', 'lease_until'] as const;

export async function claimSessionOwner(
  pool: Pool,
  sessionId: string,
  id: string,
  leaseMs: number,
) {
  validateLease(leaseMs);
  const lease = sql<Date>`clock_timestamp() + ${leaseMs} * interval '1 millisecond'`;
  const row = await queries(pool)
    .insertInto('runtime_sessions')
    .values({
      session_id: sessionId,
      owner_id: id,
      owner_epoch: 1n,
      lease_until: lease,
    })
    .onConflict((conflict) =>
      conflict
        .column('session_id')
        .doUpdateSet((eb) => ({
          owner_id: id,
          owner_epoch: eb('runtime_sessions.owner_epoch', '+', 1n),
          lease_until: lease,
        }))
        .where(
          'runtime_sessions.lease_until',
          '<=',
          sql<Date>`clock_timestamp()`,
        ),
    )
    .returning(leaseColumns)
    .executeTakeFirst();
  return ownerFrom(row);
}

export async function renewSessionOwner(
  pool: Pool,
  sessionId: string,
  owner: SessionOwner,
  leaseMs: number,
) {
  validateLease(leaseMs);
  const row = await queries(pool)
    .updateTable('runtime_sessions')
    .set({
      lease_until: sql<Date>`clock_timestamp() + ${leaseMs} * interval '1 millisecond'`,
    })
    .where('session_id', '=', sessionId)
    .where('owner_id', '=', owner.id)
    .where('owner_epoch', '=', BigInt(owner.epoch))
    .where('lease_until', '>', sql<Date>`clock_timestamp()`)
    .returning(leaseColumns)
    .executeTakeFirst();
  return ownerFrom(row);
}

export async function releaseSessionOwner(
  pool: Pool,
  sessionId: string,
  owner: SessionOwner,
): Promise<void> {
  await queries(pool)
    .updateTable('runtime_sessions')
    .set({ lease_until: sql<Date>`clock_timestamp()` })
    .where('session_id', '=', sessionId)
    .where('owner_id', '=', owner.id)
    .where('owner_epoch', '=', BigInt(owner.epoch))
    .where('lease_until', '>', sql<Date>`clock_timestamp()`)
    .execute();
}

export async function assertOwnership(
  client: DatabaseConnection,
  sessionId: string,
  owner: SessionOwner,
) {
  const row = await queries(client)
    .selectFrom('runtime_sessions')
    .select('commit_seq')
    .where('session_id', '=', sessionId)
    .where('owner_id', '=', owner.id)
    .where('owner_epoch', '=', BigInt(owner.epoch))
    .where('lease_until', '>', sql<Date>`clock_timestamp()`)
    .forUpdate()
    .executeTakeFirst();
  if (!row) throw new Error('Session ownership is unavailable or expired');
  return row.commit_seq;
}
