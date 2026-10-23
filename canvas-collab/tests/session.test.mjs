import assert from "node:assert/strict";
import test from "node:test";
import * as Y from "yjs";
import {
  createCanvasDocument,
  readCanvasDocument,
  canvasUndoManager,
  applyCanvasHistory,
} from "../src/model.mjs";
import { CollaborationSession } from "../src/session.mjs";

const value = {
  title: "画布",
  graph: { schemaVersion: 2, nodes: [], connections: [] },
};
const binary = (bytes) => Buffer.from(bytes).toString("base64");
test("detached text is private, identity fenced and bounded; unresolved original submissions cannot be reclassified", () => {
  const f = fixture();
  try {
    const old = f.session.record();
    old.scope.sessionId = "old-editor";
    const text = {
      schemaVersion: 1,
      content: {
        type: "doc",
        content: [
          {
            type: "paragraph",
            content: [{ type: "text", text: "private tail" }],
          },
        ],
      },
    };
    const wrongActor = structuredClone(old);
    wrongActor.scope.actorKey = "USER:2";
    assert.equal(
      f.session.retainDetachedText(wrongActor, "n", text),
      undefined,
    );
    const unknown = structuredClone(old);
    unknown.inFlight = { updateId: "original-unknown" };
    assert.equal(f.session.retainDetachedText(unknown, "n", text), undefined);
    const original = structuredClone(old);
    const id = f.session.retainDetachedText(old, "n", text);
    assert.ok(id);
    assert.deepEqual(old, original);
    assert.deepEqual(f.session.recoverySources[0].textDrafts.n, text);
    assert.equal(f.session.retainDetachedText(old, "n", text), id);
    assert.equal(f.session.recoverySources.length, 1);
    assert.equal(f.requests.length, 0);
    assert.equal(f.session.pending.length, 0);
    assert.deepEqual(readCanvasDocument(f.session.doc), value);
    for (let index = 1; index < 16; index++) {
      const record = structuredClone(old);
      record.scope.sessionId = "old-editor-" + index;
      assert.ok(f.session.retainDetachedText(record, "n", text));
    }
    old.scope.sessionId = "over-budget";
    assert.equal(f.session.retainDetachedText(old, "n", text), undefined);
    assert.equal(f.session.recoverySources.length, 16);
  } finally {
    f.session.destroy();
    f.server.destroy();
  }
});
test("an unresolved restore command survives a clean graph and refresh without being replayed by the CRDT queue", async () => {
  const f = fixture();
  try {
    const control = {
      operation: "RESTORE",
      status: "UNKNOWN",
      body: {
        canvasId: "10",
        commandId: crypto.randomUUID(),
        expectedEpoch: "epoch",
        expectedCommitSeq: "0",
        expectedRevision: "1",
        expectedProjectBindingRevision: "0",
        snapshotId: "20",
      },
    };
    f.session.setControl(control);
    await f.session.protect();
    assert.deepEqual(f.records.at(-1).control, control);
    assert.equal(f.session.status, "CLEAN");
    assert.equal(await f.session.flush(), null);
    assert.equal(f.requests.length, 0);
    const restored = new CollaborationSession({
      scope: f.session.scope,
      initial: f.session.confirmed,
      recovery: f.records.at(-1),
      journal: f.session.journal,
      transport: f.session.transport,
    });
    try {
      assert.deepEqual(restored.control, control);
    } finally {
      restored.destroy();
    }
  } finally {
    f.session.destroy();
    f.server.destroy();
  }
});
function fixture(overrides = {}, initialValue = value, epoch = "epoch") {
  const server = createCanvasDocument(initialValue);
  const records = [],
    requests = [];
  let blocked = false,
    commits = 0;
  const session = new CollaborationSession({
    scope: { actorKey: "USER:1", canvasId: "10", sessionId: "window-a" },
    initial: {
      epoch,
      commitSeq: "0",
      contentRevision: "1",
      graphHash: "hash",
      update: binary(Y.encodeStateAsUpdate(server)),
    },
    journal: { put: async (record) => records.push(structuredClone(record)) },
    transport: {
      query: async () => null,
      submit: async (request) => {
        requests.push(structuredClone(request));
        if (blocked) throw new Error("network");
        Y.applyUpdate(server, Buffer.from(request.update, "base64"));
        commits++;
        return {
          ...request,
          commitSeq: String(commits),
          contentRevision: String(commits + 1),
          graphHash: "next",
          effect: "VISIBLE_CHANGE",
        };
      },
      ...overrides,
    },
  });
  return {
    server,
    session,
    records,
    requests,
    block: (next) => {
      blocked = next;
    },
  };
}
test("read-only result resolution cannot replay an uncertain original even when its receipt is not yet found", async () => {
  const f = fixture();
  try {
    f.session.change({
      ...f.session.snapshot,
      title: "private before revocation",
    });
    f.block(true);
    await assert.rejects(f.session.flush(), /network/);
    const original = structuredClone(f.session.inFlight);
    await assert.rejects(
      f.session.flush({ queryOnly: true }),
      (error) => error.code === "ORIGINAL_UNCONFIRMED",
    );
    assert.deepEqual(f.session.inFlight, original);
    assert.equal(f.requests.length, 1);
    assert.equal(f.session.automaticReplayAttempts, 0);
  } finally {
    f.session.destroy();
    f.server.destroy();
  }
});

