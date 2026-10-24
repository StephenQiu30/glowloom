import * as Y from "yjs";
import {
  applyCanvasChanges,
  applyRecoveredCanvasChanges,
  readCanvasDocument,
} from "./model.mjs";
import {
  archiveRecovery,
  planRecovery,
  settleRecovery,
  recoveryFields,
} from "./recovery.mjs";
import {
  trackIntents,
  acknowledgeIntents,
  applyReadableIntents,
} from "./intents.mjs";
import {
  base64 as binary,
  unbase64 as decode,
  sha256 as hash,
} from "./binary.mjs";

const REMOTE = Symbol("committed");
const problem = (code) => Object.assign(new Error(code), { code });

/** One optimistic document and one committed document; no unaccepted cross-tab channel. */
export class CollaborationSession {
  constructor({
    scope,
    initial,
    journal,
    storage = "journal",
    transport,
    changed = () => {},
    recovery,
  }) {
    this.scope = Object.freeze({ ...scope, epoch: initial.epoch });
    if (storage !== "memory" && !journal) throw problem("JOURNAL_REQUIRED");
    this.storage = storage;
    this.journal = journal;
    this.transport = transport;
    this.changed = changed;
    this.confirmed = { ...initial };
    this.pending = [];
    this.inFlight = null;
    this.acceptedReceipt = structuredClone(recovery?.acceptedReceipt ?? null);
    this.recoverySources = structuredClone(recovery?.recoverySources ?? []);
    this.recoverySelection = structuredClone(
      recovery?.recoverySelection ?? null,
    );
    this.intents = structuredClone(recovery?.intents ?? []);
    this.inFlightIntents = structuredClone(recovery?.inFlightIntents ?? []);
    this.automaticReplayAttempts = recovery?.automaticReplayAttempts ?? 0;
    this.sealed = null;
    this.forms = recovery?.forms ?? {};
    this.textDrafts = recovery?.textDrafts ?? {};
    this.control = recovery?.control ?? null;
    this.status = "CLEAN";
    this.protected = false;
    this.closed = false;
    this.generation = recovery?.generation ?? 0;
    this.persistence = Promise.resolve();
    this.committed = new Y.Doc();
    Y.applyUpdate(this.committed, decode(initial.update));
    this.doc = this.freshDocument();
    if (recovery) {
      if (
        recovery.version !== 2 ||
        ["origin", "actorKey", "canvasId", "sessionId", "epoch"].some(
          (key) => recovery.scope[key] !== this.scope[key],
        ) ||
        recovery.confirmed.epoch !== initial.epoch
      )
        throw problem("RECOVERY_SCOPE_CHANGED");
      this.inFlight = recovery.inFlight;
      this.pending = recovery.pending.map(decode);
      this.sealed = recovery.sealed;
      if (this.recoverySelection && !this.inFlight && !this.pending.length)
        this.recoverySelection = null;
      if (this.sealed?.originalRejected) this.inFlight = null;
      if (this.inFlight && !this.sealed)
        Y.applyUpdate(this.doc, decode(this.inFlight.update));
      if (!this.sealed)
        for (const update of this.pending) Y.applyUpdate(this.doc, update);
      this.status = this.sealed
        ? this.inFlight
          ? "UNKNOWN"
          : "REJECTED"
        : this.inFlight
          ? "UNKNOWN"
          : this.pending.length || Object.keys(this.textDrafts).length
            ? "DIRTY"
            : "CLEAN";
    }
    this.watch();
  }

  freshDocument() {
    const doc = new Y.Doc();
    Y.applyUpdate(doc, Y.encodeStateAsUpdate(this.committed));
    return doc;
  }

  watch() {
    this.observedSnapshot = this.snapshot;
    this.doc.on("update", (update, origin) => {
      if (this.closed) return;
      const before = this.observedSnapshot,
        after = this.snapshot;
      this.observedSnapshot = after;
      if (origin === REMOTE) return;
      this.pending.push(new Uint8Array(update));
      this.generation++;
      this.intents = trackIntents(this.intents, before, after, this.generation);
      this.protected = false;
      if (this.status === "CLEAN") this.status = "DIRTY";
      this.changed(this);
    });
  }

