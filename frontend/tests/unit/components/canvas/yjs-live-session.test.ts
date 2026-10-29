import { createCanvasNode } from "@/components/canvas/lib/canvas-node-factory";
import { afterEach, expect, it, vi } from "vitest";
import * as Y from "yjs";
import {
  createCanvasDocument,
  readCanvasDocument,
} from "@glowloom/canvas-collab/model";
import {
  serviceSnapshot,
  serviceProjection,
  type ServiceDocument,
} from "@glowloom/canvas-collab/service-document";
import { CanvasYjsLiveSession } from "@/components/canvas/yjs-live-session";
import { RequestError } from "@/lib/request";
import { webcrypto } from "node:crypto";
class Socket {
  readyState = 1;
  sent: Record<string, unknown>[] = [];
  onopen?: () => void;
  onmessage?: (event: { data: string }) => void;
  onclose?: () => void;
  send(value: string) {
    this.sent.push(JSON.parse(value));
  }
  close() {
    this.readyState = 3;
    this.onclose?.();
  }
  receive(value: unknown) {
    this.onmessage?.({ data: JSON.stringify(value) });
  }
}
const instances: CanvasYjsLiveSession[] = [];
afterEach(() => {
  instances.splice(0).forEach((client) => client.stop());
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});
function setup(
  twoNodes = false,
  renew: (signal: AbortSignal) => Promise<void> = async () => {},
) {
  vi.stubGlobal("crypto", webcrypto);
  const actorID = crypto.randomUUID(),
    canvasID = crypto.randomUUID(),
    projectID = crypto.randomUUID(),
    scopeID = crypto.randomUUID(),
    nodeID = crypto.randomUUID();
  const document: API.DocumentDTO = {
    id: canvasID,
    project_id: projectID,
    name: "协作",
    schema_version: 1,
    operation_version: 1,
    revision: "1",
    permissions: { read: true, write: true },
    nodes: [
      {
        id: nodeID,
        title: "正文",
        node_type: "text",
        node_action: "resource",
        x: 0,
        y: 0,
        width: 320,
        height: 200,
        z_index: 1,
        content_version: "1",
        config: { text: "AB", format: "plain", format_version: 1 },
      },
    ],
    edges: [],
  };
  if (twoNodes)
    document.nodes!.push({
      ...structuredClone(document.nodes![0]),
      id: crypto.randomUUID(),
      title: "参考源",
    });
  const doc = createCanvasDocument(
    serviceSnapshot(document as unknown as ServiceDocument),
  );
  const state = {
    documentId: canvasID,
    epoch: crypto.randomUUID(),
    writerTerm: "1",
    commitSeq: "0",
    state: Buffer.from(Y.encodeStateAsUpdate(doc)).toString("base64"),
    graphHash: "a".repeat(64),
    document,
  };
  doc.destroy();
  const sockets: Socket[] = [];
  const client = new CanvasYjsLiveSession({
    canvasID,
    projectID,
    actorID,
    scopeID,
    csrf: () => "csrf",
    renew,
    socket: () => {
      const socket = new Socket();
      sockets.push(socket);
      return socket as unknown as WebSocket;
    },
  });
  instances.push(client);
  client.start();
  sockets[0].onopen?.();
  sockets[0].receive({
    type: "yjs_snapshot",
    actor_id: actorID,
    yjs_state: state,
  });
  return { client, sockets, state, actorID, nodeID };
}
it("uses memory only, sends completed edits on the unique socket and retains original bytes after lost acknowledgement", async () => {
  const writes = vi.spyOn(Storage.prototype, "setItem");
  const reads = vi.spyOn(Storage.prototype, "getItem");
  const open = vi.fn(() => {
    throw new Error("business IndexedDB forbidden");
  });
  vi.stubGlobal("indexedDB", { open });
  const { client, sockets, state, actorID } = setup();
  await vi.waitFor(() => expect(client.getSnapshot().document).not.toBeNull());
  const draft = structuredClone(client.getSnapshot().document!);
  draft.nodes[0].title = "新标题";
  client.change(draft, true);
  await vi.waitFor(() =>
    expect(sockets[0].sent.some((p) => p.type === "yjs_submit")).toBe(true),
  );
  const original = sockets[0].sent.find((p) => p.type === "yjs_submit")!;
  expect(client.getSnapshot().dirty).toBe(true);
  expect(writes).not.toHaveBeenCalled();
  expect(reads).not.toHaveBeenCalled();
  expect(open).not.toHaveBeenCalled();
  sockets[0].close();
  window.dispatchEvent(new Event("online"));
  await vi.waitFor(() => expect(sockets).toHaveLength(2));
  sockets[1].onopen?.();
  sockets[1].receive({
    type: "yjs_snapshot",
    actor_id: actorID,
    yjs_state: state,
  });
  await vi.waitFor(() =>
    expect(sockets[1].sent.some((p) => p.type === "yjs_resolve")).toBe(true),
  );
  expect(sockets[1].sent.find((p) => p.type === "yjs_resolve")!.body).toEqual(
    original.body,
  );
});
it("holds an unfinished gesture and submits only the final atomic position, without periodic polling", async () => {
  const { client, sockets } = setup();
  await vi.waitFor(() => expect(client.getSnapshot().document).not.toBeNull());
  const draft = structuredClone(client.getSnapshot().document!);
  draft.nodes[0].position = { x: 100, y: 200 };
  client.change(draft, false);
  await new Promise((resolve) => setTimeout(resolve, 20));
  expect(sockets[0].sent.filter((p) => p.type === "yjs_submit")).toHaveLength(
    0,
  );
  client.change(draft, true);
  await vi.waitFor(() =>
    expect(sockets[0].sent.filter((p) => p.type === "yjs_submit")).toHaveLength(
      1,
    ),
  );
});
it("revocation drops shared state and refuses late callbacks", async () => {
  const { client, sockets, state, actorID } = setup();
  await vi.waitFor(() => expect(client.getSnapshot().document).not.toBeNull());
  sockets[0].receive({ type: "error", status: 404, code: "not_found" });
  await vi.waitFor(() => expect(client.getSnapshot().document).toBeNull());
  sockets[0].receive({
    type: "yjs_snapshot",
    actor_id: actorID,
    yjs_state: state,
  });
  await new Promise((resolve) => setTimeout(resolve, 10));
  expect(client.getSnapshot().document).toBeNull();
});
it("captures a live composition reader before a remote deletion rebuilds the document", async () => {
  const { client, sockets, state, actorID, nodeID } = setup();
  await vi.waitFor(() => expect(client.getSnapshot().document).not.toBeNull());
  client.change(client.getSnapshot().document!, false);
  const reader = vi.fn(() => ({
    schemaVersion: 1 as const,
    content: {
      type: "doc",
      content: [
        {
          type: "paragraph",
          content: [{ type: "text", text: "尚在组合输入的中文" }],
        },
      ],
    },
  }));
  const unregister = client.textBinding(nodeID)!.registerDraftReader!(reader);
  const remote = new Y.Doc();
  Y.applyUpdate(remote, Buffer.from(state.state, "base64"));
  (remote.getMap("canvas").get("nodes") as Y.Map<unknown>).delete(nodeID);
  sockets[0].receive({
    type: "yjs_snapshot",
    actor_id: actorID,
    yjs_state: {
      ...state,
      commitSeq: "1",
      state: Buffer.from(Y.encodeStateAsUpdate(remote)).toString("base64"),
      document: { ...state.document, revision: "2", nodes: [] },
    },
  });
  await vi.waitFor(() => expect(reader).toHaveBeenCalled());
  await vi.waitFor(() =>
    expect(JSON.stringify(client.getSnapshot().recovery)).toContain(
      "尚在组合输入的中文",
    ),
  );
  const source = client.getSnapshot().recovery[0];
  expect(source.fields[0].copyRequired).toBe(true);
  await expect(
    client.applyRecovery(source.sourceId, source.fields, "1"),
  ).rejects.toThrow("RECOVERY_COPY_REQUIRED");
  expect(client.getSnapshot().recovery).toHaveLength(1);
  await client.applyRecovery(
    source.sourceId,
    source.fields.map((field) => ({ ...field, copyNode: true })),
    "1",
  );
  await vi.waitFor(() =>
    expect(client.getSnapshot().document!.nodes).toHaveLength(1),
  );
  expect(client.getSnapshot().document!.nodes[0].id).not.toBe(nodeID);
  expect(JSON.stringify(client.getSnapshot().document)).toContain(
    "尚在组合输入的中文",
  );
  await vi.waitFor(() =>
    expect(sockets[0].sent.some((packet) => packet.type === "yjs_submit")).toBe(
      true,
    ),
  );
  unregister();
  remote.destroy();
});

