import {
  BACKGROUND_CONTEXT,
  type Context,
} from '@earendil-works/chord/context';
import {
  ROOT_CONVERSATION_ID,
  type ConversationId,
  type Harness,
  type JsonObject,
} from '@earendil-works/pi-durable';
import type { BusinessClient } from '../../business/client.js';
import { canonicalHash } from '../../sessions/manifest.js';
import { StateDoc } from '../docs.js';

type BusinessSnapshot = Awaited<ReturnType<BusinessClient['snapshot']>>;

export class ProjectMemoryChanged extends Error {
  constructor() {
    super(
      'Project memory changed. Open a new conversation to continue with current project decisions.',
    );
  }
}

export class ProjectEvidenceChanged extends Error {
  constructor() {
    super(
      'Project evidence is no longer current. Start a new conversation and select current sources.',
    );
  }
}

function projectMemoryBasis(snapshot: Readonly<JsonObject>) {
  const version = snapshot.project_memory_version;
  const items = snapshot.project_memory;
  if (version === undefined && items === undefined) return undefined;
  if (
    typeof version !== 'string' ||
    !/^[a-f0-9]{64}$/.test(version) ||
    !Array.isArray(items)
  )
    throw new Error('Invalid project memory snapshot');
  const ids = items.map((item) => {
    if (
      !item ||
      typeof item !== 'object' ||
      Array.isArray(item) ||
      typeof item.id !== 'string'
    )
      throw new Error('Invalid project memory revision');
    return item.id;
  });
  return { version, ids };
}

export async function freezeProjectContext(
  harness: Harness,
  key: string,
  fresh: BusinessSnapshot,
) {
  const snapshotHash = canonicalHash(fresh);
  const frozen = await harness.commit(async (tx) => {
    const state = await tx.doc(StateDoc, ROOT_CONVERSATION_ID);
    const saved = state.model_contexts.find((item) => item.key === key);
    if (saved) {
      // Copy the transaction's draft values before the overlay settles.
      return {
        snapshot: JSON.parse(JSON.stringify(saved.snapshot)) as JsonObject,
        evidenceReceipts: Array.isArray(saved.evidence_receipts)
          ? saved.evidence_receipts.map(String)
          : [],
      };
    }
    if (Buffer.byteLength(JSON.stringify(fresh)) > 131072)
      throw new Error(
        'Business context exceeds the request limit; read specific resources through tools',
      );
    const snapshot = {
      business_snapshot: fresh as JsonObject,
      changed_since_previous: state.context_hash !== snapshotHash,
    };
    const evidenceReceipts = Array.isArray(state.project_evidence_receipts)
      ? state.project_evidence_receipts.map(String)
      : [];
    state.model_contexts.push({
      key,
      snapshot,
      evidence_receipts: evidenceReceipts,
    });
    state.context_hash = snapshotHash;
    return { snapshot, evidenceReceipts };
  }, BACKGROUND_CONTEXT);
  const snapshot = frozen.snapshot.business_snapshot;
  const memory =
    snapshot && typeof snapshot === 'object' && !Array.isArray(snapshot)
      ? projectMemoryBasis(snapshot)
      : undefined;
  return { ...frozen, memoryVersion: memory?.version };
}

export async function assertCurrentProjectMemory(
  harness: Harness,
  frozenVersion: string | undefined,
  fresh: BusinessSnapshot,
) {
  const current = projectMemoryBasis(fresh as JsonObject);
  if (frozenVersion !== current?.version) throw new ProjectMemoryChanged();
  if (!current) return;
  await harness.commit(async (tx) => {
    const state = await tx.doc(StateDoc, ROOT_CONVERSATION_ID);
    const prior = state.project_memory_revision_ids;
    if (
      Array.isArray(prior) &&
      prior.some((id) => !current.ids.includes(String(id)))
    )
      throw new ProjectMemoryChanged();
    state.project_memory_revision_ids = current.ids;
  }, BACKGROUND_CONTEXT);
}

export async function rememberProjectEvidence(
  harness: Harness,
  rootId: ConversationId,
  tool: string,
  receiptId: string,
  result: JsonObject,
  context: Context,
) {
  if (tool !== 'search_project_evidence' && tool !== 'read_project_evidence')
    return;
  if (!Array.isArray(result?.items) || result.items.length === 0) return;
  await harness.commit(async (tx) => {
    const state = await tx.doc(StateDoc, rootId);
    const ids = Array.isArray(state.project_evidence_receipts)
      ? state.project_evidence_receipts
      : [];
    if (ids.includes(receiptId)) return;
    if (ids.length >= 64)
      throw new Error(
        'Project evidence limit reached; start a new conversation',
      );
    state.project_evidence_receipts = [...ids, receiptId];
  }, context);
}
