import * as Y from "yjs";
import { AdmissionGate, digest } from "./admission.mjs";
import { canonicalJson } from "./canonical.mjs";
import {
  createCanvasDocument,
  readCanvasDocument,
  inspectCanvasDocument,
  validateCanvasTransition,
} from "./model.mjs";
import { validateCanvasRoots } from "./canvas-roots.mjs";
import { serviceSnapshot, serviceProjection } from "./service-document.mjs";
const uuid = /^[0-9a-f]{8}-(?:[0-9a-f]{4}-){3}[0-9a-f]{12}$/;
const decimal = /^(0|[1-9][0-9]{0,18})$/;
const failure = (code) =>
  Object.assign(new Error(code), { code, definitive: true });
// The transport carries the binary as base64 together with its public projection.
// Reserve 1 MiB for bindings, the accepted delta and control envelopes.
function requireSnapshotCapacity(state, projection) {
  if (
    state.byteLength > 8 * 1024 * 1024 ||
    Math.ceil(state.byteLength / 3) * 4 +
      Buffer.byteLength(JSON.stringify(projection)) >
      15 * 1024 * 1024
  )
    throw failure("STATE_TOO_LARGE");
}
export function decodeBinary(value, limit) {
  if (
    typeof value !== "string" ||
    value.length > Math.ceil(limit / 3) * 4 ||
    /[^A-Za-z0-9+/=]/.test(value)
  )
    throw failure("INVALID_UPDATE");
  const bytes = Buffer.from(value, "base64");
  if (bytes.length > limit || bytes.toString("base64") !== value)
    throw failure("INVALID_UPDATE");
  return bytes;
}
function binding(request) {
  const { documentId, epoch, writerTerm, commitSeq } = request;
  if (
    !uuid.test(documentId) ||
    !uuid.test(epoch) ||
    !decimal.test(writerTerm) ||
    !decimal.test(commitSeq) ||
    request.document?.id !== documentId
  )
    throw failure("INVALID_BINDING");
  return { documentId, epoch, writerTerm, commitSeq };
}
// Garbage-collected subtrees identify deleted container lifetimes in the public
// update encoding. Deleted characters within a live XmlText remain Items and
// continue to support concurrent insertion/deletion; never inspect doc.store.
function rejectDeletedTargets(state, update) {
  const baseline = Y.decodeUpdate(state);
  let incoming;
  try {
    incoming = Y.decodeUpdate(update);
  } catch {
    throw failure("INVALID_UPDATE");
  }
  const rangeCount = [...incoming.ds.clients.values()].reduce(
    (sum, ranges) => sum + ranges.length,
    0,
  );
  if (incoming.structs.length + rangeCount > 20000)
    throw failure("UPDATE_TOO_COMPLEX");
  const garbage = new Map(),
    removedParents = new Set();
  for (const item of baseline.structs) {
    if (item instanceof Y.GC) {
      const ranges = garbage.get(item.id.client) ?? [];
      ranges.push([item.id.clock, item.id.clock + item.length]);
      garbage.set(item.id.client, ranges);
    } else if (
      item instanceof Y.Item &&
      item.content instanceof Y.ContentDeleted
    )
      removedParents.add(`${item.id.client}:${item.id.clock}`);
  }
  const overlaps = (client, start, end) => {
    const ranges = garbage.get(client) ?? [];
    let low = 0,
      high = ranges.length;
    while (low < high) {
      const middle = (low + high) >>> 1;
      if (ranges[middle][1] <= start) low = middle + 1;
      else high = middle;
    }
    return low < ranges.length && ranges[low][0] < end;
  };
  const deleted = (id) =>
    id && typeof id === "object" && overlaps(id.client, id.clock, id.clock + 1);
  const known = Y.decodeStateVector(Y.encodeStateVectorFromUpdate(state));
  for (const item of incoming.structs)
    if (
      item.id.clock + item.length > (known.get(item.id.client) ?? 0) &&
      (deleted(item.parent) ||
        (item.parent &&
          typeof item.parent === "object" &&
          removedParents.has(`${item.parent.client}:${item.parent.clock}`)) ||
        deleted(item.origin) ||
        deleted(item.rightOrigin))
    )
      throw failure("TARGET_DELETED");
  // Delete sets in state-vector deltas are cumulative. Already committed
  // tombstones are idempotent, not evidence of a new stale edit. Missing clocks
  // are separately rejected by AdmissionGate.requireIntegrated.
}