it("keeps rejected input private, recovers only reviewed fields under a fresh immutable identity", async () => {
  const { client, sockets, state, actorID } = setup();
  await vi.waitFor(() => expect(client.getSnapshot().document).not.toBeNull());
  const draft = structuredClone(client.getSnapshot().document!);
  draft.nodes[0].title = "被拒的标题";
  client.change(draft, true);
  await vi.waitFor(() =>
    expect(sockets[0].sent.some((p) => p.type === "yjs_submit")).toBe(true),
  );
  const original = sockets[0].sent.find((p) => p.type === "yjs_submit")!;
  sockets[0].receive({
    type: "yjs_ack",
    op_id: original.op_id,
    yjs_receipt: { status: "rejected", code: "GRAPH_CONFLICT" },
  });
  await vi.waitFor(() =>
    expect(sockets[0].sent.some((p) => p.type === "sync")).toBe(true),
  );
  sockets[0].receive({
    type: "yjs_snapshot",
    actor_id: actorID,
    yjs_state: state,
  });
  await vi.waitFor(() => expect(client.getSnapshot().recovery).toHaveLength(1));
  expect(client.getSnapshot().document!.nodes[0].title).toBe("正文");
  expect(client.getSnapshot().dirty).toBe(true);
  const source = client.getSnapshot().recovery[0];
  await expect(
    client.applyRecovery(source.sourceId, source.fields, "9"),
  ).rejects.toThrow("服务器内容已变化");
  await client.applyRecovery(source.sourceId, source.fields, "0");
  await vi.waitFor(() =>
    expect(sockets[0].sent.filter((p) => p.type === "yjs_submit")).toHaveLength(
      2,
    ),
  );
  const recovered = sockets[0].sent.filter((p) => p.type === "yjs_submit")[1];
  expect(recovered.op_id).not.toBe(original.op_id);
  expect(recovered.body).toMatchObject({ reviewSeq: "0" });
  const body = recovered.body as { update: string; payloadHash: string };
  const accepted = new Y.Doc();
  Y.applyUpdate(accepted, Buffer.from(state.state, "base64"));
  Y.applyUpdate(accepted, Buffer.from(body.update, "base64"));
  const document = serviceProjection(
    readCanvasDocument(accepted),
    state.document as unknown as ServiceDocument,
  );
  sockets[0].receive({
    type: "yjs_snapshot",
    actor_id: actorID,
    yjs_state: {
      ...state,
      commitSeq: "1",
      state: Buffer.from(Y.encodeStateAsUpdate(accepted)).toString("base64"),
      document: { ...document, revision: "2" },
    },
  });
  sockets[0].receive({
    type: "yjs_ack",
    op_id: recovered.op_id,
    yjs_receipt: {
      status: "accepted",
      epoch: state.epoch,
      updateId: recovered.op_id,
      payloadHash: body.payloadHash,
      commitSeq: "1",
      contentRevision: "2",
      graphHash: state.graphHash,
      effect: "VISIBLE_CHANGE",
      update: body.update,
    },
  });
  await vi.waitFor(() => expect(client.getSnapshot().dirty).toBe(false));
  expect(client.getSnapshot().recovery).toHaveLength(0);
  accepted.destroy();
});
it("queries an old-epoch original and archives its rejected input instead of replaying bytes into the new epoch", async () => {
  const { client, sockets, state, actorID } = setup();
  await vi.waitFor(() => expect(client.getSnapshot().document).not.toBeNull());
  const draft = structuredClone(client.getSnapshot().document!);
  draft.nodes[0].title = "旧版本私有输入";
  client.change(draft, true);
  await vi.waitFor(() =>
    expect(sockets[0].sent.some((p) => p.type === "yjs_submit")).toBe(true),
  );
  const original = sockets[0].sent.find((p) => p.type === "yjs_submit")!;
  sockets[0].close();
  window.dispatchEvent(new Event("online"));
  await vi.waitFor(() => expect(sockets).toHaveLength(2));
  sockets[1].onopen?.();
  const next = { ...state, epoch: crypto.randomUUID() };
  sockets[1].receive({
    type: "yjs_snapshot",
    actor_id: actorID,
    yjs_state: next,
  });
  await vi.waitFor(() =>
    expect(sockets[1].sent.some((p) => p.type === "yjs_resolve")).toBe(true),
  );
  expect(sockets[1].sent.find((p) => p.type === "yjs_resolve")!.body).toEqual(
    original.body,
  );
  sockets[1].receive({
    type: "yjs_ack",
    op_id: original.op_id,
    yjs_receipt: { status: "rejected", code: "EPOCH_CHANGED" },
  });
  await vi.waitFor(() =>
    expect(JSON.stringify(client.getSnapshot().recovery)).toContain(
      "旧版本私有输入",
    ),
  );
  expect(client.getSnapshot().document!.nodes[0].title).toBe("正文");
  expect(client.getSnapshot().dirty).toBe(true);
  expect(sockets[1].sent.filter((p) => p.type === "yjs_submit")).toHaveLength(
    0,
  );
});

