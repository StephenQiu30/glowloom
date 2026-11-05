import type { Pool, PoolClient } from 'pg';

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

export async function claimSessionOwner(
  pool: Pool,
  sessionId: string,
  id: string,
  leaseMs: number,
) {
  validateLease(leaseMs);
  const result = await pool.query<LeaseRow>(
    `
    INSERT INTO agent.runtime_sessions AS s (session_id,owner_id,owner_epoch,lease_until)
    VALUES ($1,$2,1,clock_timestamp() + $3 * interval '1 millisecond')
    ON CONFLICT(session_id) DO UPDATE SET owner_id=$2, owner_epoch=s.owner_epoch+1,
      lease_until=clock_timestamp() + $3 * interval '1 millisecond'
    WHERE s.lease_until <= clock_timestamp()
    RETURNING owner_id,owner_epoch,lease_until`,
    [sessionId, id, leaseMs],
  );
  return ownerFrom(result.rows[0]);
}

export async function renewSessionOwner(
  pool: Pool,
  sessionId: string,
  owner: SessionOwner,
  leaseMs: number,
) {
  validateLease(leaseMs);
  const result = await pool.query<LeaseRow>(
    `UPDATE agent.runtime_sessions SET lease_until=clock_timestamp()+ $4 * interval '1 millisecond'
    WHERE session_id=$1 AND owner_id=$2 AND owner_epoch=$3 AND lease_until>clock_timestamp()
    RETURNING owner_id,owner_epoch,lease_until`,
    [sessionId, owner.id, owner.epoch, leaseMs],
  );
  return ownerFrom(result.rows[0]);
}

export async function releaseSessionOwner(
  pool: Pool,
  sessionId: string,
  owner: SessionOwner,
): Promise<void> {
  await pool.query(
    `UPDATE agent.runtime_sessions SET lease_until=clock_timestamp()
    WHERE session_id=$1 AND owner_id=$2 AND owner_epoch=$3 AND lease_until>clock_timestamp()`,
    [sessionId, owner.id, owner.epoch],
  );
}

export async function assertOwnership(
  client: Pool | PoolClient,
  sessionId: string,
  owner: SessionOwner,
) {
  const result = await client.query<{ commit_seq: string }>(
    `SELECT commit_seq FROM agent.runtime_sessions
    WHERE session_id=$1 AND owner_id=$2 AND owner_epoch=$3 AND lease_until>clock_timestamp() FOR UPDATE`,
    [sessionId, owner.id, owner.epoch],
  );
  if (!result.rows[0])
    throw new Error('Session ownership is unavailable or expired');
  return Number(result.rows[0].commit_seq);
}
