import * as Y from "yjs";
import { digest } from "./admission.js";
import { canonicalJson } from "../document/canonical.js";
import { serviceSnapshot, serviceProjection } from "../document/projection.js";
import {
  readCanvasDocument,
  applyCommandCanvasChanges,
} from "../document/model.js";
const uuid = /^[0-9a-f]{8}-(?:[0-9a-f]{4}-){3}[0-9a-f]{12}$/;
const fail = (code) =>
  Object.assign(new Error(code), { code, definitive: true });
// Go persists these identity-addressed collections ordered by UUID; the shared
// model retains insertion order. Order is not a business field (z_index is).
function comparableProjection(value) {
  return {
    ...value,
    nodes: [...value.nodes].sort((a, b) => a.id.localeCompare(b.id)),
    edges: [...value.edges].sort((a, b) => a.id.localeCompare(b.id)),
  };
}
// Go supplies a domain-validated target at an exact durable prefix. This module
// applies native shared operations; it never constructs or patches opaque bytes.
export function prepareCommands(request, state) {
  if (
    !uuid.test(request.updateId) ||
    !request.projection ||
    [
      "id",
      "project_id",
      "name",
      "schema_version",
      "operation_version",
      "revision",
      "kind",
      "scope",
      "permissions",
      "read_only_reason",
    ].some(
      (key) =>
        canonicalJson(request.projection[key] ?? null) !==
        canonicalJson(request.document[key] ?? null),
    )
  )
    throw fail("INVALID_COMMAND_PROJECTION");
  const doc = new Y.Doc();
  try {
    Y.applyUpdate(doc, state);
    const before = readCanvasDocument(doc);
    if (
      canonicalJson(
        comparableProjection(serviceProjection(before, request.document)),
      ) !== canonicalJson(comparableProjection(request.document))
    )
      throw fail("PROJECTION_DRIFT");
    const vector = Y.encodeStateVector(doc),
      clients = Y.decodeStateVector(vector),
      limit = 2n ** 53n - 1n;
    let identity =
      (BigInt(
        "0x" +
          digest(`canvas-command:${request.epoch}:${request.updateId}`).slice(
            0,
            14,
          ),
      ) %
        limit) +
      1n;
    for (let attempt = 0; clients.has(Number(identity)); attempt++) {
      if (attempt >= 64) throw fail("CLIENT_IDENTITY_BUDGET");
      identity = (identity % limit) + 1n;
    }
    doc.clientID = Number(identity);
    const target = serviceSnapshot(request.projection);
    // A sorted SQL read does not request changes to committed shared order.
    for (const collection of ["nodes", "connections"]) {
      const order = new Map<string, number>(
        before.graph[collection].map((item, index) => [item.id, index]),
      );
      target.graph[collection].sort(
        (a, b) =>
          (order.get(a.id) ?? Number.MAX_SAFE_INTEGER) -
          (order.get(b.id) ?? Number.MAX_SAFE_INTEGER),
      );
    }
    applyCommandCanvasChanges(doc, before, target);
    const update = Y.encodeStateAsUpdate(doc, vector);
    if (update.byteLength > 768 * 1024) throw fail("UPDATE_TOO_LARGE");
    return {
      update: Buffer.from(update).toString("base64"),
      payloadHash: digest(update),
    };
  } finally {
    doc.destroy();
  }
}