it("does not undo a node creation after another collaborator has edited that node", async () => {
  const { client, sockets, state, actorID } = setup();
  await vi.waitFor(() => expect(client.getSnapshot().document).not.toBeNull());
  const draft = structuredClone(client.getSnapshot().document!),
    nodeID = crypto.randomUUID();
  draft.nodes.push({
    ...structuredClone(draft.nodes[0]),
    id: nodeID,
    title: "我创建的节点",
    documentData: undefined,
  });
  client.change(draft, true);
  await vi.waitFor(() =>
    expect(sockets[0].sent.some((p) => p.type === "yjs_submit")).toBe(true),
  );
  const submitted = sockets[0].sent.find((p) => p.type === "yjs_submit")!,
    body = submitted.body as { update: string };
  const remote = new Y.Doc();
  Y.applyUpdate(remote, Buffer.from(state.state, "base64"));
  Y.applyUpdate(remote, Buffer.from(body.update, "base64"));
  const nodes = remote.getMap("canvas").get("nodes") as Y.Map<Y.Map<unknown>>;
  (nodes.get(nodeID)!.get("data") as Y.Map<unknown>).set(
    "title",
    "协作者的标题",
  );
  const document = serviceProjection(
    readCanvasDocument(remote),
    state.document as unknown as ServiceDocument,
  );
  sockets[0].receive({
    type: "yjs_snapshot",
    actor_id: actorID,
    yjs_state: {
      ...state,
      commitSeq: "2",
      state: Buffer.from(Y.encodeStateAsUpdate(remote)).toString("base64"),
      document: { ...document, revision: "3" },
    },
  });
  await vi.waitFor(() =>
    expect(
      client
        .getSnapshot()
        .document!.nodes.some((n) => n.title === "协作者的标题"),
    ).toBe(true),
  );
  client.history("undo");
  expect(
    client
      .getSnapshot()
      .document!.nodes.some((n) => n.title === "协作者的标题"),
  ).toBe(true);
  expect(client.getSnapshot().message).toContain("已有其他编辑");
  remote.destroy();
});

