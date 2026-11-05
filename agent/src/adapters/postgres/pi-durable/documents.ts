import { applyImmutableBatches } from '@earendil-works/chord/delta';
import {
  StorageRequestError,
  type DocumentAddress,
  type DocumentContent,
  type DocumentCreate,
  type DocumentId,
  type DocumentPoint,
  type DocumentRecord,
  type Seq,
  type StorageWrite,
  type StoredDocument,
} from '@earendil-works/pi-durable';
import type { PoolClient } from 'pg';
import {
  decode,
  indexKey,
  Refused,
  type BodyRow,
  type Database,
} from './records.js';

export function scopeKey(scope: DocumentAddress['scope']): string {
  return JSON.stringify(
    scope.kind === 'session'
      ? ['session']
      : scope.kind === 'task'
        ? ['task', scope.taskId]
        : ['conversation', scope.conversationId],
  );
}
export function addressKey(record: DocumentAddress | DocumentCreate): string {
  return JSON.stringify([
    scopeKey(record.scope),
    record.kind,
    record.key ?? null,
  ]);
}
function latestOnly(record: DocumentRecord) {
  return record.scope.kind !== 'conversation' || record.history === 'latest';
}
export async function findDocument(
  db: Database,
  session: string,
  address: DocumentAddress,
  at: DocumentPoint,
) {
  const args =
    at === 'current'
      ? [session, addressKey(address)]
      : [session, addressKey(address), at];
  const alive =
    at === 'current'
      ? 'retired_at_seq IS NULL'
      : 'created_at_seq <= $3 AND (retired_at_seq IS NULL OR retired_at_seq > $3)';
  return decode<DocumentRecord>(
    (
      await db.query<BodyRow>(
        `SELECT body FROM agent.runtime_records WHERE session_id=$1 AND address_key=$2 AND ${alive}`,
        args,
      )
    ).rows[0],
  );
}

export async function document(
  db: Database,
  session: string,
  id: DocumentId,
  at: DocumentPoint,
): Promise<StoredDocument | undefined> {
  type VersionRow = BodyRow & {
    version: number | null;
    content: string | null;
    deltas_since_base: number | null;
  };
  const result = await db.query<VersionRow>(
    `SELECT r.body, v.version,v.content,v.deltas_since_base FROM agent.runtime_records r
    LEFT JOIN LATERAL (SELECT version,content,deltas_since_base FROM agent.runtime_document_versions
      WHERE session_id=r.session_id AND document_id=r.id AND ($3::bigint IS NULL OR commit_seq<=$3)
      ORDER BY commit_seq DESC LIMIT 1) v ON true
    WHERE r.session_id=$1 AND r.id=$2 AND r.record_type='document'`,
    [session, id, at === 'current' ? null : at],
  );
  const row = result.rows[0];
  const record = decode<DocumentRecord>(row);
  if (!row || !record) return undefined;
  if (at !== 'current' && latestOnly(record))
    throw new StorageRequestError(
      `Document ${id} does not retain historical content`,
    );
  if (
    at === 'current'
      ? record.retiredAt !== undefined
      : record.createdAt > at ||
        (record.retiredAt !== undefined && record.retiredAt <= at)
  )
    return undefined;
  if (
    row.content === null ||
    row.version === null ||
    row.deltas_since_base === null
  )
    throw new Error(`Missing version for document ${id}`);
  return {
    record,
    version: row.version,
    value: JSON.parse(row.content),
    deltasSinceBase: row.deltas_since_base,
  };
}

type Action = {
  create?: DocumentCreate;
  content?: DocumentContent;
  retire: boolean;
};
async function actionsFor(
  client: PoolClient,
  session: string,
  writes: readonly StorageWrite[],
) {
  const actions = new Map<DocumentId, Action>();
  for (const w of writes) {
    const id = 'record' in w ? w.record.id : 'id' in w ? w.id : undefined;
    if (id === undefined) continue;
    const action = actions.get(id) ?? { retire: false };
    actions.set(id, action);
    if (w.type === 'document.retire') {
      if (action.retire)
        throw new Refused(`Document ${id} is retired more than once`);
      action.retire = true;
      continue;
    }
    if (action.content !== undefined)
      throw new Refused(`Document ${id} has more than one content command`);
    if (w.type === 'document.create' || w.type === 'document.change')
      action.content = w.content;
    if (w.type === 'document.create' || w.type === 'document.copy')
      action.create = w.record;
  }
  for (const w of writes) {
    if (w.type !== 'document.copy') continue;
    const action = actions.get(w.record.id)!;
    const reject = () =>
      new Refused(`Document copy ${w.record.id} was rejected`);
    if (actions.has(w.source.id) || action.content) throw reject();
    let source;
    try {
      source = await document(client, session, w.source.id, w.source.at);
    } catch (e) {
      if (e instanceof StorageRequestError) throw reject();
      throw e;
    }
    if (
      !source ||
      source.record.scope.kind !== 'conversation' ||
      w.record.scope.kind !== 'conversation' ||
      source.record.kind !== w.record.kind ||
      source.record.key !== w.record.key ||
      source.record.history !== w.record.history ||
      source.record.fork !== w.record.fork
    )
      throw reject();
    action.content = {
      kind: 'base',
      version: source.version,
      value: source.value,
    };
  }
  return actions;
}

