import * as Y from "yjs";
import { canonicalJson } from "../document/canonical.js";
import { unbase64 } from "../protocol/binary.js";
import { readCanvasDocument } from "../document/model.js";
import { trackIntents, readIntent, applyReadableIntents } from "./intents.js";

const copy = structuredClone;
const equal = (a: any, b: any) => canonicalJson(a) === canonicalJson(b);
export const intentKey = (intent: any) =>
  JSON.stringify([intent.collection, intent.id, intent.field]);
const fail = (code: string) => new Error(code);
const cell = (value: any) =>
  value === undefined
    ? { exists: false }
    : { exists: true, value: copy(value) };

function snapshotOf(initial: any) {
  const doc = new Y.Doc();
  try {
    Y.applyUpdate(doc, unbase64(initial.update));
    return readCanvasDocument(doc);
  } finally {
    doc.destroy();
  }
}

/** A device-local atomic handoff, never a binary replay into a different epoch. */
export function archiveRecovery(record: any, initial: any) {
  if (record.inFlight || record.control) throw fail("ORIGINAL_UNCONFIRMED");
  const sources = copy(record.recoverySources ?? []);
  const privateInput =
    record.sealed ||
    record.pending.length ||
    record.intents.length ||
    Object.keys(record.forms).length ||
    Object.keys(record.textDrafts).length;
  const attemptSource = sources.find(
    (source: any) => source.id === record.recoverySelection?.sourceId,
  );
  const selectedKeys = new Set(
    record.recoverySelection?.mappings.map((mapping: any) => mapping.targetKey) ??
      [],
  );
  const onlySelectedAttempt =
    attemptSource &&
    record.sealed &&
    !Object.keys(record.forms).length &&
    !Object.keys(record.textDrafts).length &&
    (record.sealed.intents ?? []).every((intent: any) =>
      selectedKeys.has(intentKey(intent)),
    );
  if (onlySelectedAttempt) {
    attemptSource.lastAttempt = {
      scope: copy(record.scope),
      confirmed: copy(record.confirmed),
      sealed: copy(record.sealed),
      forms: copy(record.forms),
      textDrafts: copy(record.textDrafts),
    };
  } else if (privateInput) {
    if (sources.length >= 16) throw fail("RECOVERY_SOURCE_BUDGET");
    const sealed = copy(
      record.sealed ?? {
        reason: "EXPLICIT_RECOVERY",
        snapshot: record.snapshot,
        inFlight: null,
        pending: record.pending,
        intents: record.intents,
      },
    );
    sealed.intents ??= copy([...record.inFlightIntents, ...record.intents]);
    if (!sealed.intents.length && record.pending.length) {
      sealed.intents = trackIntents(
        [],
        snapshotOf(record.confirmed),
        sealed.snapshot,
        record.generation,
      );
    }
    sources.push({
      id: crypto.randomUUID(),
      generation: record.generation,
      scope: copy(record.scope),
      confirmed: copy(record.confirmed),
      sealed,
      forms: copy(record.forms),
      textDrafts: copy(record.textDrafts),
      textGenerations: {},
      locations: {},
    });
  }
  return {
    version: 2,
    generation: record.generation + 1,
    scope: { ...record.scope, epoch: initial.epoch },
    confirmed: copy(initial),
    inFlight: null,
    acceptedReceipt: null,
    recoverySources: sources,
    recoverySelection: null,
    intents: [],
    inFlightIntents: [],
    automaticReplayAttempts: 0,
    pending: [],
    snapshot: snapshotOf(initial),
    sealed: null,
    forms: {},
    textDrafts: {},
    control: null,
    updatedAt: Date.now(),
  };
}

/** Immutable review values are compared again immediately before applying a selection. */
export function recoveryFields(source: any, current: any) {
  const intents = new Map<
    string,
    import("../../types/session.js").SemanticIntent
  >(
    (source.sealed.intents ?? []).map((intent: any) => [
      intentKey(intent),
      copy(intent),
    ]),
  );
  for (const [id, document] of Object.entries(
    source.textDrafts as import("../../types/session.js").RecoverySource["textDrafts"],
  )) {
    const identity = {
      collection: "nodes" as const,
      id,
      field: "textDocument",
    };
    intents.set(intentKey(identity), {
      ...identity,
      generation: source.textGenerations?.[id] ?? source.generation,
      before: readIntent(source.sealed.snapshot, identity),
      after: cell(document),
    });
  }
  for (const [id, form] of Object.entries(source.forms)) {
    const identity = { collection: "nodes" as const, id, field: "form" };
    intents.set(intentKey(identity), {
      ...identity,
      generation: source.generation,
      before: cell(undefined),
      after: cell(form),
    });
  }
  return [...intents.values()].map((intent: any) => {
    const targetId = source.locations[intent.id] ?? intent.id;
    const target = { ...intent, id: targetId };
    const alive = current.graph.nodes.some((node: any) => node.id === targetId);
    return {
      ...intent,
      key: intentKey(intent),
      targetId,
      current:
        intent.field === "form" ? cell(undefined) : readIntent(current, target),
      copyRequired:
        intent.collection === "nodes" &&
        intent.after.exists &&
        (!alive || intent.field === "entity"),
    };
  });
}