it("presence never enters autosave and restore retries retain the original immutable control identity", async () => {
  const { client, sockets, state, actorID } = setup();
  await vi.waitFor(() => expect(client.getSnapshot().connected).toBe(true));
  sockets[0].receive({
    type: "yjs_presence",
    presence: [
      {
        actorId: actorID,
        clientId: crypto.randomUUID(),
        cursor: { x: 40, y: 90 },
        updatedAt: new Date().toISOString(),
      },
    ],
  });
  await vi.waitFor(() => expect(client.getPresence()).toHaveLength(1));
  expect(client.getSnapshot().dirty).toBe(false);
  client.setCursor({ x: 120, y: 80 });
  await vi.waitFor(() =>
    expect(sockets[0].sent.some((p) => p.type === "yjs_presence")).toBe(true),
  );
  expect(sockets[0].sent.some((p) => p.type === "yjs_submit")).toBe(false);
  const restored = client.restoreVersion(crypto.randomUUID(), "0");
  const original = sockets[0].sent.find((p) => p.type === "yjs_restore")!;
  expect(client.getSnapshot().dirty).toBe(true);
  expect(client.getSnapshot().writable).toBe(false);
  sockets[0].receive({
    type: "heartbeat",
    permissions: { read: true, write: true },
  });
  await vi.waitFor(() =>
    expect(
      sockets[0].sent.filter((p) => p.type === "yjs_restore"),
    ).toHaveLength(2),
  );
  expect(sockets[0].sent.filter((p) => p.type === "yjs_restore")[1]).toEqual(
    original,
  );
  sockets[0].receive({
    type: "yjs_restore_ack",
    op_id: original.op_id,
    yjs_restore: { epoch: state.epoch, revision: "2" },
  });
  await restored;
  expect(client.getSnapshot().dirty).toBe(false);
});