  get snapshot() {
    return readCanvasDocument(this.doc);
  }
  get committedSnapshot() {
    return readCanvasDocument(this.committed);
  }

  change(after, before = this.snapshot) {
    if (this.closed || this.sealed || this.status === "REJECTED")
      throw problem("RECOVERY_REQUIRED");
    applyCanvasChanges(this.doc, before, after);
  }

  record() {
    const preparing = this.inFlight && !this.inFlight.payloadHash;
    return structuredClone({
      version: 2,
      generation: this.generation,
      scope: this.scope,
      confirmed: {
        ...this.confirmed,
      },
      inFlight: preparing ? null : this.inFlight,
      acceptedReceipt: this.acceptedReceipt,
      recoverySources: this.recoverySources,
      recoverySelection:
        preparing && this.recoverySelection
          ? { ...this.recoverySelection, updateId: null }
          : this.recoverySelection,
      intents: preparing
        ? trackIntents(
            [],
            this.committedSnapshot,
            applyReadableIntents(this.snapshot, [
              ...this.inFlightIntents,
              ...this.intents,
            ]),
            this.generation,
          )
        : this.intents,
      inFlightIntents: preparing ? [] : this.inFlightIntents,
      automaticReplayAttempts: this.automaticReplayAttempts,
      pending: [
        ...(preparing ? [this.inFlight.update] : []),
        ...this.pending.map(binary),
      ],
      snapshot: this.snapshot,
      sealed: this.sealed,
      forms: this.forms,
      textDrafts: this.textDrafts,
      control: this.control,
      updatedAt: Date.now(),
    });
  }

  setForms(forms) {
    this.forms = structuredClone(forms);
    this.generation++;
    this.protected = false;
    this.changed(this);
  }

  setTextDraft(nodeId, document, source) {
    if (this.closed) return;
    const snapshot = this.snapshot;
    const exists = snapshot.graph.nodes.some((node) => node.id === nodeId);
    if (!document && !exists && !Object.hasOwn(this.textDrafts, nodeId)) return;
    if (
      JSON.stringify(this.textDrafts[nodeId] ?? null) ===
      JSON.stringify(document)
    )
      return;
    if (document) this.textDrafts[nodeId] = structuredClone(document);
    else delete this.textDrafts[nodeId];
    if (!exists && document) {
      this.sealed ??= {
        reason: "REMOTE_DELETED",
        snapshot,
        inFlight: this.inFlight,
        pending: this.pending.map(binary),
        nodeIds: [nodeId],
      };
      const readable = this.sealed.snapshot.graph.nodes;
      const old = readable.findIndex((node) => node.id === nodeId);
      const node = old >= 0 ? readable[old] : source;
      if (node) {
        const retained = {
          ...structuredClone(node),
          textDocument: structuredClone(document),
        };
        if (old >= 0) readable[old] = retained;
        else readable.push(retained);
      }
      if (!this.inFlight) this.status = "REJECTED";
    } else if (this.status === "CLEAN" && document) this.status = "DIRTY";
    else if (
      this.status === "DIRTY" &&
      !this.pending.length &&
      !this.inFlight &&
      !Object.keys(this.textDrafts).length
    )
      this.status = "CLEAN";
    this.generation++;
    this.protected = false;
    this.changed(this);
  }

  async persist() {
    const generation = this.generation,
      record = this.record();
    const write = this.persistence
      .catch(() => {})
      .then(() =>
        this.storage === "memory" ? undefined : this.journal.put(record),
      );
    this.persistence = write;
    await write;
    if (!this.closed && generation === this.generation) this.protected = true;
    return record;
  }

  setControl(control) {
    this.control = structuredClone(control);
    this.generation++;
    this.protected = false;
    this.changed(this);
  }

