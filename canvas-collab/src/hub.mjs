import { AdmissionGate, digest } from "./admission.mjs";
import { base64, unbase64 } from "./binary.mjs";
import { validateCanvasRoots } from "./canvas-roots.mjs";
import {
  readCanvasDocument,
  inspectCanvasDocument,
  validateCanvasTransition,
} from "./model.mjs";

const fail = (code) => Object.assign(new Error(code), { code });
const sameRoom = (left, right) =>
  left.canvasId === right.canvasId && left.epoch === right.epoch;

/** Thin committed-delivery coordinator. Java remains the authority for every read and write. */
export class CollaborationHub {
  constructor({ store, maxConnections = 128, maxAdmissions = 4 }) {
    this.store = store;
    this.maxConnections = maxConnections;
    this.maxAdmissions = maxAdmissions;
    this.admissions = 0;
    this.reads = 0;
    this.readWaiters = [];
    this.sessions = new Set();
    this.gate = new AdmissionGate({
      store,
      allowedRoots: ["canvas"],
      validateRoots: validateCanvasRoots,
      project: readCanvasDocument,
      inspectDocument: inspectCanvasDocument,
      validateTransition: validateCanvasTransition,
      validate: () => {},
      authorize: (scope) =>
        [...this.sessions].some(
          (session) => session.scope === scope && !session.closed,
        ),
    });
  }

  async join(scope, send, drop) {
    if (this.sessions.size >= this.maxConnections)
      throw fail("CONNECTION_BUDGET");
    const session = {
      scope: Object.freeze({ ...scope }),
      send,
      drop,
      closed: false,
      seen: -1n,
      queue: Promise.resolve(),
      queued: 0,
      refreshing: null,
      refreshNeeded: false,
    };
    session.send = (message) =>
      send({
        ...message,
        actorKey: session.scope.actorKey,
        canvasId: session.scope.canvasId,
        sessionId: session.scope.sessionId,
      });
    this.sessions.add(session);
    try {
      await this.refresh(session);
      // Close the bootstrap/subscription race by reconciling once after the first delivery.
      await this.refresh(session);
      return session;
    } catch (error) {
      this.leave(session);
      throw error;
    }
  }

  renew(session, binding) {
    for (const name of [
      "actorKey",
      "canvasId",
      "epoch",
      "bindingRevision",
      "sessionId",
      "connectionId",
    ])
      if (session.scope[name] !== binding[name]) throw fail("BINDING_CHANGED");
    session.scope = Object.freeze({ ...binding });
  }

  enqueue(session, action) {
    if (session.closed) return Promise.reject(fail("CONNECTION_CLOSED"));
    if (session.queued >= 2) return Promise.reject(fail("DELIVERY_BUDGET"));
    session.queued++;
    const operation = session.queue
      .catch(() => {})
      .then(async () => {
        if (session.closed) throw fail("CONNECTION_CLOSED");
        return action();
      });
    session.queue = operation;
    return operation.finally(() => {
      session.queued--;
    });
  }

  async deliver(session, loaded, hint) {
    const seq = BigInt(loaded.commitSeq);
    if (seq <= session.seen) {
      if (
        session.lifecycle !== loaded.metadata.lifecycle ||
        session.edit !== loaded.metadata.edit
      )
        await session.send({
          type: "WATERMARK",
          epoch: loaded.epoch,
          commitSeq: String(seq),
          lifecycle: loaded.metadata.lifecycle,
          edit: loaded.metadata.edit,
        });
      session.lifecycle = loaded.metadata.lifecycle;
      session.edit = loaded.metadata.edit;
      return;
    }
    const delta =
      hint &&
      BigInt(hint.receipt.commitSeq) === seq &&
      session.seen + 1n === seq;
    await session.send({
      type: delta ? "COMMITTED_UPDATE" : "SYNC_STATE",
      snapshot: !delta,
      epoch: loaded.epoch,
      commitSeq: String(seq),
      contentRevision: loaded.metadata.contentRevision,
      bindingRevision: loaded.metadata.bindingRevision,
      modelVersion: loaded.metadata.modelVersion,
      graphHash: loaded.metadata.graphHash,
      lifecycle: loaded.metadata.lifecycle,
      edit: loaded.metadata.edit,
      update: base64(delta ? hint.update : loaded.update),
    });
    session.seen = seq;
    session.lifecycle = loaded.metadata.lifecycle;
    session.edit = loaded.metadata.edit;
  }