it.each([false, true])(
  "submits the real add-reference editor path after rich-text promotion=%s",
  async (editText) => {
    const { connectCanvasDocument } =
      await import("@/components/canvas/lib/canvas-document-connections");
    const { client, sockets, nodeID } = setup(true);
    await vi.waitFor(() =>
      expect(client.getSnapshot().document).not.toBeNull(),
    );
    if (editText) {
      const text = client.textBinding(nodeID)!.fragment.get(0) as Y.XmlElement;
      (text.get(0) as Y.XmlText).insert(0, "中文");
      await vi.waitFor(() => expect(client.getSnapshot().dirty).toBe(true));
    }
    const draft = client.getSnapshot().document!;
    const linked = connectCanvasDocument(
      draft,
      draft.nodes[1].id,
      draft.nodes[0].id,
    );
    client.change(linked, true);
    await vi.waitFor(() =>
      expect(sockets[0].sent.some((p) => p.type === "yjs_submit")).toBe(true),
    );
    expect(client.getSnapshot().message).toBeFalsy();
    expect(client.getSnapshot().document!.connections).toHaveLength(1);
  },
);

it("retries transient renewal failures after a socket disconnect without an online event", async () => {
  vi.useFakeTimers();
  try {
    const renew = vi
      .fn()
      .mockRejectedValueOnce(new RequestError("unavailable", 503))
      .mockResolvedValue(undefined);
    const { sockets } = setup(false, renew);
    await vi.advanceTimersByTimeAsync(0);
    sockets[0].close();
    await vi.advanceTimersByTimeAsync(2000);
    expect(renew).toHaveBeenCalledTimes(1);
    await vi.advanceTimersByTimeAsync(4000);
    expect(renew).toHaveBeenCalledTimes(2);
    expect(sockets).toHaveLength(2);
  } finally {
    vi.useRealTimers();
  }
});

it("does not retry renewal when current account authority is denied", async () => {
  vi.useFakeTimers();
  try {
    const renew = vi.fn().mockRejectedValue(new RequestError("forbidden", 403));
    const { sockets } = setup(false, renew);
    await vi.advanceTimersByTimeAsync(0);
    sockets[0].close();
    await vi.advanceTimersByTimeAsync(60000);
    expect(renew).toHaveBeenCalledTimes(1);
    expect(sockets).toHaveLength(1);
  } finally {
    vi.useRealTimers();
  }
});