/** No database, mutable room, or pre-acceptance broadcast is owned by this process. */
export async function computeCandidate(action, request) {
  const bound = binding(request);
  if (action === "initialize") {
    const doc = createCanvasDocument(serviceSnapshot(request.document));
    try {
      const snapshot = readCanvasDocument(doc),
        state = Y.encodeStateAsUpdate(doc);
      if (state.byteLength > 8 * 1024 * 1024) throw failure("STATE_TOO_LARGE");
      const projection = serviceProjection(snapshot, request.document);
      requireSnapshotCapacity(state, projection);
      if (canonicalJson(projection) !== canonicalJson(request.document))
        throw failure("MIGRATION_NOT_LOSSLESS");
      return {
        ...bound,
        state: Buffer.from(state).toString("base64"),
        stateHash: digest(state),
        graphHash: digest(canonicalJson(snapshot)),
        projection,
      };
    } finally {
      doc.destroy();
    }
  }
  if (action !== "prepare") throw failure("UNKNOWN_ACTION");
  const state = decodeBinary(request.state, 8 * 1024 * 1024),
    update = decodeBinary(request.update, 768 * 1024);
  if (!uuid.test(request.updateId) || digest(update) !== request.payloadHash)
    throw failure("IDENTITY_MISMATCH");
  rejectDeletedTargets(state, update);
  let result,
    projectionBase = request.document;
  const gate = new AdmissionGate({
    allowedRoots: ["canvas"],
    validateRoots: validateCanvasRoots,
    inspectDocument: inspectCanvasDocument,
    validateTransition: (before, doc) => {
      const transition = validateCanvasTransition(before, doc);
      if (transition)
        projectionBase = {
          ...request.document,
          nodes: [
            ...request.document.nodes,
            ...(request.restorationNodes ?? []).filter(
              (n) =>
                transition.nodes.includes(n.id) &&
                !request.document.nodes.some((live) => live.id === n.id),
            ),
          ],
        };
      normalizeReferenceLabels(doc, projectionBase, transition);
      normalizeNodeReferenceLabels(doc, projectionBase, transition);
      return transition;
    },
    project: readCanvasDocument,
    authorize: async () => true,
    validate: async (snapshot) => {
      serviceProjection(snapshot, projectionBase);
    },
    store: {
      receipt: async () => null,
      load: async () => ({
        epoch: request.epoch,
        commitSeq: request.commitSeq,
        update: state,
      }),
      commit: async (_scope, candidate) => {
        requireSnapshotCapacity(
          candidate.state,
          serviceProjection(candidate.graph, projectionBase),
        );
        const appliedUpdate = Y.diffUpdate(
          candidate.state,
          Y.encodeStateVectorFromUpdate(state),
        );
        if (appliedUpdate.byteLength > 768 * 1024)
          throw failure("UPDATE_TOO_LARGE");
        result = {
          ...bound,
          updateId: request.updateId,
          payloadHash: request.payloadHash,
          state: Buffer.from(candidate.state).toString("base64"),
          stateHash: digest(candidate.state),
          graphHash: digest(canonicalJson(candidate.graph)),
          effect: candidate.effect,
          transition: candidate.transition,
          appliedUpdate: Buffer.from(appliedUpdate).toString("base64"),
          projection: serviceProjection(candidate.graph, projectionBase),
        };
        return { committed: true, receipt: result };
      },
    },
  });
  await gate.submit(
    { epoch: request.epoch },
    { epoch: request.epoch, updateId: request.updateId, update },
  );
  return result;
}