test("automatic unknown replay is query-first, persisted before sending and cannot replenish its budget on refresh", async () => {
  let queries = 0,
    submissions = 0;
  const f = fixture({
    query: async () => {
      queries++;
      return null;
    },
    submit: async () => {
      submissions++;
      if (submissions > 1)
        assert.equal(f.records.at(-1).automaticReplayAttempts, submissions - 1);
      throw new Error("lost result");
    },
  });
  let resumed;
  try {
    f.session.change({ ...f.session.snapshot, title: "private draft" });
    await assert.rejects(f.session.flush(), /lost result/);
    const original = structuredClone(f.session.inFlight);
    for (let attempt = 1; attempt <= 2; attempt++) {
      await assert.rejects(f.session.flush({ automatic: true }), /lost result/);
      assert.equal(queries, attempt * 3);
      assert.equal(submissions, attempt + 1);
      assert.deepEqual(f.session.inFlight, original);
    }
    resumed = new CollaborationSession({
      scope: f.session.scope,
      initial: f.session.confirmed,
      recovery: f.session.record(),
      journal: f.session.journal,
      transport: f.session.transport,
    });
    await assert.rejects(resumed.flush({ automatic: true }), /REPLAY_BUDGET/);
    assert.equal(submissions, 3);
    assert.equal(resumed.status, "UNKNOWN");
    assert.deepEqual(resumed.inFlight, original);
    resumed.journal.put = async () => {
      throw new Error("quota");
    };
    resumed.automaticReplayAttempts = 0;
    await assert.rejects(
      resumed.flush({ automatic: true }),
      /RECOVERY_UNPROTECTED/,
    );
    assert.equal(
      submissions,
      3,
      "automatic replay cannot bypass a failed counter transaction",
    );
  } finally {
    resumed?.destroy();
    f.session.destroy();
    f.server.destroy();
  }
});

