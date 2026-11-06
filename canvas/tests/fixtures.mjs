import * as Y from "yjs";

export const scope = Object.freeze({
  canvasId: "test-canvas",
  epoch: "epoch-1",
  actor: "USER:1",
  sessionId: "window-a",
});

export function documentFrom(update) {
  const doc = new Y.Doc();
  if (update) Y.applyUpdate(doc, update);
  return doc;
}

export function change(doc, mutate) {
  const updates = [];
  const capture = (update) => updates.push(update);
  doc.on("update", capture);
  try {
    doc.transact(() => mutate(doc));
  } finally {
    doc.off("update", capture);
  }
  return Y.mergeUpdates(updates);
}

export function project(doc) {
  return {
    nodes: doc.getMap("nodes").toJSON(),
    edges: doc.getMap("edges").toJSON(),
    text: doc.getText("text").toString(),
  };
}

export function validate(graph) {
  const visit = (id, path = new Set()) => {
    if (path.has(id))
      throw Object.assign(new Error("cycle"), { code: "INVALID_GRAPH" });
    const next = new Set(path).add(id);
    for (const edge of Object.values(graph.edges)) {
      if (!graph.nodes[edge.from] || !graph.nodes[edge.to])
        throw Object.assign(new Error("missing endpoint"), {
          code: "INVALID_GRAPH",
        });
      if (edge.from === id) visit(edge.to, next);
    }
  };
  Object.keys(graph.nodes).forEach((id) => visit(id));
}

// Test collaborator: actual SQL transactions are exercised separately by test:mysql.
export class MemoryStore {
  constructor(doc = new Y.Doc()) {
    this.state = {
      epoch: scope.epoch,
      commitSeq: "0",
      update: Y.encodeStateAsUpdate(doc),
    };
    this.receipts = new Map();
    this.commits = [];
  }
  async load() {
    return structuredClone(this.state);
  }
  async receipt(binding, id) {
    return this.receipts.get(`${binding.actor}:${id}`);
  }
  async commit(binding, candidate) {
    if (candidate.expectedCommitSeq !== this.state.commitSeq)
      return { committed: false };
    const receipt = {
      updateId: candidate.updateId,
      payloadHash: candidate.payloadHash,
      epoch: binding.epoch,
      commitSeq: String(BigInt(this.state.commitSeq) + 1n),
      graphHash: candidate.graphHash,
      effect: candidate.effect,
    };
    this.state = {
      epoch: binding.epoch,
      commitSeq: receipt.commitSeq,
      update: candidate.state,
    };
    this.receipts.set(`${binding.actor}:${candidate.updateId}`, receipt);
    this.commits.push(candidate);
    return { committed: true, receipt };
  }
}
