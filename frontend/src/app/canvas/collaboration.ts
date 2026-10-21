import { canonical } from "@/components/generation/draft-save";
import type { Canvas } from "./document";
import {
  applyEffects,
  fieldPolicy,
  usesNode,
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
type HistoryIntent = { direction: "undo" | "redo"; target: string };
type Intent = { key: string; operations: Operation[]; history?: HistoryIntent };
type Pending = Envelope & {
  history?: HistoryIntent;
  display: Operation[];
  receipt?: Receipt;
  applied?: boolean;
  invalid?: string;
};
export type CollaborationTransition = {
  kind: "local" | "remote" | "canonical" | "status";
  operations?: Operation[];
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
  const paths = (op: Operation): string[][] => {
    const p = op.path ?? [];
    try {
      const group = fieldPolicy(p).group;
      return group?.length ? group.map((k) => [...p.slice(0, -1), k]) : [p];
    } catch {
      return [p];
    }
  };
  return b.some((y) =>
    a.some((x) => {
      if (x.type === "create_node" && usesNode(y, x.node?.id)) return true;
      if (x.node_id !== y.node_id) return false;
      if (
        x.type === "delete_node" ||
        x.type === "migrate_composer" ||
        y.type === "delete_node"
      )
        return true;
      if (
        paths(x).some((p) =>
          paths(y).some((q) =>
            p
              .slice(0, Math.min(p.length, q.length))
              .every((v, i) => v === q[i]),
          ),
        )
      )
        return true;
      const p = x.path ?? [],
        q = y.path ?? [];
      if (
        x.item_id &&
        (q.includes("@" + x.item_id) ||
          x.item_id === y.before_id ||
          y.text?.some((c) =>
            c.insert?.some(
              (t) => t.type === "reference" && t.reference_id === x.item_id,
            ),
          ))
      )
        return true;
      // A model/source premise only constrains its own finite container.
      if (["model", "source"].includes(p.at(-1) ?? "")) {
        try {
          const context = fieldPolicy(q).context;
          if (
            context &&
            context === p.at(-1) &&
            p.slice(0, -1).every((v, i) => v === q[i])
          )
            return true;
        } catch {
          /* closed structural path */
        }
      }
      return false;
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
  private undoStack: string[] = [];
  private redoStack: string[] = [];
  lastRejection?: Receipt;
  invalidDraft = false;
  private composing = false;
  private resynchronizing = false;
  private draining = false;
  private held: ({ changes: Change[] } | { receipt: Receipt })[] = [];
  private heldBytes = 0;
  private waiters: {
    resolve: (doc: Canvas) => void;
    reject: (error: Error) => void;
  }[] = [];
  constructor(
    initial: Canvas,
    private changed: (transition?: CollaborationTransition) => void,
    private submit: (envelope: Envelope) => void,
    private options: {
      validate?: (document: Canvas) => boolean;
    } = {},
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
  get isComposing() {
    return this.composing;
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
    const continuation =
      this.composing &&
      operations.every(
        (op) =>
          op.type === "text" ||
          (op.type === "set" && op.path?.join(".") === "config.text_source"),
      );
    if ((!this.writable || this.resynchronizing) && !continuation)
      return operationError("connection_read_only");
    if (this.pending?.body.undo_of) operationError("undo_pending");
    if (!operations.length) return;
    // Symbols and any prediction-only labels can never enter the wire intent.
    const clean = JSON.parse(JSON.stringify(operations)) as Operation[];
    applyOperations(this.document, clean);
    if (
      new TextEncoder().encode(
        JSON.stringify({
          type: "resolve",
          op_id: key,
          body: {
            operation_version: 1,
            base_revision: 2147483647,
            operations: clean,
          },
        }),
      ).length >
      1 << 20
    )
      operationError("canvas_request_too_large");
    // Bound the page-memory backlog during a slow or disconnected transport.
    // Keep the accepted queue intact if the new intent exceeds this budget.
    if (
      new TextEncoder().encode(
        JSON.stringify([...this.queue.map((item) => item.operations), clean]),
      ).length >
      1 << 20
    )
      operationError("canvas_request_too_large");
    this.queue.push({ key, operations: clean });
    this.pump();
    this.changed();
  }
  private pump() {
    if (this.composing || this.draining || this.resynchronizing) return;
    if (!this.pending && this.queue.length && this.online && this.writable) {
      const first = this.queue[0];
      let count = 0,
        operations: Operation[] = [];
      for (const item of this.queue) {
        if (count > 0 && (first.history || item.history)) break;
        const joined = [...operations, ...item.operations];
        if (
          joined.length > 100 ||
          new TextEncoder().encode(
            JSON.stringify({
              type: "resolve",
              op_id: first.key,
              body: {
                operation_version: 1,
                base_revision: 2147483647,
                operations: joined,
              },
            }),
          ).length >
            1 << 20
        )
          break;
        operations = joined;
        count++;
      }
      if (count === 0) operationError("canvas_request_too_large");
      const valid =
        !this.options.validate ||
        this.options.validate(applyOperations(this.base, operations));
      if (!valid) {
        this.invalidDraft = true;
        for (const waiter of this.waiters.splice(0))
          waiter.reject(new Error("invalid_draft"));
        return;
      }
      this.invalidDraft = false;
      this.queue.splice(0, count);
      const intent = { ...first, operations };
      const body = freeze(
        JSON.parse(
          JSON.stringify({
            operation_version: 1,
            base_revision: this.base.revision,
            operations: intent.operations,
            ...(intent.history ? { undo_of: intent.history.target } : {}),
          }),
        ) as Body,
      );
      this.pending = {
        key: intent.key,
        body,
        display: intent.operations,
        history: intent.history,
      };
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
    if (this.composing) {
      this.hold({ receipt });
      return;
    }
    const p = this.pending;
    if (!p || receipt.op_id !== p.key) return;
    if (p.receipt && JSON.stringify(p.receipt) !== JSON.stringify(receipt))
      return operationError("inconsistent_receipt");
    p.receipt = receipt;
    if (receipt.status === "rejected") {
      if (p.applied) return operationError("inconsistent_receipt");
      this.lastRejection = receipt;
      if (p.history) {
        const stack =
          p.history.direction === "undo" ? this.undoStack : this.redoStack;
        if (stack.at(-1) === p.history.target) stack.pop();
        this.pending = undefined;
      } else this.recoverPending(receipt.code ?? "rejected");
    } else if (p.applied) {
      if (receipt.revision !== this.appliedRevision)
        return operationError("inconsistent_receipt");
      this.acceptPending(p);
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
    let state = this.base;
    for (const q of this.queue) {
      if (depends(blocked, q.operations)) {
        this.recovery.push({ ...q, reason: "dependency_rejected" });
        blocked.push(...q.operations);
      } else {
        try {
          state = applyOperations(state, q.operations);
          remaining.push(q);
        } catch {
          this.recovery.push({ ...q, reason: "precondition_changed" });
          blocked.push(...q.operations);
        }
      }
    }
    this.queue = remaining;
    this.pending = undefined;
  }
  changes(changes: Change[]) {
    if (this.composing) {
      this.hold({ changes });
      return;
    }
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
        this.changed({ kind: "canonical", operations: [] });
        if (p.receipt) {
          if (p.receipt.revision !== change.revision)
            return operationError("inconsistent_receipt");
          this.acceptPending(p);
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
        const next: Intent[] = [],
          failed: Operation[] = [];
        for (const q of this.queue) {
          try {
            if (depends(failed, q.operations))
              throw new Error("dependency_rejected");
            const [remote, local] = transformOperationPair(
              state,
              carry,
              q.operations,
            );
            state = applyOperations(state, q.operations);
            carry = remote;
            next.push({ ...q, operations: local });
          } catch (error) {
            failed.push(...q.operations);
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
        this.changed({ kind: "remote", operations: carry });
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
  private acceptPending(p: Pending) {
    if (p.history) {
      if (p.history.direction === "undo") {
        if (this.undoStack.at(-1) === p.history.target) this.undoStack.pop();
        this.redoStack.push(p.key);
      } else {
        if (this.redoStack.at(-1) === p.history.target) this.redoStack.pop();
        this.undoStack.push(p.key);
      }
    } else if (
      p.body.operations.some(
        (op) => !["noop", "migrate_composer"].includes(op.type),
      )
    ) {
      this.undoStack.push(p.key);
      this.redoStack = [];
    }
    this.pending = undefined;
  }
  get canUndo() {
    return (
      this.online &&
      this.writable &&
      !this.dirty &&
      !this.recovery.length &&
      !this.composing &&
      !this.resynchronizing &&
      this.undoStack.length > 0
    );
  }
  get canRedo() {
    return (
      this.online &&
      this.writable &&
      !this.dirty &&
      !this.recovery.length &&
      !this.composing &&
      !this.resynchronizing &&
      this.redoStack.length > 0
    );
  }
  history(direction: "undo" | "redo") {
    if (!(direction === "undo" ? this.canUndo : this.canRedo)) return false;
    const target = (direction === "undo" ? this.undoStack : this.redoStack).at(
      -1,
    )!;
    this.queue.push({
      key: crypto.randomUUID(),
      operations: [],
      history: { direction, target },
    });
    this.pump();
    this.changed();
    return true;
  }
  get canVerifyResync() {
    return !this.composing && (!this.pending || !!this.pending.receipt);
  }
  requireResync() {
    this.resynchronizing = true;
    this.changed();
  }
  verifiedResync(snapshot: Canvas) {
    if (
      snapshot.id !== this.base.id ||
      snapshot.project_id !== this.base.project_id ||
      snapshot.revision < this.base.revision
    )
      operationError("invalid_receipt");
    if (this.composing) operationError("composition_in_progress");
    const p = this.pending;
    if (p && !p.receipt) operationError("request_unconfirmed");
    if (p?.receipt?.status === "accepted") {
      if (!p.receipt.revision || snapshot.revision < p.receipt.revision)
        operationError("invalid_receipt");
      this.acceptPending(p);
    } else if (p) {
      this.recovery.push({
        key: p.key,
        operations: p.body.operations,
        reason: p.receipt?.code ?? "history_unavailable",
      });
      this.pending = undefined;
    }
    for (const q of this.queue)
      this.recovery.push({ ...q, reason: "history_unavailable" });
    this.queue = [];
    this.base = structuredClone(snapshot);
    this.resynchronizing = false;
    this.changed({ kind: "canonical" });
    this.pump();
  }
  discardRecovery() {
    if (this.dirty) operationError("request_unconfirmed");
    this.recovery = [];
    this.lastRejection = undefined;
    this.changed();
  }
  private hold(event: { changes: Change[] } | { receipt: Receipt }) {
    const bytes = new TextEncoder().encode(JSON.stringify(event)).length;
    if (this.held.length >= 128 || this.heldBytes + bytes > 4 << 20)
      operationError("history_unavailable");
    this.held.push(event);
    this.heldBytes += bytes;
  }
  composition(active: boolean) {
    this.composing = active;
    if (!active) {
      this.draining = true;
      try {
        for (const event of this.held.splice(0)) {
          if ("changes" in event) this.changes(event.changes);
          else this.receipt(event.receipt);
        }
      } finally {
        this.heldBytes = 0;
        this.draining = false;
      }
      this.pump();
      this.changed();
    }
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
        canonical(n.config.draft?.composer?.document) !==
        canonical(other.config.draft?.composer?.document)
      )
        return false;
    }
    return true;
  }
  flush(): Promise<Canvas> {
    if (!this.online || !this.writable || this.resynchronizing)
      return Promise.reject(new Error("request_unconfirmed"));
    if (!this.dirty)
      return this.recovery.length
        ? Promise.reject(new Error("collaboration_recovery_required"))
        : Promise.resolve(this.base);
    const promise = new Promise<Canvas>((resolve, reject) =>
      this.waiters.push({ resolve, reject }),
    );
    this.pump();
    return promise;
  }
  dispose() {
    this.online = false;
    for (const w of this.waiters.splice(0))
      w.reject(new Error("request_unconfirmed"));
    this.pending = undefined;
    this.queue = [];
    this.recovery = [];
    this.held = [];
    this.heldBytes = 0;
    this.undoStack = [];
    this.redoStack = [];
    this.base = { ...this.base, nodes: [], edges: [] };
  }
}