test("accepted bytes that lose a competing single-value field preserve the user's semantic input for local recovery", async () => {
  const f = fixture(
    {},
    {
      title: "canvas",
      graph: {
        schemaVersion: 2,
        connections: [],
        nodes: [
          {
            id: "n",
            type: "text",
            nodeTypeVersion: 1,
            title: "original",
            position: { x: 0, y: 0 },
            width: 200,
            height: 100,
          },
        ],
      },
    },
  );
  try {
    f.session.doc.clientID = 1;
    const local = structuredClone(f.session.snapshot);
    local.graph.nodes[0].title = "my intended title";
    f.session.change(local);
    const vector = Y.encodeStateVector(f.server);
    f.server.clientID = 2;
    f.server
      .getMap("canvas")
      .get("nodes")
      .get("n")
      .get("data")
      .set("title", "accepted peer title");
    await f.session.receive({
      epoch: "epoch",
      commitSeq: "1",
      contentRevision: "2",
      graphHash: "peer",
      update: binary(Y.encodeStateAsUpdate(f.server, vector)),
    });
    assert.equal(
      f.session.snapshot.graph.nodes[0].title,
      "accepted peer title",
    );
    f.session.transport.submit = async (request) => {
      Y.applyUpdate(f.server, Buffer.from(request.update, "base64"));
      return {
        ...request,
        commitSeq: "2",
        contentRevision: "2",
        graphHash: "peer",
        effect: "CAUSAL_CHANGE",
      };
    };
    await f.session.flush();
    assert.equal(
      f.session.status,
      "REJECTED",
      "durable receipt cannot pretend all visible intent prevailed",
    );
    assert.equal(
      f.session.inFlight,
      null,
      "the original outcome is known and must never be replayed",
    );
    assert.equal(f.session.sealed.reason, "UNAPPLIED_FIELDS");
    assert.equal(
      f.session.sealed.snapshot.graph.nodes[0].title,
      "my intended title",
    );
    assert.equal(
      f.session.snapshot.graph.nodes[0].title,
      "accepted peer title",
    );
    assert.equal(f.session.sealed.originalOutcome, "COMMITTED");
    assert.equal(f.session.sealed.intents[0].field, "title");
    assert.deepEqual(f.records.at(-1).sealed.intents, f.session.sealed.intents);
  } finally {
    f.session.destroy();
    f.server.destroy();
  }
});

test("a losing local field remains recoverable when its node is deleted before the original update is sent", async () => {
  const f = fixture(
    {},
    {
      title: "canvas",
      graph: {
        schemaVersion: 2,
        connections: [],
        nodes: [
          {
            id: "n",
            type: "text",
            nodeTypeVersion: 1,
            title: "original",
            position: { x: 0, y: 0 },
            width: 200,
            height: 100,
          },
        ],
      },
    },
  );
  try {
    f.session.doc.clientID = 1;
    const local = structuredClone(f.session.snapshot);
    local.graph.nodes[0].title = "private intent";
    f.session.change(local);
    let vector = Y.encodeStateVector(f.server);
    f.server.clientID = 2;
    f.server
      .getMap("canvas")
      .get("nodes")
      .get("n")
      .get("data")
      .set("title", "peer");
    await f.session.receive({
      epoch: "epoch",
      commitSeq: "1",
      contentRevision: "2",
      graphHash: "peer",
      update: binary(Y.encodeStateAsUpdate(f.server, vector)),
    });
    vector = Y.encodeStateVector(f.server);
    f.server.getMap("canvas").get("nodes").delete("n");
    await f.session.receive({
      epoch: "epoch",
      commitSeq: "2",
      contentRevision: "3",
      graphHash: "deleted",
      update: binary(Y.encodeStateAsUpdate(f.server, vector)),
    });
    assert.equal(f.session.status, "REJECTED");
    assert.equal(
      f.session.sealed.snapshot.graph.nodes[0].title,
      "private intent",
    );
    assert.equal(f.session.sealed.snapshot.graph.nodes[0].width, 200);
    assert.equal(f.session.sealed.intents[0].after.value, "private intent");
  } finally {
    f.session.destroy();
    f.server.destroy();
  }
});

test("undoing a field while its first value is in flight keeps a separate semantic intent for the later batch", async () => {
  const f = fixture(
    {},
    {
      title: "canvas",
      graph: {
        schemaVersion: 2,
        connections: [],
        nodes: [
          {
            id: "n",
            type: "text",
            nodeTypeVersion: 1,
            title: "original",
            position: { x: 0, y: 0 },
            width: 200,
            height: 100,
          },
        ],
      },
    },
  );
  let release, started;
  const held = new Promise((resolve) => {
    release = resolve;
  });
  const beginning = new Promise((resolve) => {
    started = resolve;
  });
  const submit = f.session.transport.submit;
  f.session.transport.submit = async (request) => {
    started();
    await held;
    return submit(request);
  };
  try {
    const moved = structuredClone(f.session.snapshot);
    moved.graph.nodes[0].position.x = 10;
    f.session.change(moved);
    const saving = f.session.flush();
    await beginning;
    const undone = structuredClone(f.session.snapshot);
    undone.graph.nodes[0].position.x = 0;
    f.session.change(undone);
    release();
    await saving;
    assert.equal(f.session.status, "DIRTY");
    const intent = f.session
      .record()
      .intents.find((value) => value.field === "placement");
    assert.equal(intent.after.value.position.x, 0);
    assert.equal(intent.before.value.position.x, 10);
    await f.session.flush();
    assert.equal(f.session.status, "CLEAN");
  } finally {
    release();
    f.session.destroy();
    f.server.destroy();
  }
});

