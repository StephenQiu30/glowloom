import type { JsonObject } from '@earendil-works/pi-durable';
import {
  database,
  type DatabaseConnection,
} from '../../database/connection.js';
import { publicState } from '../../pi-durable/events.js';
import { AgentError } from '../errors.js';
import type { RuntimeEvent } from '../types.js';
import type { RunRow } from './store.js';

export async function readRunEvents(
  pool: DatabaseConnection,
  run: RunRow,
  after: string | undefined,
): Promise<RuntimeEvent[]> {
  const match = /^(0|[1-9]\d*):(0|[1-9]\d*)$/.exec(after ?? '0:0');
  if (!match) throw new AgentError('invalid_event_cursor');
  const seq = BigInt(match[1]!);
  const ordinal = BigInt(match[2]!);
  if (seq > 9007199254740991n || ordinal > 2147483647n)
    throw new AgentError('invalid_event_cursor');
  const aggregate = await database(pool).runtimeEvent.aggregate({
    where: { session_id: run.session_id },
    _min: { commit_seq: true },
  });
  if (seq > 0n && aggregate._min.commit_seq && seq < aggregate._min.commit_seq)
    throw new AgentError('snapshot_required');
  const result = await database(pool).runtimeEvent.findMany({
    select: {
      commit_seq: true,
      event_ordinal: true,
      event_type: true,
      payload: true,
    },
    where: {
      session_id: run.session_id,
      OR: [
        { commit_seq: { gt: seq } },
        { commit_seq: seq, event_ordinal: { gt: Number(ordinal) } },
      ],
      payload: { path: ['root_run_id'], equals: run.id },
    },
    orderBy: [{ commit_seq: 'asc' }, { event_ordinal: 'asc' }],
    take: 100,
  });
  return result.map((row) => ({
    cursor: `${row.commit_seq}:${row.event_ordinal}`,
    type: row.event_type,
    payload:
      row.event_type === 'agent.state'
        ? publicState(row.payload as JsonObject)
        : row.payload,
  }));
}