async function acknowledgeAfterLaterSnapshot(lostOriginal = false) {
  const fixture = setup();
  const { client, sockets, state, actorID, nodeID } = fixture;
  await vi.waitFor(() => expect(client.getSnapshot().document).not.toBeNull());
  const draft = structuredClone(client.getSnapshot().document!);
  draft.nodes[0].title = "本人的已接受标题";
  client.change(draft, true);
  await vi.waitFor(() =>
    expect(sockets[0].sent.some((p) => p.type === "yjs_submit")).toBe(true),
  );
  const request = sockets[0].sent.find((p) => p.type === "yjs_submit")!;
  const frozen = request.body as {
    update: string;
    updateId: string;
    payloadHash: string;
  };
  const server = new Y.Doc();
  Y.applyUpdate(server, Buffer.from(state.state, "base64"));
  Y.applyUpdate(server, Buffer.from(frozen.update, "base64"));
  const node = (
    server.getMap("canvas").get("nodes") as Y.Map<Y.Map<unknown>>
  ).get(nodeID)!;
  if (lostOriginal)
    (node.get("data") as Y.Map<unknown>).set("title", "同时编辑胜出的标题");
  const accepted = {
    ...serviceProjection(
      readCanvasDocument(server),
      state.document as unknown as ServiceDocument,
    ),
    revision: "2",
  } as unknown as API.DocumentDTO;
  (node.get("data") as Y.Map<unknown>).set("title", "后来协作者的标题");
  const latest = {
    type: "yjs_snapshot",
    actor_id: actorID,
    yjs_state: {
      ...state,
      commitSeq: "2",
      state: Buffer.from(Y.encodeStateAsUpdate(server)).toString("base64"),
      document: {
        ...serviceProjection(
          readCanvasDocument(server),
          state.document as unknown as ServiceDocument,
        ),
        revision: "3",
      },
    },
  };
  sockets[0].receive(latest);
  sockets[0].receive({
    type: "yjs_ack",
    op_id: frozen.updateId,
    yjs_receipt: {
      ...frozen,
      status: "accepted",
      effect: "VISIBLE_CHANGE",
      epoch: state.epoch,
      commitSeq: "1",
      contentRevision: "2",
      graphHash: "b".repeat(64),
    },
  });
  server.destroy();
  await vi.waitFor(() =>
    expect(sockets[0].sent.some((p) => p.type === "yjs_history")).toBe(true),
  );
  const history = sockets[0].sent.find((p) => p.type === "yjs_history")!;
  expect(history.body).toEqual({ before: "3" });
  const recover = async () => {
    await vi.waitFor(() =>
      expect(sockets[0].sent.some((p) => p.type === "sync")).toBe(true),
    );
    sockets[0].receive(latest);
    await vi.waitFor(() =>
      expect(client.getSnapshot().recovery).toHaveLength(1),
    );
    expect(sockets[0].sent.filter((p) => p.type === "sync")).toHaveLength(1);
  };
  return { ...fixture, accepted, history, recover };
}

it.each([false, true])(
  "proves the original accepted revision when a later snapshot overtakes its ACK; losing intent=%s",
  async (lostOriginal) => {
    const { client, sockets, state, accepted, history, recover } =
      await acknowledgeAfterLaterSnapshot(lostOriginal);
    const versionID = crypto.randomUUID();
    sockets[0].receive({
      type: "yjs_history",
      op_id: history.op_id,
      yjs_versions: [{ id: versionID, epoch: state.epoch, revision: "2" }],
    });
    await vi.waitFor(() =>
      expect(sockets[0].sent.some((p) => p.type === "yjs_version")).toBe(true),
    );
    const version = sockets[0].sent.find((p) => p.type === "yjs_version")!;
    expect(version.body).toEqual({ id: versionID });
    sockets[0].receive({
      type: "yjs_version",
      op_id: version.op_id,
      document: accepted,
    });
    await vi.waitFor(() =>
      expect(client.getSnapshot().phase).toBe(
        lostOriginal ? "输入已保留，需处理" : "已保存",
      ),
    );
    expect(client.getSnapshot().document!.nodes[0].title).toBe(
      "后来协作者的标题",
    );
    expect(client.getSnapshot().dirty).toBe(lostOriginal);
    if (lostOriginal) {
      await recover();
      expect(client.getSnapshot().recovery[0].fields[0].after.value).toBe(
        "本人的已接受标题",
      );
    } else expect(client.getSnapshot().recovery).toEqual([]);
    expect(sockets[0].sent.filter((p) => p.type === "yjs_submit")).toHaveLength(
      1,
    );
  },
);