test("a later peer edit is never mistaken for an unapplied original when its immutable business snapshot proves acceptance", async () => {
  const f = fixture();
  let reads = 0,
    accepted;
  try {
    f.session.change({ ...f.session.snapshot, title: "accepted original" });
    f.session.transport.submit = async (request) => {
      Y.applyUpdate(f.server, Buffer.from(request.update, "base64"));
      accepted = readCanvasDocument(f.server);
      f.server.getMap("canvas").set("title", "later peer edit");
      await f.session.receive({
        epoch: "epoch",
        commitSeq: "2",
        contentRevision: "3",
        graphHash: "later",
        snapshot: true,
        update: binary(Y.encodeStateAsUpdate(f.server)),
      });
      return {
        ...request,
        commitSeq: "1",
        contentRevision: "2",
        graphHash: "original",
        effect: "VISIBLE_CHANGE",
        snapshotId: "123",
      };
    };
    f.session.transport.confirmedSnapshot = async (receipt) => {
      reads++;
      assert.equal(receipt.snapshotId, "123");
      return accepted;
    };
    await f.session.flush();
    assert.equal(reads, 1);
    assert.equal(f.session.status, "CLEAN");
    assert.equal(f.session.sealed, null);
    assert.equal(f.session.snapshot.title, "later peer edit");
  } finally {
    f.session.destroy();
    f.server.destroy();
  }
});