  async applyRecovery(sourceId, selected) {
    if (
      this.closed ||
      this.sealed ||
      this.status !== "CLEAN" ||
      this.control ||
      this.recoverySelection ||
      Object.keys(this.forms).length ||
      Object.keys(this.textDrafts).length
    ) {
      throw problem("RECOVERY_REQUIRED");
    }
    const source = this.recoverySources.find((value) => value.id === sourceId);
    if (!source) throw problem("RECOVERY_SOURCE_MISSING");
    const before = this.snapshot,
      allocations = {};
    // A rejected creation never allocated its ID on the server. Rebuild a new
    // causal update, retaining identity only inside that same epoch.
    if (
      source.scope.epoch === this.confirmed.epoch &&
      source.sealed.originalRejected === true
    ) {
      for (const intent of source.sealed.intents ?? []) {
        if (
          intent.field !== "entity" ||
          intent.before.exists ||
          !intent.after.exists ||
          !["nodes", "connections"].includes(intent.collection) ||
          before.graph[intent.collection].some(
            (entity) => entity.id === intent.id,
          )
        )
          continue;
        allocations[
          intent.collection === "connections"
            ? `connection:${intent.id}`
            : intent.id
        ] = intent.id;
      }
    }
    const plan = planRecovery(source, before, selected, allocations);
    this.recoverySelection = { ...plan.selection, forms: plan.forms };
    this.generation++;
    try {
      await this.persist();
    } catch (error) {
      this.recoverySelection = null;
      throw error;
    }
    if (this.closed) throw problem("RECOVERY_REQUIRED");
    // Recheck the reviewed fields after asynchronous device protection, preserving unrelated accepted changes.
    try {
      const latest = planRecovery(
        source,
        this.snapshot,
        selected,
        plan.selection.locations,
      );
      applyRecoveredCanvasChanges(this.doc, this.snapshot, latest.snapshot);
    } catch (error) {
      this.recoverySelection = null;
      await this.protect();
      throw error;
    }
    if (!this.pending.length) {
      this.forms = { ...this.forms, ...plan.forms };
      this.recoverySources = settleRecovery(
        this.recoverySources,
        this.recoverySelection,
        [],
        true,
      );
      this.recoverySelection = null;
      this.generation++;
    }
    if (!(await this.protect())) throw this.journalError;
    this.changed(this);
  }

  retainSourceText(scope, nodeId, document, sourceNode, sourceId) {
    if (this.closed || !document) return false;
    const source = this.recoverySources.find(
      (value) =>
        (!sourceId || value.id === sourceId) &&
        ["origin", "actorKey", "canvasId", "epoch", "sessionId"].every(
          (key) => value.scope[key] === scope[key],
        ),
    );
    if (!source) return false;
    if (JSON.stringify(source.textDrafts[nodeId]) === JSON.stringify(document))
      return true;
    source.textDrafts[nodeId] = structuredClone(document);
    source.textGenerations ??= {};
    source.textGenerations[nodeId] = ++this.generation;
    if (
      !source.sealed.snapshot.graph.nodes.some((node) => node.id === nodeId) &&
      sourceNode
    ) {
      source.sealed.snapshot.graph.nodes.push(structuredClone(sourceNode));
    }
    this.protected = false;
    this.changed(this);
    return true;
  }

  retainDetachedText(record, nodeId, document, sourceNode) {
    if (
      this.closed ||
      !document ||
      record.inFlight ||
      record.control ||
      !["origin", "actorKey", "canvasId"].every(
        (key) => record.scope[key] === this.scope[key],
      )
    )
      return;
    const existing = this.recoverySources.find((source) =>
      ["origin", "actorKey", "canvasId", "epoch", "sessionId"].every(
        (key) => source.scope[key] === record.scope[key],
      ),
    );
    if (existing) {
      this.retainSourceText(
        record.scope,
        nodeId,
        document,
        sourceNode,
        existing.id,
      );
      return existing.id;
    }
    if (this.recoverySources.length >= 16) return;
    const detached = structuredClone(record);
    detached.recoverySources = [];
    detached.recoverySelection = null;
    detached.textDrafts[nodeId] = structuredClone(document);
    if (
      sourceNode &&
      !detached.snapshot.graph.nodes.some((node) => node.id === nodeId)
    )
      detached.snapshot.graph.nodes.push(structuredClone(sourceNode));
    const source = archiveRecovery(detached, this.confirmed).recoverySources[0];
    if (!source) return;
    source.textGenerations[nodeId] = ++this.generation;
    this.recoverySources.push(source);
    this.protected = false;
    this.changed(this);
    return source.id;
  }

