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
import { and, desc, eq, gt, isNull, lte, or, sql } from 'drizzle-orm';
import { database, type AgentTransaction } from '../../database/connection.js';
import {
  runtimeRecords,
  runtimeDocumentVersions,
} from '../../database/schema.js';
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
  const alive =
    at === 'current'
      ? isNull(runtimeRecords.retired_at_seq)
      : and(
          lte(runtimeRecords.created_at_seq, at),
          or(
            isNull(runtimeRecords.retired_at_seq),
            gt(runtimeRecords.retired_at_seq, at),
          ),
        );
  return decode<DocumentRecord>(
    (
      await database(db)
        .select({ body: runtimeRecords.body })
        .from(runtimeRecords)
        .where(
          and(
            eq(runtimeRecords.session_id, session),
            eq(runtimeRecords.address_key, addressKey(address)),
            alive,
          ),
        )
    )[0],
  );
}

export async function document(
  connection: Database,
  session: string,
  id: DocumentId,
  at: DocumentPoint,
): Promise<StoredDocument | undefined> {
  const db = database(connection);
  const latest = db
    .select({
      version: runtimeDocumentVersions.version,
      content: runtimeDocumentVersions.content,
      deltas_since_base: runtimeDocumentVersions.deltas_since_base,
    })
    .from(runtimeDocumentVersions)
    .where(
      and(
        eq(runtimeDocumentVersions.session_id, runtimeRecords.session_id),
        eq(runtimeDocumentVersions.document_id, runtimeRecords.id),
        at === 'current'
          ? undefined
          : lte(runtimeDocumentVersions.commit_seq, at),
      ),
    )
    .orderBy(desc(runtimeDocumentVersions.commit_seq))
    .limit(1)
    .as('latest');
  const row = (
    await db
      .select({
        body: runtimeRecords.body,
        version: latest.version,
        content: latest.content,
        deltas_since_base: latest.deltas_since_base,
      })
      .from(runtimeRecords)
      .leftJoinLateral(latest, sql`true`)
      .where(
        and(
          eq(runtimeRecords.session_id, session),
          eq(runtimeRecords.id, id),
          eq(runtimeRecords.record_type, 'document'),
        ),
      )
  )[0];
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
  const actions = await actionsFor(client, session, writes);
  // Free addresses first, so simultaneous retirement/recreation is independent of write order.
  for (const [id, action] of actions) {
    if (!action.retire || action.create) continue;
    const current = await document(client, session, id, 'current');
    if (!current) throw new Refused(`Unknown or retired document: ${id}`);
    await client
      .update(runtimeRecords)
      .set({ retired_at_seq: seq })
      .where(
        and(eq(runtimeRecords.session_id, session), eq(runtimeRecords.id, id)),
      );
  }
  for (const [id, action] of actions) {
    const { create, content, retire } = action;
    const old = (
      await client
        .select({
          record_type: runtimeRecords.record_type,
          body: runtimeRecords.body,
        })
        .from(runtimeRecords)
        .where(
          and(
            eq(runtimeRecords.session_id, session),
            eq(runtimeRecords.id, id),
          ),
        )
    )[0];
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
      await client.insert(runtimeRecords).values({
        session_id: session,
        id,
        record_type: 'document',
        body,
        commit_seq: seq,
        kind_key: indexKey(record.kind),
        scope_key: scopeKey(record.scope),
        address_key: addressKey(record),
        created_at_seq: seq,
        retired_at_seq: retire ? seq : null,
      });
    } else if (retire) {
      await client
        .update(runtimeRecords)
        .set({ body })
        .where(
          and(
            eq(runtimeRecords.session_id, session),
            eq(runtimeRecords.id, id),
          ),
        );
    }
    if (content) {
      let value,
        deltas = 0;
      if (content.kind === 'base') value = content.value;
      else {
        const latest = (
          await client
            .select({
              version: runtimeDocumentVersions.version,
              content: runtimeDocumentVersions.content,
              deltas_since_base: runtimeDocumentVersions.deltas_since_base,
            })
            .from(runtimeDocumentVersions)
            .where(
              and(
                eq(runtimeDocumentVersions.session_id, session),
                eq(runtimeDocumentVersions.document_id, id),
              ),
            )
            .orderBy(desc(runtimeDocumentVersions.commit_seq))
            .limit(1)
        )[0];
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
        await client
          .delete(runtimeDocumentVersions)
          .where(
            and(
              eq(runtimeDocumentVersions.session_id, session),
              eq(runtimeDocumentVersions.document_id, id),
            ),
          );
      if (!retire || !latestOnly(record))
        await client.insert(runtimeDocumentVersions).values({
          session_id: session,
          document_id: id,
          commit_seq: seq,
          version: content.version,
          content: JSON.stringify(value),
          deltas_since_base: deltas,
        });
    } else if (retire && latestOnly(record)) {
      await client
        .delete(runtimeDocumentVersions)
        .where(
          and(
            eq(runtimeDocumentVersions.session_id, session),
            eq(runtimeDocumentVersions.document_id, id),
          ),
        );
    }
  }
}