test("refresh while checking an accepted prefix preserves its known receipt and never queries or resends that update", async () => {
  const f = fixture();
  let release, proofStarted, accepted, resumed;
  const held = new Promise((resolve) => {
    release = resolve;
  });
  const started = new Promise((resolve) => {
    proofStarted = resolve;
  });
  try {
    f.session.change({ ...f.session.snapshot, title: "accepted original" });
    f.session.transport.submit = async (request) => {
      Y.applyUpdate(f.server, Buffer.from(request.update, "base64"));
      accepted = readCanvasDocument(f.server);
      f.server.getMap("canvas").set("title", "later peer edit");
      await f.session.receive({
        epoch: "epoch",
        commitSeq: "2",
        contentRevision: "3",
        graphHash: "later",
        snapshot: true,
        update: binary(Y.encodeStateAsUpdate(f.server)),
      });
      return {
        ...request,
        commitSeq: "1",
        contentRevision: "2",
        graphHash: "original",
        effect: "VISIBLE_CHANGE",
        snapshotId: "123",
      };
    };
    f.session.transport.confirmedSnapshot = async () => {
      proofStarted();
      await held;
      return accepted;
    };
    const saving = f.session.flush();
    await started;
    const record = await f.session.persist();
    assert.equal(record.acceptedReceipt.snapshotId, "123");
    assert.equal(record.inFlight.updateId, record.acceptedReceipt.updateId);
    let requests = 0;
    resumed = new CollaborationSession({
      scope: f.session.scope,
      initial: f.session.confirmed,
      recovery: record,
      journal: f.session.journal,
      transport: {
        query: async () => {
          requests++;
          throw Error("must not query");
        },
        submit: async () => {
          requests++;
          throw Error("must not resend");
        },
        confirmedSnapshot: async () => accepted,
      },
    });
    await resumed.flush();
    assert.equal(requests, 0);
    assert.equal(resumed.status, "CLEAN");
    assert.equal(resumed.snapshot.title, "later peer edit");
    release();
    await saving;
  } finally {
    release();
    resumed?.destroy();
    f.session.destroy();
    f.server.destroy();
  }
});
test("a pending explicit undo can follow its own acknowledged delete without sealing or using a stale restoration watermark", async () => {
  const node = {
    id: "n",
    type: "text",
    nodeTypeVersion: 1,
    title: "text",
    position: { x: 0, y: 0 },
    width: 200,
    height: 100,
  };
  const epoch = crypto.randomUUID();
  const f = fixture(
    {},
    { ...value, graph: { ...value.graph, nodes: [node] } },
    epoch,
  );
  const undo = canvasUndoManager(f.session.doc);
  let release, submitted;
  const started = new Promise((resolve) => {
    submitted = resolve;
  });
  const held = new Promise((resolve) => {
    release = resolve;
  });
  const originalSubmit = f.session.transport.submit;
  let first = true;
  f.session.transport.submit = async (request) => {
    if (first) {
      first = false;
      submitted();
      await held;
    }
    return originalSubmit(request);
  };
  try {
    f.session.change(value);
    const saving = f.session.flush();
    await started;
    applyCanvasHistory(f.session.doc, undo, "undo", { epoch, commitSeq: "0" });
    const moved = structuredClone(f.session.snapshot);
    moved.graph.nodes[0].position.x = 10;
    f.session.change(moved);
    release();
    await saving;
    assert.equal(f.session.sealed, null);
    assert.equal(f.session.status, "DIRTY");
    assert.equal(
      f.session.doc.getMap("canvas").get("restoration").sourceCommitSeq,
      "1",
    );
    await f.session.flush();
    assert.equal(f.session.status, "CLEAN");
    assert.equal(readCanvasDocument(f.server).graph.nodes[0].position.x, 10);
  } finally {
    release();
    undo.destroy();
    f.session.destroy();
    f.server.destroy();
  }
});
test("a remote deletion preserves IME text that has reached the DOM but not a Yjs transaction, including a late composition tail", async () => {
  const node = {
    id: "n",
    type: "text",
    nodeTypeVersion: 1,
    title: "text",
    position: { x: 0, y: 0 },
    width: 200,
    height: 100,
  };
  const f = fixture({}, { ...value, graph: { ...value.graph, nodes: [node] } });
  const draft = (text) => ({
    schemaVersion: 1,
    content: {
      type: "doc",
      content: [{ type: "paragraph", content: [{ type: "text", text }] }],
    },
  });
  try {
    f.session.setTextDraft("n", draft("组合输入中"), node);
    const vector = Y.encodeStateVector(f.server);
    f.server.getMap("canvas").get("nodes").delete("n");
    await f.session.receive({
      epoch: "epoch",
      commitSeq: "1",
      contentRevision: "2",
      graphHash: "deleted",
      update: binary(Y.encodeStateAsUpdate(f.server, vector)),
    });
    assert.equal(f.session.status, "REJECTED");
    assert.equal(
      f.session.sealed.snapshot.graph.nodes[0].textDocument.content.content[0]
        .content[0].text,
      "组合输入中",
    );
    f.session.setTextDraft("n", draft("组合输入中文"), node);
    await f.session.protect();
    assert.equal(
      f.records.at(-1).sealed.snapshot.graph.nodes[0].textDocument.content
        .content[0].content[0].text,
      "组合输入中文",
    );
    assert.equal(f.session.snapshot.graph.nodes.length, 0);
    assert.equal(f.requests.length, 0);
  } finally {
    f.session.destroy();
    f.server.destroy();
  }
});