  async discardRecoverySource(sourceId) {
    if (this.closed || this.recoverySelection?.sourceId === sourceId)
      throw problem("ORIGINAL_UNCONFIRMED");
    const previous = this.recoverySources;
    this.recoverySources = previous.filter((value) => value.id !== sourceId);
    this.generation++;
    try {
      await this.persist();
    } catch (error) {
      this.recoverySources = previous;
      throw error;
    }
    this.changed(this);
  }

  releaseResolvedSources(retainedIds = []) {
    const retained = new Set(retainedIds);
    const next = this.recoverySources.filter(
      (source) =>
        retained.has(source.id) ||
        this.recoverySelection?.sourceId === source.id ||
        recoveryFields(source, source.sealed.snapshot).length,
    );
    if (next.length === this.recoverySources.length) return false;
    this.recoverySources = next;
    this.generation++;
    this.protected = false;
    return true;
  }

  async flush(options = {}) {
    if (this.flushing) return this.flushing;
    const operation = this.send(options);
    this.flushing = operation;
    try {
      return await operation;
    } finally {
      this.flushing = null;
    }
  }

  async protect() {
    try {
      await this.persist();
      this.journalError = null;
      return true;
    } catch (error) {
      this.protected = false;
      this.journalError = error;
      return false;
    }
  }

