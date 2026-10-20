import type { Canvas } from "./document";
import {
  applyEffects,
  applyOperations,
  operationError,
  transformOperationPair,
  type Effect,
  type Operation,
} from "./operations";

export type Body = {
  operation_version: 1;
  base_revision: number;
  operations: Operation[];
  undo_of?: string;
};
export type Envelope = { key: string; body: Body };
export type Receipt = {
  status: "accepted" | "rejected";
  op_id: string;
  revision?: number;
  code?: string;
};
export type Change = {
  revision: number;
  actor_id: string;
  op_id: string;
  operations: Operation[];
  effects: Effect[];
};
type Intent = { key: string; operations: Operation[] };
type Pending = Envelope & {
  display: Operation[];
  receipt?: Receipt;
  applied?: boolean;
  invalid?: string;
};
export type Recovery = Intent & { reason: string };
function freeze<T>(value: T): T {
  if (value && typeof value === "object") {
    Object.freeze(value);
    for (const item of Object.values(value)) freeze(item);
  }
  return value;
}
// Dependency is deliberately about operation targets and source premises, not
// whole-node equality. Distinct scalar fields remain independently recoverable.
function depends(a: Operation[], b: Operation[]): boolean {
  const creates = new Set(
    a.filter((o) => o.type === "create_node").map((o) => o.node?.id),
  );
  return b.some(
    (y) =>
      creates.has(y.node_id) ||
      creates.has(y.edge?.source_node_id) ||
      creates.has(y.edge?.target_node_id) ||
      a.some((x) => {
        if (x.node_id !== y.node_id) return false;
        if (
          x.type === "delete_node" ||
          x.type === "create_node" ||
          y.type === "delete_node"
        )
          return true;
        const p = x.path ?? [],
          q = y.path ?? [],
          n = Math.min(p.length, q.length);
        if (p.slice(0, n).every((v, i) => v === q[i])) return true;
        return (
          (x.item_id !== undefined && q.includes("@" + x.item_id)) ||
          (x.item_id !== undefined && x.item_id === y.before_id) ||
          p.at(-1) === "model" ||
          p.at(-1) === "source"
        );
      }),
  );
}