test("unknown result retains original frozen bytes and later input until its own ACK", async () => {
  const f = fixture();
  try {
    f.session.change({ ...value, title: "甲" });
    f.block(true);
    await assert.rejects(f.session.flush(), /network/);
    assert.equal(f.session.status, "UNKNOWN");
    const original = structuredClone(f.requests[0]);
    f.session.change({ ...value, title: "甲乙" });
    f.block(false);
    await f.session.flush();
    assert.deepEqual(f.requests[1], original);
    assert.equal(f.session.status, "DIRTY");
    await f.session.flush();
    assert.equal(f.session.status, "CLEAN");
    assert.equal(readCanvasDocument(f.server).title, "甲乙");
    assert.equal(f.records[0].inFlight.update, original.update);
    assert.notEqual(f.requests[2].updateId, original.updateId);
  } finally {
    f.session.destroy();
    f.server.destroy();
  }
});
test("a denied original-result query cannot prove that an unknown write was rejected", async () => {
  const f = fixture();
  try {
    f.session.change({ ...value, title: "unknown" });
    f.block(true);
    await assert.rejects(f.session.flush(), /network/);
    const original = structuredClone(f.session.inFlight);
    f.session.transport.query = async () => {
      throw Object.assign(new Error("access lost"), { definitive: true });
    };
    await assert.rejects(f.session.flush(), /access lost/);
    assert.equal(f.session.status, "UNKNOWN");
    assert.deepEqual(f.session.inFlight, original);
    assert.equal(f.session.sealed, null);
  } finally {
    f.session.destroy();
    f.server.destroy();
  }
});

test("a definitively denied replay keeps the unknown original queryable and never replays its dependent chain", async () => {
  const f = fixture();
  try {
    f.session.change({ ...value, title: "original" });
    f.block(true);
    await assert.rejects(f.session.flush(), /network/);
    const original = structuredClone(f.session.inFlight);
    f.session.change({ ...value, title: "later" });
    let replays = 0;
    f.session.transport.submit = async () => {
      replays++;
      throw Object.assign(new Error("replay denied"), {
        definitive: true,
        code: "IMMUTABLE_FIELD",
      });
    };
    await assert.rejects(f.session.flush(), /replay denied/);
    assert.equal(f.session.status, "UNKNOWN");
    assert.equal(f.session.inFlight.updateId, original.updateId);
    assert.equal(f.session.sealed.snapshot.title, "later");
    await assert.rejects(f.session.flush(), /ORIGINAL_UNCONFIRMED/);
    assert.equal(replays, 1);
    Y.applyUpdate(f.server, Buffer.from(original.update, "base64"));
    f.session.transport.query = async () => ({
      ...original,
      commitSeq: "1",
      contentRevision: "2",
      graphHash: "accepted",
      effect: "VISIBLE_CHANGE",
    });
    await f.session.flush();
    assert.equal(f.session.inFlight, null);
    assert.equal(f.session.status, "REJECTED");
    assert.equal(f.session.snapshot.title, "original");
    assert.equal(f.session.sealed.snapshot.title, "later");
  } finally {
    f.session.destroy();
    f.server.destroy();
  }
});

test("committed remote content never enters the local send queue", async () => {
  const f = fixture();
  try {
    const before = Y.encodeStateVector(f.server);
    f.server.getMap("canvas").set("title", "远端");
    await f.session.receive({
      epoch: "epoch",
      commitSeq: "1",
      contentRevision: "2",
      graphHash: "remote",
      update: binary(Y.encodeStateAsUpdate(f.server, before)),
    });
    assert.equal(f.session.snapshot.title, "远端");
    assert.equal(f.session.status, "CLEAN");
    await f.session.flush();
    assert.equal(f.requests.length, 0);
  } finally {
    f.session.destroy();
    f.server.destroy();
  }
});

test("business rejection seals the dependent local chain before replacing the polluted document", async () => {
  const f = fixture({
    submit: async () => {
      throw Object.assign(new Error("invalid"), {
        definitive: true,
        code: "INVALID_GRAPH",
      });
    },
  });
  try {
    const polluted = f.session.doc;
    f.session.change({ ...value, title: "被拒稿" });
    await assert.rejects(f.session.flush(), /invalid/);
    assert.equal(f.session.status, "REJECTED");
    assert.notEqual(f.session.doc, polluted);
    assert.equal(f.session.snapshot.title, "画布");
    assert.equal(f.records.at(-1).sealed.snapshot.title, "被拒稿");
    assert.throws(
      () => f.session.change({ ...value, title: "不能继续" }),
      /RECOVERY_REQUIRED/,
    );
  } finally {
    f.session.destroy();
    f.server.destroy();
  }
});