it.each(["missing", "epoch", "document", "revision"])(
  "does not substitute the latest state for an unavailable or mismatched original history: %s",
  async (mismatch) => {
    const { client, sockets, state, accepted, history, recover } =
      await acknowledgeAfterLaterSnapshot();
    sockets[0].receive({
      type: "yjs_history",
      op_id: history.op_id,
      yjs_versions:
        mismatch === "missing"
          ? []
          : [
              {
                id: crypto.randomUUID(),
                epoch: mismatch === "epoch" ? crypto.randomUUID() : state.epoch,
                revision: "2",
              },
            ],
    });
    if (mismatch === "document" || mismatch === "revision") {
      await vi.waitFor(() =>
        expect(sockets[0].sent.some((p) => p.type === "yjs_version")).toBe(
          true,
        ),
      );
      const version = sockets[0].sent.find((p) => p.type === "yjs_version")!;
      sockets[0].receive({
        type: "yjs_version",
        op_id: version.op_id,
        document: {
          ...accepted,
          ...(mismatch === "document"
            ? { id: crypto.randomUUID() }
            : { revision: "3" }),
        },
      });
    }
    await vi.waitFor(() =>
      expect(client.getSnapshot().phase).toBe("输入已保留，需处理"),
    );
    expect(client.getSnapshot().dirty).toBe(true);
    await recover();
    expect(client.getSnapshot().recovery[0].fields[0].after.value).toBe(
      "本人的已接受标题",
    );
    expect(sockets[0].sent.filter((p) => p.type === "yjs_submit")).toHaveLength(
      1,
    );
  },
);

it.each([false, true])(
  "allows creation undo after server label allocation but protects foreign links: %s",
  async (foreignLink) => {
    const f = setup();
    await vi.waitFor(() =>
      expect(f.client.getSnapshot().document).not.toBeNull(),
    );
    const { connectCanvasDocumentSources } =
      await import("@/components/canvas/lib/canvas-document-connections");
    const original = f.client.getSnapshot().document!;
    const image = createCanvasNode("image", { x: 400, y: 0 });
    f.client.change(
      connectCanvasDocumentSources(
        { ...original, nodes: [...original.nodes, image] },
        [f.nodeID],
        image.id,
      ),
      true,
    );
    await vi.waitFor(() =>
      expect(f.sockets[0].sent.some((p) => p.type === "yjs_submit")).toBe(true),
    );
    const packet = f.sockets[0].sent.find((p) => p.type === "yjs_submit")!;
    const remote = new Y.Doc();
    Y.applyUpdate(remote, Buffer.from(f.state.state, "base64"));
    Y.applyUpdate(
      remote,
      Buffer.from((packet.body as { update: string }).update, "base64"),
    );
    const root = remote.getMap("canvas");
    const nodes = root.get("nodes") as Y.Map<Y.Map<unknown>>;
    (nodes.get(image.id)!.get("data") as Y.Map<unknown>).set(
      "referenceLabels",
      { [f.nodeID]: { kind: "text", ordinal: 1 } },
    );
    if (foreignLink) {
      const edges = root.get("connections") as Y.Map<Y.Map<unknown>>;
      const id = crypto.randomUUID();
      edges.set(
        id,
        new Y.Map(
          Object.entries({
            data: {
              id,
              fromNodeId: image.id,
              toNodeId: f.nodeID,
              native: {
                id,
                source_node_id: image.id,
                target_node_id: f.nodeID,
                edge_type: "annotation",
              },
            },
            order: 1,
          }),
        ),
      );
    }
    const document = serviceProjection(
      readCanvasDocument(remote),
      f.state.document as unknown as ServiceDocument,
    );
    f.sockets[0].receive({
      type: "yjs_snapshot",
      actor_id: f.actorID,
      yjs_state: {
        ...f.state,
        commitSeq: "1",
        state: Buffer.from(Y.encodeStateAsUpdate(remote)).toString("base64"),
        document: { ...document, revision: "2" },
      },
    });
    await vi.waitFor(() => expect(f.client.commitSeq).toBe("1"));
    f.client.history("undo");
    expect(
      f.client.getSnapshot().document!.nodes.some((n) => n.id === image.id),
    ).toBe(foreignLink);
    if (foreignLink)
      expect(f.client.getSnapshot().message).toContain("已有其他编辑");
    else {
      expect(f.client.getSnapshot().document!.connections).toHaveLength(0);
      f.client.history("redo");
      expect(
        f.client.getSnapshot().document!.nodes.some((n) => n.id === image.id),
      ).toBe(true);
      expect(f.client.getSnapshot().document!.connections).toHaveLength(1);
    }
    remote.destroy();
  },
);