  async send({ automatic = false, queryOnly = false } = {}) {
    if (this.closed || this.status === "REJECTED")
      throw problem("RECOVERY_REQUIRED");
    if (Object.keys(this.textDrafts).length && !this.inFlight)
      throw problem("UNAPPLIED_TEXT");
    const unknown = this.status === "UNKNOWN";
    if (!this.inFlight) {
      if (!this.pending.length) return null;
      const pending = this.pending.splice(0);
      this.acceptedReceipt = null;
      this.inFlightIntents = structuredClone(this.intents);
      this.intents = [];
      this.automaticReplayAttempts = 0;
      const update = Y.mergeUpdates(pending);
      // Install the frozen batch before awaiting hashing; subsequent edits stay pending.
      this.inFlight = {
        epoch: this.confirmed.epoch,
        sessionId: this.scope.sessionId,
        updateId: crypto.randomUUID(),
        update: binary(update),
        payloadHash: "",
        ...(this.recoverySelection
          ? { reviewSeq: this.confirmed.commitSeq }
          : {}),
      };
      if (this.recoverySelection && !this.recoverySelection.updateId) {
        this.recoverySelection.updateId = this.inFlight.updateId;
      }
      this.inFlight.payloadHash = await hash(update);
    }
    const request = this.inFlight;
    if (!(await this.protect()) && this.recoverySelection)
      throw problem("RECOVERY_UNPROTECTED");
    if (this.closed) return null;
    this.status = "SAVING";
    this.changed(this);
    let acknowledged = false;
    let phase = unknown ? "QUERY" : "SUBMIT";
    try {
      let receipt = this.acceptedReceipt;
      if (unknown && !receipt) {
        for (const delay of automatic ? [0, 1000, 2000] : [0]) {
          if (delay) await new Promise((resolve) => setTimeout(resolve, delay));
          if (this.closed) return null;
          receipt = await this.transport.query(request);
          if (receipt) break;
        }
      }
      if (!receipt) {
        if (this.sealed || queryOnly) throw problem("ORIGINAL_UNCONFIRMED");
        if (unknown && automatic) {
          if (this.automaticReplayAttempts >= 2) throw problem("REPLAY_BUDGET");
          this.automaticReplayAttempts++;
          this.generation++;
          this.protected = false;
          if (!(await this.protect())) throw problem("RECOVERY_UNPROTECTED");
          if (this.closed) return null;
        }
        phase = "SUBMIT";
        receipt = await this.transport.submit(structuredClone(request));
      }
      if (this.closed) return null;
      if (
        receipt.updateId !== request.updateId ||
        receipt.epoch !== request.epoch ||
        receipt.payloadHash !== request.payloadHash
      )
        throw problem("INVALID_RECEIPT");
      await this.receive(
        {
          ...receipt,
          update: receipt.update ?? request.update,
        },
        false,
      );
      acknowledged = true;
      this.acceptedReceipt = structuredClone(receipt);
      const restoration = this.pendingRestoration();
      if (
        restoration &&
        restoration.epoch === receipt.epoch &&
        BigInt(restoration.sourceCommitSeq) + 1n === BigInt(receipt.commitSeq)
      ) {
        // Only this exact original receipt can prove a single accepted causal predecessor.
        this.doc.transact(
          () =>
            this.doc.getMap("canvas").set("restoration", {
              ...restoration,
              sourceCommitSeq: receipt.commitSeq,
            }),
          "local-restoration",
        );
      }
      let projection = this.committedSnapshot,
        verified = true;
      if (
        this.inFlightIntents.length &&
        BigInt(this.confirmed.commitSeq) > BigInt(receipt.commitSeq)
      ) {
        try {
          projection = await this.transport.confirmedSnapshot?.(receipt);
          if (!projection) verified = false;
        } catch {
          verified = false;
        }
        if (this.closed) return receipt;
      }
      if (this.inFlight === request) this.inFlight = null;
      this.acceptedReceipt = null;
      const meaning = verified
        ? acknowledgeIntents(this.intents, this.inFlightIntents, projection)
        : {
            pending: structuredClone(this.intents),
            unapplied: structuredClone(this.inFlightIntents),
          };
      this.intents = meaning.pending;
      this.inFlightIntents = [];
      if (this.recoverySelection?.updateId === request.updateId) {
        const selection = this.recoverySelection;
        this.recoverySources = settleRecovery(
          this.recoverySources,
          selection,
          meaning.unapplied,
          verified,
        );
        if (verified) {
          const unresolved = new Set(
            meaning.unapplied.map((value) =>
              JSON.stringify([value.collection, value.id, value.field]),
            ),
          );
          const appliedForms = Object.fromEntries(
            Object.entries(selection.forms ?? {}).filter(
              ([id]) =>
                !unresolved.has(JSON.stringify(["nodes", id, "entity"])),
            ),
          );
          this.forms = { ...this.forms, ...appliedForms };
        }
        const selectedKeys = new Set(
          selection.mappings.map((value) => value.targetKey),
        );
        // A losing explicit recovery stays in its source; it is never rebased and submitted again automatically.
        meaning.unapplied = meaning.unapplied.filter(
          (value) =>
            !selectedKeys.has(
              JSON.stringify([value.collection, value.id, value.field]),
            ),
        );
        this.recoverySelection = null;
      }
      if (meaning.unapplied.length) {
        const retained = [...meaning.unapplied, ...meaning.pending];
        this.sealed ??= {
          reason: verified ? "UNAPPLIED_FIELDS" : "INTENT_PROOF_UNAVAILABLE",
          snapshot: applyReadableIntents(this.snapshot, retained),
          inFlight: null,
          pending: this.pending.map(binary),
        };
        this.sealed.intents = retained;
      }
      if (this.sealed) {
        this.sealed.originalOutcome = "COMMITTED";
        this.sealed.originalReceipt = structuredClone(receipt);
      }
      this.status = this.sealed
        ? "REJECTED"
        : this.pending.length || Object.keys(this.textDrafts).length
          ? "DIRTY"
          : "CLEAN";
      this.generation++;
      this.protected = false;
      await this.protect();
      if (this.sealed && this.protected) {
        this.doc.destroy();
        this.doc = this.freshDocument();
        this.pending = [];
        this.intents = [];
        this.watch();
        await this.protect();
      }
      this.changed(this);
      return receipt;
    } catch (error) {
      if (this.closed) throw error;
      if (acknowledged) {
        this.changed(this);
        throw error;
      }
      if (
        error.definitive &&
        (phase === "SUBMIT" || error.originalOutcome === "REJECTED")
      ) {
        // Persist the entire dependent chain before discarding its CRDT client identity.
        this.sealed ??= {
          reason: error.code ?? "REJECTED_UPDATE",
          snapshot: applyReadableIntents(this.snapshot, [
            ...this.inFlightIntents,
            ...this.intents,
          ]),
          intents: structuredClone([...this.inFlightIntents, ...this.intents]),
          inFlight: this.inFlight,
          pending: this.pending.map(binary),
        };
        this.sealed.failureReason = error.code ?? "REJECTED_UPDATE";
        this.sealed.originalRejected =
          !unknown || error.originalOutcome === "REJECTED";
        this.status = this.sealed.originalRejected ? "REJECTED" : "UNKNOWN";
        if (await this.protect()) {
          this.doc.destroy();
          this.doc = this.freshDocument();
          this.pending = [];
          this.intents = [];
          this.inFlightIntents = [];
          if (this.sealed.originalRejected) this.inFlight = null;
          this.watch();
        }
      } else this.status = "UNKNOWN";
      this.changed(this);
      throw error;
    }
  }

