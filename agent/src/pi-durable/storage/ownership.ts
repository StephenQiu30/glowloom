import { and, eq, gt, lte, sql } from 'drizzle-orm';
import type { Pool } from 'pg';
import {
  database,
  type DatabaseConnection,
} from '../../database/connection.js';
import { runtimeSessions } from '../../database/schema.js';

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
  owner_epoch: string;
  lease_until: Date;
}
function ownerFrom(row?: LeaseRow): SessionOwner {
  if (!row) throw new Error('Session ownership is unavailable or expired');
  return {
    id: row.owner_id,
    epoch: row.owner_epoch,
    leaseUntil: row.lease_until,
  };
}

const leaseColumns = {
  owner_id: runtimeSessions.owner_id,
  owner_epoch: runtimeSessions.owner_epoch,
  lease_until: runtimeSessions.lease_until,
};
function ownership(sessionId: string, owner: SessionOwner) {
  return and(
    eq(runtimeSessions.session_id, sessionId),
    eq(runtimeSessions.owner_id, owner.id),
    eq(runtimeSessions.owner_epoch, owner.epoch),
    gt(runtimeSessions.lease_until, sql`clock_timestamp()`),
  );
}

export async function claimSessionOwner(
  pool: Pool,
  sessionId: string,
  id: string,
  leaseMs: number,
) {
  validateLease(leaseMs);
  const lease = sql`clock_timestamp() + ${leaseMs} * interval '1 millisecond'`;
  const rows = await database(pool)
    .insert(runtimeSessions)
    .values({
      session_id: sessionId,
      owner_id: id,
      owner_epoch: '1',
      lease_until: lease,
    })
    .onConflictDoUpdate({
      target: runtimeSessions.session_id,
      set: {
        owner_id: id,
        owner_epoch: sql`${runtimeSessions.owner_epoch} + 1`,
        lease_until: lease,
      },
      setWhere: lte(runtimeSessions.lease_until, sql`clock_timestamp()`),
    })
    .returning(leaseColumns);
  return ownerFrom(rows[0]);
}

export async function renewSessionOwner(
  pool: Pool,
  sessionId: string,
  owner: SessionOwner,
  leaseMs: number,
) {
  validateLease(leaseMs);
  const rows = await database(pool)
    .update(runtimeSessions)
    .set({
      lease_until: sql`clock_timestamp() + ${leaseMs} * interval '1 millisecond'`,
    })
    .where(ownership(sessionId, owner))
    .returning(leaseColumns);
  return ownerFrom(rows[0]);
}

export async function releaseSessionOwner(
  pool: Pool,
  sessionId: string,
  owner: SessionOwner,
): Promise<void> {
  await database(pool)
    .update(runtimeSessions)
    .set({ lease_until: sql`clock_timestamp()` })
    .where(ownership(sessionId, owner));
}

export async function assertOwnership(
  client: DatabaseConnection,
  sessionId: string,
  owner: SessionOwner,
) {
  const rows = await database(client)
    .select({ commit_seq: runtimeSessions.commit_seq })
    .from(runtimeSessions)
    .where(ownership(sessionId, owner))
    .for('update');
  if (!rows[0]) throw new Error('Session ownership is unavailable or expired');
  return rows[0].commit_seq;
}
