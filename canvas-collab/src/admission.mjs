import { createHash } from "node:crypto";
import * as Y from "yjs";
import { canonicalJson } from "./canonical.mjs";

const failure = (code, message = code) =>
  Object.assign(new Error(message), {
    code,
    definitive: !["CONTENTION_LIMIT", "STATE_TOO_LARGE"].includes(code),
  });
export const digest = (bytes) =>
  createHash("sha256").update(bytes).digest("hex");

/** Public update decoding plus integrated clocks; no private Yjs store fields. */
function requireIntegrated(doc, decoded) {
  const clocks = Y.decodeStateVector(Y.encodeStateVector(doc));
  for (const item of decoded.structs) {
    if ((clocks.get(item.id.client) ?? 0) < item.id.clock + item.length)
      throw failure("MISSING_DEPENDENCY");
  }
  // A delete-only update can have no structs and an unchanged state vector.
  for (const [client, ranges] of decoded.ds.clients)
    for (const range of ranges)
      if ((clocks.get(client) ?? 0) < range.clock + range.len)
        throw failure("MISSING_DEPENDENCY");
}

/** Admission primitive only. The durable adapter owns authorization, CAS and Outbox. */
export class AdmissionGate {
  constructor({
    store,
    project,
    validate,
    authorize,
    broadcast = async () => {},
    onPublishError = () => {},
    maxUpdateBytes = 1024 * 1024,
    maxStructs = 20000,
    maxStateBytes = 8 * 1024 * 1024,
    maxRetries = 4,
    allowedRoots = ["nodes", "edges", "text"],
    inspectDocument,
    validateTransition,
    validateRoots,
  }) {
    Object.assign(this, {
      store,
      project,
      validate,
      authorize,
      broadcast,
      onPublishError,
      maxUpdateBytes,
      maxStructs,
      maxStateBytes,
      maxRetries,
      inspectDocument,
      validateTransition,
      validateRoots,
    });
    this.allowedRoots = new Set(allowedRoots);
  }

  async submit(scope, request) {
    if (!(await this.authorize(scope))) throw failure("FORBIDDEN");
    if (request.epoch !== scope.epoch) throw failure("EPOCH_CHANGED");
    if (
      typeof request.updateId !== "string" ||
      !request.updateId.length ||
      request.updateId.length > 96
    )
      throw failure("INVALID_IDENTITY");
    if (!(request.update instanceof Uint8Array))
      throw failure("INVALID_UPDATE");
    if (request.update.byteLength > this.maxUpdateBytes)
      throw failure("UPDATE_TOO_LARGE");
    const update = new Uint8Array(request.update);
    const payloadHash = digest(update);
    const original = await this.store.receipt(scope, request.updateId);
    if (original) {
      if (original.payloadHash !== payloadHash) throw failure("IDENTITY_REUSE");
      return original;
    }
    let decoded;
    try {
      decoded = Y.decodeUpdate(update);
    } catch {
      throw failure("INVALID_UPDATE");
    }
    const ranges = [...decoded.ds.clients.values()].reduce(
      (n, values) => n + values.length,
      0,
    );
    if (decoded.structs.length + ranges > this.maxStructs)
      throw failure("UPDATE_TOO_COMPLEX");

    for (let attempt = 0; attempt < this.maxRetries; attempt++) {
      const base = await this.store.load(scope);
      if (base.epoch !== scope.epoch) throw failure("EPOCH_CHANGED");
      if (base.update.byteLength > this.maxStateBytes)
        throw failure("STATE_TOO_LARGE");
      const candidate = new Y.Doc();
      try {
        Y.applyUpdate(candidate, base.update);
        const previousState = Y.encodeStateAsUpdate(candidate);
        const inspection = this.inspectDocument?.(candidate);
        const previousGraph = canonicalJson(this.project(candidate));
        try {
          Y.applyUpdate(candidate, update);
        } catch {
          throw failure("INVALID_UPDATE");
        }
        requireIntegrated(candidate, decoded);
        if (this.validateRoots) await this.validateRoots(candidate);
        else
          for (const name of candidate.share.keys())
            if (!this.allowedRoots.has(name)) throw failure("INVALID_ROOT");
        let transition, graph;
        try {
          transition = await this.validateTransition?.(inspection, candidate);
          graph = this.project(candidate);
          await this.validate(graph, {
            scope,
            previousGraph: JSON.parse(previousGraph),
          });
        } catch (error) {
          error.definitive = true;
          throw error;
        }
        const state = Y.encodeStateAsUpdate(candidate);
        if (state.byteLength > this.maxStateBytes)
          throw Object.assign(failure("STATE_TOO_LARGE"), { definitive: true });
        const graphBytes = canonicalJson(graph);
        const effect =
          digest(state) === digest(previousState)
            ? "NO_CHANGE"
            : graphBytes === previousGraph
              ? "CAUSAL_CHANGE"
              : "VISIBLE_CHANGE";
        const result = await this.store.commit(scope, {
          updateId: request.updateId,
          payloadHash,
          update,
          state,
          graph,
          graphHash: digest(graphBytes),
          effect,
          expectedCommitSeq: base.commitSeq,
          baseMetadata: base.metadata,
          transition,
        });
        if (!result.committed) continue;
        if (!result.replayed && effect !== "NO_CHANGE") {
          try {
            await this.broadcast(scope, result.receipt);
          } catch (error) {
            // The adapter committed its Outbox atomically; publish is just a hint.
            this.onPublishError(error, result.receipt);
          }
        }
        return result.receipt;
      } finally {
        candidate.destroy();
      }
    }
    throw failure("CONTENTION_LIMIT");
  }
}