test("unavailable journal preserves online saving without claiming local protection", async () => {
  const f = fixture();
  try {
    f.session.journal.put = async () => {
      throw new Error("quota");
    };
    f.session.change({ ...value, title: "本地" });
    await f.session.flush();
    assert.equal(f.requests.length, 1);
    assert.equal(f.session.protected, false);
    assert.equal(f.session.snapshot.title, "本地");
  } finally {
    f.session.destroy();
    f.server.destroy();
  }
});

test("refresh queries the frozen original and keeps a sealed draft out of the fresh document", async () => {
  const f = fixture({
    submit: async () => {
      throw Object.assign(new Error("rejected"), {
        definitive: true,
        code: "INVALID_GRAPH",
      });
    },
  });
  let restored;
  try {
    f.session.change({ ...value, title: "封存" });
    await assert.rejects(f.session.flush());
    restored = new CollaborationSession({
      scope: f.session.scope,
      initial: f.session.confirmed,
      recovery: f.records.at(-1),
      journal: f.session.journal,
      transport: f.session.transport,
    });
    assert.equal(restored.status, "REJECTED");
    assert.equal(restored.snapshot.title, "画布");
  } finally {
    restored?.destroy();
    f.session.destroy();
    f.server.destroy();
  }
});

test("ACK persistence failure preserves the known durable result without resending", async () => {
  const f = fixture();
  try {
    f.session.change({ ...value, title: "已提交" });
    const save = f.session.journal.put;
    f.session.journal.put = async (record) => {
      if (!record.inFlight) throw new Error("quota after ACK");
      return save(record);
    };
    await f.session.flush();
    assert.equal(f.session.status, "CLEAN");
    assert.equal(f.session.protected, false);
    await f.session.flush();
    assert.equal(f.requests.length, 1);
  } finally {
    f.session.destroy();
    f.server.destroy();
  }
});

test("a crash while hashing preserves unsent bytes as pending and recovery continues its persisted generation", async () => {
  const f = fixture();
  let restored;
  try {
    f.session.change({ ...value, title: "未发送稿" });
    const sending = f.session.flush();
    const crash = f.session.record();
    assert.equal(crash.inFlight, null);
    assert.equal(crash.pending.length, 1);
    await sending;
    restored = new CollaborationSession({
      scope: f.session.scope,
      initial: f.session.confirmed,
      recovery: { ...crash, generation: 7 },
      journal: f.session.journal,
      transport: f.session.transport,
    });
    assert.equal(restored.generation, 7);
    assert.equal(restored.snapshot.title, "未发送稿");
  } finally {
    restored?.destroy();
    f.session.destroy();
    f.server.destroy();
  }
});

test("remote deletion seals a readable preimage of unconfirmed node edits instead of silently acknowledging hidden CRDT changes", async () => {
  const source = {
    ...value,
    graph: {
      ...value.graph,
      nodes: [
        {
          id: "n",
          type: "group",
          nodeTypeVersion: 1,
          title: "原节点",
          position: { x: 0, y: 0 },
          width: 320,
          height: 200,
          zIndex: 0,
        },
      ],
    },
  };
  const f = fixture({}, source);
  try {
    const local = structuredClone(source);
    local.graph.nodes[0].title = "未确认输入";
    f.session.change(local);
    const before = Y.encodeStateVector(f.server);
    f.server.getMap("canvas").get("nodes").delete("n");
    await f.session.receive({
      epoch: "epoch",
      commitSeq: "1",
      contentRevision: "2",
      graphHash: "deleted",
      update: binary(Y.encodeStateAsUpdate(f.server, before)),
    });
    assert.equal(f.session.snapshot.graph.nodes.length, 0);
    assert.equal(f.session.status, "REJECTED");
    assert.equal(
      f.records.at(-1).sealed.snapshot.graph.nodes[0].title,
      "未确认输入",
    );
    await assert.rejects(f.session.flush(), /RECOVERY_REQUIRED/);
    assert.equal(f.requests.length, 0);
  } finally {
    f.session.destroy();
    f.server.destroy();
  }
});