export async function writeDocuments(
  client: PoolClient,
  session: string,
  seq: Seq,
  writes: readonly StorageWrite[],
) {
  const actions = await actionsFor(client, session, writes);
  // Free addresses first, so simultaneous retirement/recreation is independent of write order.
  for (const [id, action] of actions) {
    if (!action.retire || action.create) continue;
    const current = await document(client, session, id, 'current');
    if (!current) throw new Refused(`Unknown or retired document: ${id}`);
    await client.query(
      'UPDATE agent.runtime_records SET retired_at_seq=$3 WHERE session_id=$1 AND id=$2',
      [session, id, seq],
    );
  }
  for (const [id, action] of actions) {
    const { create, content, retire } = action;
    const prior = await client.query<{ record_type: string; body: string }>(
      'SELECT record_type, body FROM agent.runtime_records WHERE session_id=$1 AND id=$2',
      [session, id],
    );
    const old = prior.rows[0];
    if (create && old)
      throw new Refused(`ID ${id} already belongs to ${old.record_type}`);
    if (!create && old?.record_type !== 'document')
      throw new Refused(`Unknown document: ${id}`);
    const previousRecord = decode<DocumentRecord>(old);
    if (previousRecord?.retiredAt !== undefined)
      throw new Refused(`Document ${id} is retired`);
    const record: DocumentRecord = create
      ? { ...create, createdAt: seq }
      : previousRecord!;
    const body = JSON.stringify(
      retire ? { ...record, retiredAt: seq } : record,
    );
    if (create) {
      if (!retire && (await findDocument(client, session, create, 'current')))
        throw new Refused('Document address already has a current incarnation');
      await client.query(
        `INSERT INTO agent.runtime_records(session_id,id,record_type,body,commit_seq,kind_key,scope_key,address_key,created_at_seq,retired_at_seq)
        VALUES($1,$2,'document',$3,$4,$5,$6,$7,$4,$8)`,
        [
          session,
          id,
          body,
          seq,
          indexKey(record.kind),
          scopeKey(record.scope),
          addressKey(record),
          retire ? seq : null,
        ],
      );
    } else if (retire) {
      await client.query(
        'UPDATE agent.runtime_records SET body=$3 WHERE session_id=$1 AND id=$2',
        [session, id, body],
      );
    }
    if (content) {
      let value,
        deltas = 0;
      if (content.kind === 'base') value = content.value;
      else {
        const latest = (
          await client.query<{
            version: number;
            content: string;
            deltas_since_base: number;
          }>(
            `SELECT version,content,deltas_since_base FROM agent.runtime_document_versions
          WHERE session_id=$1 AND document_id=$2 ORDER BY commit_seq DESC LIMIT 1`,
            [session, id],
          )
        ).rows[0];
        if (!latest) throw new Refused(`Document ${id} delta has no base`);
        if (latest.version !== content.version)
          throw new Refused(
            `Document ${id} version transition requires a base`,
          );
        value = applyImmutableBatches(JSON.parse(latest.content), [
          content.ops,
        ]);
        deltas = latest.deltas_since_base + 1;
      }
      if (latestOnly(record))
        await client.query(
          'DELETE FROM agent.runtime_document_versions WHERE session_id=$1 AND document_id=$2',
          [session, id],
        );
      if (!retire || !latestOnly(record))
        await client.query(
          `INSERT INTO agent.runtime_document_versions(session_id,document_id,commit_seq,version,content,deltas_since_base)
        VALUES($1,$2,$3,$4,$5,$6)`,
          [session, id, seq, content.version, JSON.stringify(value), deltas],
        );
    } else if (retire && latestOnly(record)) {
      await client.query(
        'DELETE FROM agent.runtime_document_versions WHERE session_id=$1 AND document_id=$2',
        [session, id],
      );
    }
  }
}
