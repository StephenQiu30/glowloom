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
import {
  database,
  queries,
  type AgentTransaction,
} from '../database/connection.js';
import type { Prisma } from '../database/generated/client.js';
import { decode, indexKey, Refused, type Database } from './records.js';

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
  const alive: Prisma.RuntimeRecordWhereInput =
    at === 'current'
      ? { retired_at_seq: null }
      : {
          created_at_seq: { lte: BigInt(at) },
          OR: [
            { retired_at_seq: null },
            { retired_at_seq: { gt: BigInt(at) } },
          ],
        };
  return decode<DocumentRecord>(
    await database(db).runtimeRecord.findFirst({
      select: { body: true },
      where: {
        session_id: session,
        address_key: addressKey(address),
        ...alive,
      },
    }),
  );
}

export async function document(
  connection: Database,
  session: string,
  id: DocumentId,
  at: DocumentPoint,
): Promise<StoredDocument | undefined> {
  // Keep the record and its latest content in one database snapshot.
  const row = await queries(connection)
    .selectFrom('runtime_records as record')
    .leftJoinLateral(
      (eb) => {
        let latest = eb
          .selectFrom('runtime_document_versions')
          .select(['version', 'content', 'deltas_since_base'])
          .whereRef('session_id', '=', 'record.session_id')
          .whereRef('document_id', '=', 'record.id');
        if (at !== 'current')
          latest = latest.where('commit_seq', '<=', BigInt(at));
        return latest.orderBy('commit_seq', 'desc').limit(1).as('latest');
      },
      (join) => join.onTrue(),
    )
    .select([
      'record.body',
      'latest.version',
      'latest.content',
      'latest.deltas_since_base',
    ])
    .where('record.session_id', '=', session)
    .where('record.id', '=', BigInt(id))
    .where('record.record_type', '=', 'document')
    .executeTakeFirst();
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
  client: AgentTransaction,
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
  client: AgentTransaction,
  session: string,
  seq: Seq,
  writes: readonly StorageWrite[],
) {
  const db = database(client);
  const actions = await actionsFor(client, session, writes);
  // Free addresses first, so simultaneous retirement/recreation is independent of write order.
  for (const [id, action] of actions) {
    if (!action.retire || action.create) continue;
    const current = await document(client, session, id, 'current');
    if (!current) throw new Refused(`Unknown or retired document: ${id}`);
    await db.runtimeRecord.update({
      where: { session_id_id: { session_id: session, id: BigInt(id) } },
      data: { retired_at_seq: BigInt(seq) },
    });
  }
  for (const [id, action] of actions) {
    const { create, content, retire } = action;
    const old = await db.runtimeRecord.findUnique({
      select: { record_type: true, body: true },
      where: { session_id_id: { session_id: session, id: BigInt(id) } },
    });
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
      await db.runtimeRecord.create({
        data: {
          session_id: session,
          id: BigInt(id),
          record_type: 'document',
          body,
          commit_seq: BigInt(seq),
          kind_key: indexKey(record.kind),
          scope_key: scopeKey(record.scope),
          address_key: addressKey(record),
          created_at_seq: BigInt(seq),
          retired_at_seq: retire ? BigInt(seq) : null,
        },
      });
    } else if (retire) {
      await db.runtimeRecord.update({
        where: { session_id_id: { session_id: session, id: BigInt(id) } },
        data: { body },
      });
    }
    if (content) {
      let value,
        deltas = 0;
      if (content.kind === 'base') value = content.value;
      else {
        const latest = await db.runtimeDocumentVersion.findFirst({
          select: { version: true, content: true, deltas_since_base: true },
          where: { session_id: session, document_id: BigInt(id) },
          orderBy: { commit_seq: 'desc' },
        });
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
        await db.runtimeDocumentVersion.deleteMany({
          where: { session_id: session, document_id: BigInt(id) },
        });
      if (!retire || !latestOnly(record))
        await db.runtimeDocumentVersion.create({
          data: {
            session_id: session,
            document_id: BigInt(id),
            commit_seq: BigInt(seq),
            version: content.version,
            content: JSON.stringify(value),
            deltas_since_base: deltas,
          },
        });
    } else if (retire && latestOnly(record)) {
      await db.runtimeDocumentVersion.deleteMany({
        where: { session_id: session, document_id: BigInt(id) },
      });
    }
  }
}