// Authority is the committed DTO under Go's CAS fence, never a client's counter.
function normalizeReferenceLabels(doc, base, transition) {
  const previous = new Map(
    base.nodes.map((n) => [n.id, n.config?.composer?.labelState]),
  );
  for (const node of readCanvasDocument(doc).graph.nodes) {
    if (transition?.nodes.includes(node.id)) continue;
    if (!node.references) {
      if (!previous.has(node.id) || !previous.get(node.id)) continue;
      // Undoing the first promotion may remove the registry container marker.
      // Keep an empty registry and its accepted label tombstones in Yjs itself.
      doc
        .getMap("canvas")
        .get("nodes")
        .get(node.id)
        .get("data")
        .set("referencesPresent", true);
      node.references = [];
    }
    const labels = structuredClone(
      previous.get(node.id) ?? {
        highWater: { image: 0, video: 0, audio: 0, subject: 0 },
        allocations: {},
      },
    );
    for (const reference of node.references) {
      const kind =
        reference.source?.kind === "subject" ? "subject" : reference.mediaKind;
      if (!["image", "video", "audio", "subject"].includes(kind))
        throw failure("INVALID_REFERENCE");
      const old = labels.allocations[reference.id];
      if (old && old.labelKind !== kind)
        throw failure("REFERENCE_IDENTITY_CHANGED");
      if (!old) {
        if (labels.highWater[kind] >= Number.MAX_SAFE_INTEGER)
          throw failure("REFERENCE_LABEL_LIMIT");
        Object.defineProperty(labels.allocations, reference.id, {
          value: { labelKind: kind, ordinal: ++labels.highWater[kind] },
          enumerable: true,
          writable: true,
          configurable: true,
        });
      }
    }
    if (canonicalJson(labels) !== canonicalJson(node.labelState ?? {}))
      doc
        .getMap("canvas")
        .get("nodes")
        .get(node.id)
        .get("data")
        .set("labelState", labels);
  }
}

// Node mentions use stable source-node identities, independently of media binding
// registries. Both public editor paths receive server-assigned display numbers.
function normalizeNodeReferenceLabels(doc, base, transition) {
  const graph = readCanvasDocument(doc).graph,
    byId = new Map(graph.nodes.map((n) => [n.id, n]));
  for (const target of graph.nodes) {
    // The trusted inverse owns every restored field, including legacy nodes
    // whose pre-deletion version did not yet allocate display labels.
    if (transition?.nodes.includes(target.id)) continue;
    const previous = base.nodes.find((n) => n.id === target.id)?.config?.editor
      ?.metadata?.referenceLabels;
    const labels = structuredClone(previous ?? {}),
      high = { image: 0, video: 0, audio: 0, text: 0 };
    for (const value of Object.values(labels))
      high[value.kind] = Math.max(high[value.kind] ?? 0, value.ordinal);
    const sourceIDs = graph.connections
      .filter((e) => e.toNodeId === target.id)
      .flatMap((e) =>
        byId.get(e.fromNodeId)?.type === "group"
          ? graph.nodes
              .filter((n) => n.parentId === e.fromNodeId)
              .map((n) => n.id)
          : [e.fromNodeId],
      )
      .sort();
    for (const id of sourceIDs) {
      const kind = byId.get(id)?.type;
      if (!Object.hasOwn(high, kind)) continue;
      if (!Object.hasOwn(labels, id)) {
        if (high[kind] >= Number.MAX_SAFE_INTEGER)
          throw failure("REFERENCE_LABEL_LIMIT");
        labels[id] = { kind, ordinal: ++high[kind] };
      }
    }
    if (Object.keys(labels).length > 10000)
      throw failure("REFERENCE_LABEL_LIMIT");
    const shared = doc.getMap("canvas").get("nodes").get(target.id);
    const walk = (element) => {
      if (element instanceof Y.XmlElement && element.nodeName === "mention") {
        const allocation = labels[element.getAttribute("id")];
        if (allocation) {
          const label =
            {
              image: "图片",
              video: "参考视频",
              audio: "参考音频",
              text: "文本",
            }[allocation.kind] + allocation.ordinal;
          if (element.getAttribute("label") !== label)
            element.setAttribute("label", label);
          if (element.getAttribute("token") !== label)
            element.setAttribute("token", label);
        }
      }
      if (element instanceof Y.XmlElement || element instanceof Y.XmlFragment)
        for (const child of element.toArray()) walk(child);
    };
    if (shared.get("body")) walk(shared.get("body"));
    if (
      (Object.keys(labels).length || target.referenceLabels) &&
      canonicalJson(labels) !== canonicalJson(target.referenceLabels ?? {})
    )
      doc
        .getMap("canvas")
        .get("nodes")
        .get(target.id)
        .get("data")
        .set("referenceLabels", labels);
  }
}