/** Session-owned memory only. This is the sole owner of B/P/Q and wire identity. */
export class Collaboration {
  base: Canvas;
  pending?: Pending;
  queue: Intent[] = [];
  recovery: Recovery[] = [];
  private online = false;
  private writable = false;
  private actor = "";
  private waiters: {
    resolve: (doc: Canvas) => void;
    reject: (error: Error) => void;
  }[] = [];
  constructor(
    initial: Canvas,
    private changed: () => void,
    private submit: (envelope: Envelope) => void,
  ) {
    this.base = structuredClone(initial);
  }
  get document(): Canvas {
    let doc = this.base;
    if (this.pending && !this.pending.applied && !this.pending.invalid)
      doc = applyOperations(doc, this.pending.display);
    for (const item of this.queue) doc = applyOperations(doc, item.operations);
    return doc;
  }
  get dirty() {
    return !!this.pending || this.queue.length > 0;
  }
  connection(online: boolean, writable: boolean, actor = this.actor) {
    this.online = online;
    this.writable = writable;
    this.actor = actor;
    this.pump();
    this.changed();
  }
  enqueue(operations: Operation[], key = crypto.randomUUID()) {
    if (!this.online || !this.writable)
      return operationError("connection_read_only");
    // Symbols and any prediction-only labels can never enter the wire intent.
    const clean = JSON.parse(JSON.stringify(operations)) as Operation[];
    applyOperations(this.document, clean);
    this.queue.push({ key, operations: clean });
    this.pump();
    this.changed();
  }
  private pump() {
    if (!this.pending && this.queue.length && this.online && this.writable) {
      const intent = this.queue.shift()!;
      const body = freeze(
        JSON.parse(
          JSON.stringify({
            operation_version: 1,
            base_revision: this.base.revision,
            operations: intent.operations,
          }),
        ) as Body,
      );
      this.pending = { key: intent.key, body, display: intent.operations };
      this.submit({ key: intent.key, body });
    }
    if (!this.dirty) {
      const waiters = this.waiters.splice(0);
      for (const w of waiters) {
        if (this.recovery.length)
          w.reject(new Error("collaboration_recovery_required"));
        else w.resolve(this.base);
      }
    }
  }
  receipt(receipt: Receipt) {
    const p = this.pending;
    if (!p || receipt.op_id !== p.key) return;
    if (p.receipt && JSON.stringify(p.receipt) !== JSON.stringify(receipt))
      return operationError("inconsistent_receipt");
    p.receipt = receipt;
    if (receipt.status === "rejected") {
      if (p.applied) return operationError("inconsistent_receipt");
      this.recoverPending(receipt.code ?? "rejected");
    } else if (p.applied) {
      if (receipt.revision !== this.appliedRevision)
        return operationError("inconsistent_receipt");
      this.pending = undefined;
    }
    this.pump();
    this.changed();
  }
  private appliedRevision?: number;
  private recoverPending(reason: string) {
    const p = this.pending!;
    this.recovery.push({ key: p.key, operations: p.body.operations, reason });
    const blocked = [...p.body.operations],
      remaining: Intent[] = [];
    for (const q of this.queue) {
      if (depends(blocked, q.operations)) {
        this.recovery.push({ ...q, reason: "dependency_rejected" });
        blocked.push(...q.operations);
      } else {
        try {
          applyOperations(this.base, q.operations);
          remaining.push(q);
        } catch {
          this.recovery.push({ ...q, reason: "precondition_changed" });
        }
      }
    }
    this.queue = remaining;
    this.pending = undefined;
  }
  changes(changes: Change[]) {
    for (const change of changes) {
      if (change.revision <= this.base.revision) continue;
      if (change.revision !== this.base.revision + 1)
        return operationError("history_unavailable");
      const before = this.base,
        p = this.pending;
      if (p && change.actor_id === this.actor && change.op_id === p.key) {
        if (p.receipt?.status === "rejected")
          return operationError("inconsistent_receipt");
        let predicted: Canvas | undefined;
        try {
          if (!p.invalid) predicted = applyOperations(before, p.display);
        } catch {
          /* The immutable request still requires a terminal receipt. */
        }
        this.base = {
          ...applyEffects(before, change.effects),
          revision: change.revision,
        };
        p.applied = true;
        this.appliedRevision = change.revision;
        // Canonical label/input revisions do not alter sequence positions. If a
        // server effect has an unexplained text/identity difference, retain Q for review.
        if (predicted && !this.compatibleNormalization(predicted, this.base)) {
          for (const q of this.queue)
            this.recovery.push({ ...q, reason: "canonical_effect_changed" });
          this.queue = [];
        }
        if (p.receipt) {
          if (p.receipt.revision !== change.revision)
            return operationError("inconsistent_receipt");
          this.pending = undefined;
        }
      } else {
        let carry = change.operations,
          state = before;
        if (p && !p.applied && !p.invalid) {
          try {
            const [remote, local] = transformOperationPair(
              state,
              carry,
              p.display,
            );
            state = applyOperations(state, p.display);
            p.display = local;
            carry = remote;
          } catch (error) {
            p.invalid =
              error instanceof Error ? error.message : "prediction_invalid";
            this.blockDependent(p.body.operations, p.invalid);
          }
        }
        const next: Intent[] = [];
        for (const q of this.queue) {
          try {
            const [remote, local] = transformOperationPair(
              state,
              carry,
              q.operations,
            );
            state = applyOperations(state, q.operations);
            carry = remote;
            next.push({ ...q, operations: local });
          } catch (error) {
            this.recovery.push({
              ...q,
              reason:
                error instanceof Error ? error.message : "precondition_changed",
            });
          }
        }
        this.queue = next;
        this.base = {
          ...applyEffects(before, change.effects),
          revision: change.revision,
        };
      }
    }
    this.pump();
    this.changed();
  }
  private blockDependent(operations: Operation[], reason: string) {
    const blocked = [...operations],
      remaining: Intent[] = [];
    for (const q of this.queue) {
      if (depends(blocked, q.operations)) {
        this.recovery.push({ ...q, reason });
        blocked.push(...q.operations);
      } else remaining.push(q);
    }
    this.queue = remaining;
  }
  private compatibleNormalization(predicted: Canvas, actual: Canvas) {
    // Text is never re-positioned by string diff. The transform kernel must explain
    // its exact new coordinates; unknown effects require explicit recovery.
    for (const n of predicted.nodes) {
      const other = actual.nodes.find((x) => x.id === n.id);
      if (!other) return false;
      for (const key of ["text"] as const)
        if (n.config[key] !== other.config[key]) return false;
      if (
        JSON.stringify(n.config.draft?.composer?.document) !==
        JSON.stringify(other.config.draft?.composer?.document)
      )
        return false;
    }
    return true;
  }
  flush(): Promise<Canvas> {
    if (!this.dirty)
      return this.recovery.length
        ? Promise.reject(new Error("collaboration_recovery_required"))
        : Promise.resolve(this.base);
    return new Promise((resolve, reject) =>
      this.waiters.push({ resolve, reject }),
    );
  }
  dispose() {
    this.online = false;
    for (const w of this.waiters.splice(0))
      w.reject(new Error("request_unconfirmed"));
  }
}