  async receive(message, persist = true) {
    if (this.closed) return;
    if (message.epoch !== this.confirmed.epoch) throw problem("EPOCH_CHANGED");
    const incoming = BigInt(message.commitSeq),
      current = BigInt(this.confirmed.commitSeq);
    if (incoming > current + 1n && !message.snapshot)
      throw problem("RESYNC_REQUIRED");
    const update = decode(message.update);
    const local = this.snapshot;
    const previous = readCanvasDocument(this.committed);
    Y.applyUpdate(this.committed, update, REMOTE);
    const committedUpdate = binary(Y.encodeStateAsUpdate(this.committed));
    if (incoming >= current) {
      this.confirmed = {
        ...this.confirmed,
        commitSeq: message.commitSeq,
        contentRevision: message.contentRevision,
        graphHash: message.graphHash,
        update: committedUpdate,
      };
    } else this.confirmed = { ...this.confirmed, update: committedUpdate };
    this.generation++;
    const alive = new Set(
      readCanvasDocument(this.committed).graph.nodes.map((node) => node.id),
    );
    const previousNodes = new Map(
      previous.graph.nodes.map((node) => [node.id, node]),
    );
    const restoration = this.pendingRestoration();
    const privateIntents = [...this.inFlightIntents, ...this.intents];
    const privateNodes = new Set(
      privateIntents
        .filter(
          (intent) =>
            intent.collection === "nodes" &&
            (intent.field !== "entity" || intent.after.exists),
        )
        .map((intent) => intent.id),
    );
    const lost = local.graph.nodes.filter(
      (node) =>
        previousNodes.has(node.id) &&
        !alive.has(node.id) &&
        !restoration?.nodes.includes(node.id) &&
        (this.textDrafts[node.id] ||
          privateNodes.has(node.id) ||
          JSON.stringify(previousNodes.get(node.id)) !== JSON.stringify(node)),
    );
    if (lost.length) {
      const readable = applyReadableIntents(local, privateIntents);
      this.sealed ??= {
        reason: "REMOTE_DELETED",
        snapshot: {
          ...readable,
          graph: {
            ...readable.graph,
            nodes: readable.graph.nodes.map((node) =>
              this.textDrafts[node.id]
                ? {
                    ...node,
                    textDocument: structuredClone(this.textDrafts[node.id]),
                  }
                : node,
            ),
          },
        },
        intents: structuredClone(privateIntents),
        inFlight: this.inFlight,
        pending: this.pending.map(binary),
        nodeIds: lost.map((node) => node.id),
      };
      if (!this.inFlight) this.status = "REJECTED";
      // Keep the readable preimage before Yjs is allowed to GC a deleted shared subtree.
      await this.protect();
    }
    Y.applyUpdate(this.doc, update, REMOTE);
    this.generation++;
    if (persist) await this.protect();
    this.changed(this);
  }

  destroy() {
    this.closed = true;
    this.doc.destroy();
    this.committed.destroy();
  }
  pendingRestoration() {
    const current = this.doc.getMap("canvas").get("restoration"),
      committed = this.committed.getMap("canvas").get("restoration");
    return current && current.id !== committed?.id ? current : null;
  }
}