function remapNode(node: any, locations: any, alive: any) {
  const result = copy(node);
  result.id = locations[node.id] ?? node.id;
  if (result.parentId) {
    const parent = locations[result.parentId] ?? result.parentId;
    if (!alive.has(parent)) delete result.parentId;
    else result.parentId = parent;
  }
  const visit = (value: any) => {
    if (!value || typeof value !== "object") return;
    if (value.type === "mention" && typeof value.attrs?.id === "string") {
      const old = value.attrs.id;
      if (locations[old]) value.attrs.id = locations[old];
      else if (old.startsWith("canvas:") && locations[old.slice(7)])
        value.attrs.id = "canvas:" + locations[old.slice(7)];
    }
    if (Array.isArray(value.content)) value.content.forEach(visit);
  };
  visit(result.textDocument?.content);
  return result;
}

export function planRecovery(source: any, current: any, selected: any[], allocations: any = {}) {
  if (!selected.length) throw fail("RECOVERY_SELECTION_EMPTY");
  const actual = new Map<
    string,
    import("../../types/recovery.js").RecoveryField
  >(recoveryFields(source, current).map((field: any) => [field.key, field]));
  const keys = new Set();
  for (const chosen of selected) {
    const field = actual.get(chosen.key);
    if (
      keys.has(chosen.key) ||
      !field ||
      field.generation !== chosen.generation ||
      !equal(field.after, chosen.after) ||
      !equal(field.current, chosen.current) ||
      field.targetId !== chosen.targetId
    ) {
      throw fail("RECOVERY_REVIEW_CHANGED");
    }
    if (field.copyRequired && !chosen.copyNode)
      throw fail("RECOVERY_COPY_REQUIRED");
    keys.add(chosen.key);
  }
  const snapshot = copy(current),
    locations = copy(source.locations),
    mappings: import("../../types/session.js").RecoverySelection["mappings"] =
      [],
    forms: Record<string, any> = {};
  const copied = new Set(),
    retainedNodes: ({ id: string; textDocument?: unknown } & Record<
      string,
      unknown
    >)[] = [];
  for (const chosen of selected) {
    const field = actual.get(chosen.key)!;
    if (field.copyRequired && !copied.has(field.id)) {
      const node = source.sealed.snapshot.graph.nodes.find(
        (value: any) => value.id === field.id,
      );
      if (!node) throw fail("RECOVERY_NODE_UNAVAILABLE");
      locations[field.id] = allocations[field.id] ?? crypto.randomUUID();
      copied.add(field.id);
      const retained = copy(node);
      if (source.textDrafts[field.id])
        retained.textDocument = copy(source.textDrafts[field.id]);
      retainedNodes.push(retained);
    }
  }
  const alive = new Set([
    ...snapshot.graph.nodes.map((node: any) => node.id),
    ...retainedNodes.map((node: any) => locations[node.id] ?? node.id),
  ]);
  snapshot.graph.nodes.push(
    ...retainedNodes.map((node: any) => remapNode(node, locations, alive)),
  );
  const changes: import("../../types/session.js").SemanticIntent[] = [];
  for (const chosen of selected) {
    const field = actual.get(chosen.key)!,
      targetId = locations[field.id] ?? field.id;
    let targetKey;
    if (field.field === "form") {
      if (!alive.has(targetId)) throw fail("RECOVERY_TARGET_DELETED");
      forms[targetId] = copy(field.after.value);
      targetKey = JSON.stringify([
        "nodes",
        targetId,
        copied.has(field.id) ? "entity" : "form",
      ]);
    } else if (copied.has(field.id) && field.collection === "nodes") {
      targetKey = JSON.stringify(["nodes", targetId, "entity"]);
    } else {
      const intent = { ...field, id: targetId };
      if (field.collection === "connections" && field.after.exists) {
        intent.id =
          allocations["connection:" + field.id] ?? crypto.randomUUID();
        locations["connection:" + field.id] = intent.id;
        intent.after = copy(field.after);
        const edge = intent.after.value as Record<string, string>;
        edge.id = intent.id;
        for (const name of ["fromNodeId", "toNodeId"]) {
          const id = locations[edge[name]] ?? edge[name];
          if (!alive.has(id)) throw fail("RECOVERY_TARGET_DELETED");
          edge[name] = id;
        }
      }
      changes.push(intent);
      targetKey = intentKey(intent);
    }
    mappings.push({
      targetKey,
      sourceKeys: [{ key: field.key, generation: field.generation }],
    });
  }
  return {
    snapshot: applyReadableIntents(snapshot, changes),
    forms,
    selection: { sourceId: source.id, updateId: null, mappings, locations },
  };
}

/** Only the original accepted batch can release its selected generations. */
export function settleRecovery(sources: any, selection: any, unapplied: any[], verified: any) {
  const result = copy(sources),
    source = result.find((value: any) => value.id === selection.sourceId);
  if (!source || !verified) return result;
  const lost = new Set(unapplied.map(intentKey));
  const clear = new Set();
  for (const mapping of selection.mappings) {
    if (lost.has(mapping.targetKey)) continue;
    for (const original of mapping.sourceKeys) {
      const field = recoveryFields(source, source.sealed.snapshot).find(
        (value: any) => value.key === original.key,
      );
      if (field?.generation === original.generation) clear.add(original.key);
    }
  }
  source.sealed.intents = (source.sealed.intents ?? []).filter(
    (intent: any) => !clear.has(intentKey(intent)),
  );
  for (const id of Object.keys(source.textDrafts))
    if (clear.has(JSON.stringify(["nodes", id, "textDocument"])))
      delete source.textDrafts[id];
  for (const id of Object.keys(source.forms))
    if (clear.has(JSON.stringify(["nodes", id, "form"])))
      delete source.forms[id];
  source.locations = { ...source.locations, ...selection.locations };
  return result;
}