  refresh(session, hint) {
    if (session.refreshing) {
      session.refreshNeeded = true;
      return session.refreshing;
    }
    const operation = this.enqueue(session, async () => {
      do {
        session.refreshNeeded = false;
        await this.withRead(async () => {
          if (session.closed) throw fail("CONNECTION_CLOSED");
          const loaded = await this.store.load(session.scope);
          await this.deliver(session, loaded, hint);
        });
        hint = undefined;
      } while (session.refreshNeeded && !session.closed);
    });
    session.refreshing = operation;
    operation
      .finally(() => {
        session.refreshing = null;
      })
      .catch(() => {});
    return operation;
  }

  async message(session, message) {
    if (session.closed || !this.sessions.has(session))
      throw fail("CONNECTION_CLOSED");
    if (!/^[0-9a-f-]{36}$/.test(message.requestId))
      throw fail("INVALID_REQUEST");
    if (message.type === "SYNC_STATE") {
      await this.refresh(session);
      return;
    }
    if (message.type === "QUERY_RESULT") {
      const receipt = await this.store.receipt(
        { ...session.scope, epoch: message.epoch },
        message.updateId,
      );
      await this.refresh(session);
      await this.enqueue(session, () =>
        session.send({
          type: "QUERY_RESULT",
          requestId: message.requestId,
          receipt,
        }),
      );
      return;
    }
    if (message.type !== "SUBMIT_UPDATE") throw fail("INVALID_REQUEST");
    if (this.admissions >= this.maxAdmissions || session.accepting)
      throw fail("ADMISSION_BUSY");
    this.admissions++;
    session.accepting = true;
    try {
      const update = unbase64(message.update);
      if (digest(update) !== message.payloadHash) throw fail("PAYLOAD_HASH");
      const receipt = await this.gate.submit(session.scope, {
        ...message,
        update,
      });
      const hint = { receipt, update };
      for (const peer of this.sessions) {
        if (peer === session || !sameRoom(peer.scope, session.scope)) continue;
        void this.refresh(peer, hint).catch(() => this.leave(peer));
      }
      // Ensure an ACK for a newer durable prefix is preceded by its committed content.
      try {
        await this.refresh(session, hint);
        await this.enqueue(session, () =>
          session.send({
            type: "DURABLE_ACK",
            requestId: message.requestId,
            receipt,
          }),
        );
      } catch {
        // A failed read/send after commit cannot prove that the original update was rejected.
        throw Object.assign(fail("DELIVERY_UNCONFIRMED"), {
          definitive: false,
        });
      }
    } finally {
      this.admissions--;
      session.accepting = false;
    }
  }

  async wake(canvasId, epoch) {
    await Promise.allSettled(
      [...this.sessions]
        .filter((session) => session.scope.canvasId === canvasId)
        .map((session) =>
          this.refresh(session).catch(() => this.leave(session)),
        ),
    );
  }

  async withRead(action) {
    if (this.reads < this.maxAdmissions) this.reads++;
    else {
      if (this.readWaiters.length >= this.maxConnections)
        throw fail("READ_BUDGET");
      await new Promise((resolve) => this.readWaiters.push(resolve));
    }
    try {
      return await action();
    } finally {
      const next = this.readWaiters.shift();
      if (next) next();
      else this.reads--;
    }
  }

  async idle() {
    await Promise.allSettled(
      [...this.sessions].map((session) => session.queue),
    );
  }
  leave(session) {
    if (session.closed) return;
    session.closed = true;
    this.sessions.delete(session);
    session.drop();
  }
  close() {
    for (const session of [...this.sessions]) this.leave(session);
  }
}
