import * as publishMedia from "@/components/canvas/publish-media";
import {
  openCanvasPendingJournal,
  type CanvasPendingStore,
} from "@/components/canvas/lib/canvas-yjs-pending-journal";
import type { RecoveryRecord } from "@glowloom/canvas-collab/session";
import type { ServiceGraph } from "@glowloom/canvas-collab/service-document";
import { connectCanvasDocumentSources } from "@/components/canvas/lib/canvas-document-connections";
import { serviceDocumentFromEditor } from "@/components/canvas/service-graph";
import { createCanvasNode } from "@/components/canvas/lib/canvas-node-factory";
import {
  createContainer,
  containerDescendantIDs,
} from "@/components/canvas/lib/canvas-containers";
import {
  emptyDirectorConfig,
  editDirector,
  readDirector,
} from "@/components/canvas/director/director-model";
import {
  emptyTimelineConfig,
  editTimeline,
  readTimeline,
} from "@/components/canvas/timeline/timeline-model";
import { afterEach, expect, it, vi } from "vitest";
import { Editor } from "@tiptap/core";
import {
  ComposerSession,
  composerEditorOptions,
} from "@/components/generation/composer-session";
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
import { emptyPromptDraft } from "@/components/generation/prompt-document";
import { webcrypto } from "node:crypto";
import { changeCanvasColorTags } from "@/components/canvas/lib/canvas-color-tags";
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
const composers: ComposerSession[] = [];
afterEach(() => {
  instances.splice(0).forEach((client) => client.stop());
  composers.splice(0).forEach((composer) => composer.editor.destroy());
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});
function setup(
  twoNodes = false,
  renew: (signal: AbortSignal) => Promise<void> = async () => {},
  configure?: (document: API.DocumentDTO) => void,
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
  configure?.(document);
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
    recoveryJournal: false,
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
it("persists colorTags through the real live-session Yjs update and authoritative reopen", async () => {
  const { client, sockets, state, actorID, nodeID } = setup();
  await vi.waitFor(() => expect(client.getSnapshot().document).not.toBeNull());
  const graph = client.getSnapshot().document!;
  const next = {
    ...graph,
    nodes: changeCanvasColorTags(
      changeCanvasColorTags(graph.nodes, new Set([nodeID]), "cyan"),
      new Set([nodeID]),
      "purple",
    ),
  };
  client.change(next, true);
  await vi.waitFor(() =>
    expect(sockets[0].sent.some((p) => p.type === "yjs_submit")).toBe(true),
  );
  const packet = sockets[0].sent.find((p) => p.type === "yjs_submit")!;
  const body = packet.body as { update: string; payloadHash: string };
  const accepted = new Y.Doc();
  Y.applyUpdate(accepted, Buffer.from(state.state, "base64"));
  Y.applyUpdate(accepted, Buffer.from(body.update, "base64"));
  const document = serviceProjection(
    readCanvasDocument(accepted),
    state.document as unknown as ServiceDocument,
  );
  expect(
    (document as unknown as API.DocumentDTO).nodes?.[0]?.config?.editor
      ?.metadata?.colorTags,
  ).toEqual(["cyan", "purple"]);
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
    op_id: packet.op_id,
    yjs_receipt: {
      status: "accepted",
      epoch: state.epoch,
      updateId: packet.op_id,
      payloadHash: body.payloadHash,
      commitSeq: "1",
      contentRevision: "2",
      graphHash: state.graphHash,
      effect: "VISIBLE_CHANGE",
      update: body.update,
    },
  });
  await vi.waitFor(() => expect(client.getSnapshot().dirty).toBe(false));
  expect(client.getSnapshot().document?.nodes[0].metadata?.colorTags).toEqual([
    "cyan",
    "purple",
  ]);
  accepted.destroy();
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

function inputFixture(inputRevision = 1) {
  return setup(false, undefined, (document) => {
    document.nodes![0].config!.draft = {
      composer: emptyPromptDraft() as API.PromptDraft,
    };
    document.nodes![0].input_revision = inputRevision;
  });
}
async function acceptInputUpdate(
  f: ReturnType<typeof setup>,
  alter?: (document: API.DocumentDTO) => void,
) {
  const { client, sockets, state, actorID } = f;
  await vi.waitFor(() =>
    expect(sockets[0].sent.some((p) => p.type === "yjs_submit")).toBe(true),
  );
  const request = sockets[0].sent.find((p) => p.type === "yjs_submit")!;
  const frozen = request.body as { update: string; payloadHash: string };
  const accepted = new Y.Doc();
  Y.applyUpdate(accepted, Buffer.from(state.state, "base64"));
  Y.applyUpdate(accepted, Buffer.from(frozen.update, "base64"));
  const document = {
    ...serviceProjection(
      readCanvasDocument(accepted),
      state.document as unknown as ServiceDocument,
    ),
    revision: "2",
  } as API.DocumentDTO;
  document.nodes![0].input_revision = 2;
  alter?.(document);
  sockets[0].receive({
    type: "yjs_snapshot",
    actor_id: actorID,
    yjs_state: {
      ...state,
      commitSeq: "1",
      state: Buffer.from(Y.encodeStateAsUpdate(accepted)).toString("base64"),
      document,
    },
  });
  sockets[0].receive({
    type: "yjs_ack",
    op_id: request.op_id,
    yjs_receipt: {
      ...frozen,
      status: "accepted",
      epoch: state.epoch,
      updateId: request.op_id,
      commitSeq: "1",
      contentRevision: "2",
      graphHash: state.graphHash,
      effect: "VISIBLE_CHANGE",
    },
  });
  accepted.destroy();
  await vi.waitFor(() => expect(client.getSnapshot().dirty).toBe(false));
}
it("prepareInput never initializes a missing draft or commits unfinished body input", async () => {
  const { client, sockets, nodeID } = setup();
  await vi.waitFor(() => expect(client.getSnapshot().document).not.toBeNull());
  const reader = client.textBinding(nodeID)!.registerDraftReader!(() => ({
    schemaVersion: 1,
    content: {
      type: "doc",
      content: [
        { type: "paragraph", content: [{ type: "text", text: "正在输入" }] },
      ],
    },
  }));
  await expect(client.prepareInput(nodeID)).rejects.toThrow();
  expect(
    client.getSnapshot().document!.nodes[0].serviceData!.config?.draft,
  ).toBeUndefined();
  expect(client.changeInput(nodeID, emptyPromptDraft(), true)).toBe(false);
  expect(sockets[0].sent.filter((p) => p.type === "yjs_submit")).toHaveLength(
    0,
  );
  reader();
});
it("prepareInput waits for the sole save queue and accepts Go zero fields and JSON key reordering", async () => {
  const f = setup();
  await vi.waitFor(() =>
    expect(f.client.getSnapshot().document).not.toBeNull(),
  );
  const input = emptyPromptDraft();
  input.document.content[0].content = [{ type: "text", text: "已确认输入" }];
  f.client.changeInput(f.nodeID, input);
  let done = false;
  const pointer = f.client.prepareInput(f.nodeID).then((v) => {
    done = true;
    return v;
  });
  await vi.waitFor(() =>
    expect(f.sockets[0].sent.some((p) => p.type === "yjs_submit")).toBe(true),
  );
  expect(done).toBe(false);
  await acceptInputUpdate(f, (doc) => {
    const draft = doc.nodes![0].config!.draft!;
    draft.prompt = "";
    draft.negative_prompt = "";
    draft.aspect_ratio = "";
    draft.candidate_count = 0;
    draft.composer = Object.fromEntries(
      Object.entries(draft.composer!).reverse(),
    ) as API.PromptDraft;
  });
  await expect(pointer).resolves.toEqual({
    kind: "canvas_input",
    canvas_id: f.state.documentId,
    node_id: f.nodeID,
    canvas_revision: 2,
    input_revision: 2,
  });
  expect(f.sockets[0].sent.filter((p) => p.type === "yjs_submit")).toHaveLength(
    1,
  );
});
it("prepareInput rejects absent authoritative input revision rather than substituting content revision", async () => {
  const { client, nodeID } = inputFixture(0);
  await vi.waitFor(() => expect(client.getSnapshot().document).not.toBeNull());
  await expect(client.prepareInput(nodeID)).rejects.toThrow(
    "生成输入的保存版本尚未确认",
  );
});
it("prepareInput rejects an input change while its original save is pending", async () => {
  const { client, nodeID } = inputFixture();
  await vi.waitFor(() => expect(client.getSnapshot().document).not.toBeNull());
  const a = emptyPromptDraft();
  a.document.content[0].content = [{ type: "text", text: "A" }];
  client.changeInput(nodeID, a);
  const rejected = expect(client.prepareInput(nodeID)).rejects.toThrow(
    "生成输入已变化",
  );
  const b = structuredClone(a);
  b.document.content[0].content = [{ type: "text", text: "B" }];
  client.changeInput(nodeID, b);
  await rejected;
});
it("prepareInput refuses offline and an active body gesture without modifying the draft", async () => {
  const { client, nodeID, sockets } = inputFixture();
  await vi.waitFor(() => expect(client.getSnapshot().document).not.toBeNull());
  client.change(client.getSnapshot().document!, false);
  await expect(client.prepareInput(nodeID)).rejects.toThrow();
  const changed = emptyPromptDraft();
  changed.document.content[0].content = [
    { type: "text", text: "gesture input" },
  ];
  expect(client.changeInput(nodeID, changed)).toBe(false);
  client.change(client.getSnapshot().document!, true);
  sockets[0].close();
  await expect(client.prepareInput(nodeID)).rejects.toThrow();
});

function createInputComposer(f: ReturnType<typeof setup>) {
  const composer = new ComposerSession(
    new Editor(
      composerEditorOptions({
        contextKey: f.nodeID,
        replicaId: "input-test",
        draft: emptyPromptDraft(),
      }),
    ),
  );
  composers.push(composer);
  return composer;
}
function remoteInput(
  f: ReturnType<typeof setup>,
  text: string,
  deleted = false,
) {
  const remote = new Y.Doc();
  Y.applyUpdate(remote, Buffer.from(f.state.state, "base64"));
  const nodes = remote.getMap("canvas").get("nodes") as Y.Map<Y.Map<unknown>>;
  if (deleted) nodes.delete(f.nodeID);
  else {
    const data = nodes.get(f.nodeID)!.get("data") as Y.Map<unknown>;
    const native = structuredClone(data.get("native")) as {
      config: Record<string, unknown>;
    };
    const input = emptyPromptDraft();
    input.document.content[0].content = [{ type: "text", text }];
    native.config.draft = { composer: input };
    data.set("native", native);
  }
  const document = {
    ...serviceProjection(
      readCanvasDocument(remote),
      f.state.document as unknown as ServiceDocument,
    ),
    revision: "2",
  } as API.DocumentDTO;
  if (!deleted) document.nodes![0].input_revision = 2;
  f.sockets[0].receive({
    type: "yjs_snapshot",
    actor_id: f.actorID,
    yjs_state: {
      ...f.state,
      commitSeq: "1",
      document,
      state: Buffer.from(Y.encodeStateAsUpdate(remote)).toString("base64"),
    },
  });
  remote.destroy();
}
it("an unchanged read-only boot emits no draft and no write", async () => {
  const f = inputFixture();
  await vi.waitFor(() =>
    expect(f.client.getSnapshot().document).not.toBeNull(),
  );
  f.sockets[0].receive({
    type: "heartbeat",
    permissions: { read: true, write: false },
  });
  await vi.waitFor(() => expect(f.client.getSnapshot().writable).toBe(false));
  expect(f.client.changeInput(f.nodeID, emptyPromptDraft())).toBe(true);
  expect(f.client.readInputDraft(f.nodeID)).toBeNull();
  expect(f.sockets[0].sent.filter((p) => p.type === "yjs_submit")).toHaveLength(
    0,
  );
});

it("opening a resource without a saved composer does not create an undo frame or conflict", async () => {
  const f = setup();
  await vi.waitFor(() =>
    expect(f.client.getSnapshot().document).not.toBeNull(),
  );
  const composer = createInputComposer(f);
  const unbind = f.client.bindInputComposer(f.nodeID, composer);
  expect(f.client.changeInput(f.nodeID, emptyPromptDraft())).toBe(true);
  expect(f.client.readInputDraft(f.nodeID)).toBeNull();
  expect(f.client.getSnapshot().canUndo).toBe(false);
  expect(f.sockets[0].sent.filter((p) => p.type === "yjs_submit")).toHaveLength(
    0,
  );
  composer.editor.commands.insertContent("实际用户输入");
  expect(f.client.changeInput(f.nodeID, composer.snapshot().draft)).toBe(true);
  expect(f.client.readInputDraft(f.nodeID)).toBeNull();
  unbind();
});

it("deleting a clean empty resource with its composer open does not retain a phantom draft", async () => {
  const f = setup();
  await vi.waitFor(() =>
    expect(f.client.getSnapshot().document).not.toBeNull(),
  );
  const composer = createInputComposer(f);
  const unbind = f.client.bindInputComposer(f.nodeID, composer);
  remoteInput(f, "", true);
  unbind();
  expect(f.client.readInputDraft(f.nodeID)).toBeNull();
  expect(f.client.getSnapshot().recovery).toHaveLength(0);
});

it("quote preparation can explicitly materialize an empty input", async () => {
  const f = setup();
  await vi.waitFor(() =>
    expect(f.client.getSnapshot().document).not.toBeNull(),
  );
  expect(f.client.changeInput(f.nodeID, emptyPromptDraft(), true)).toBe(true);
  expect(
    f.client.getSnapshot().document?.nodes[0].serviceData?.config?.draft
      ?.composer,
  ).toEqual(emptyPromptDraft());
  await vi.waitFor(() =>
    expect(
      f.sockets[0].sent.filter((p) => p.type === "yjs_submit"),
    ).toHaveLength(1),
  );
});
it("input guards and pending uploads reject generation without creating a write", async () => {
  const f = inputFixture();
  await vi.waitFor(() =>
    expect(f.client.getSnapshot().document).not.toBeNull(),
  );
  const remove = f.client.registerInputGuard(f.nodeID, () => false);
  await expect(f.client.prepareInput(f.nodeID)).rejects.toThrow();
  remove();
  const draft = emptyPromptDraft();
  draft.references.push({
    id: "upload",
    source: {
      kind: "pending_upload",
      clientUploadId: "c",
      uploadAttemptId: "a",
    },
    bindingGeneration: 1,
    mediaKind: "image",
    role: "reference",
    order: 0,
  });
  draft.labelState.allocations.upload = { labelKind: "image", ordinal: 1 };
  draft.labelState.highWater.image = 1;
  f.client.changeInput(f.nodeID, draft);
  await expect(f.client.prepareInput(f.nodeID)).rejects.toThrow(
    "参考素材尚未上传完成",
  );
});
it("stop promptly rejects a save waiter", async () => {
  const f = inputFixture();
  await vi.waitFor(() =>
    expect(f.client.getSnapshot().document).not.toBeNull(),
  );
  const draft = emptyPromptDraft();
  draft.document.content[0].content = [{ type: "text", text: "pending" }];
  f.client.changeInput(f.nodeID, draft);
  const pending = expect(f.client.waitForSaved()).rejects.toThrow();
  f.client.stop();
  await pending;
});
it("a bound clean composer applies remote input in place with an authoritative transaction", async () => {
  const f = inputFixture();
  await vi.waitFor(() =>
    expect(f.client.getSnapshot().document).not.toBeNull(),
  );
  const composer = createInputComposer(f);
  const transactions: boolean[] = [];
  composer.editor.on("transaction", ({ transaction }) =>
    transactions.push(!!transaction.getMeta("composer-authoritative")),
  );
  f.client.bindInputComposer(f.nodeID, composer);
  remoteInput(f, "remote text");
  await vi.waitFor(() => expect(composer.editor.getText()).toBe("remote text"));
  expect(transactions).toContain(true);
  expect(f.client.readInputDraft(f.nodeID)).toBeNull();
  await expect(f.client.prepareInput(f.nodeID)).resolves.toMatchObject({
    canvas_revision: 2,
    input_revision: 2,
  });
});
it.each(["server", "local"] as const)(
  "remote input during composition retains local input until explicit %s resolution",
  async (choice) => {
    const f = inputFixture();
    await vi.waitFor(() =>
      expect(f.client.getSnapshot().document).not.toBeNull(),
    );
    const composer = createInputComposer(f);
    f.client.bindInputComposer(f.nodeID, composer);
    composer.editor.view.dom.dispatchEvent(
      new CompositionEvent("compositionstart", { bubbles: true }),
    );
    composer.editor.view.dispatch(
      composer.editor.state.tr.insertText("本页输入").setMeta("composition", 1),
    );
    remoteInput(f, "远端输入");
    await vi.waitFor(() =>
      expect(JSON.stringify(f.client.getSnapshot().document)).toContain(
        "远端输入",
      ),
    );
    expect(composer.editor.getText()).toBe("本页输入");
    expect(JSON.stringify(f.client.readInputDraft(f.nodeID)?.draft)).toContain(
      "本页输入",
    );
    expect(() => f.client.resolveInputDraft(f.nodeID, choice)).toThrow(
      "请先结束输入法编辑",
    );
    await expect(f.client.prepareInput(f.nodeID)).rejects.toThrow();
    composer.editor.view.dom.dispatchEvent(
      new CompositionEvent("compositionend", { bubbles: true }),
    );
    await vi.waitFor(() => expect(composer.composing).toBe(false));
    expect(f.client.changeInput(f.nodeID, composer.snapshot().draft)).toBe(
      false,
    );
    expect(f.client.resolveInputDraft(f.nodeID, choice)).toBe(true);
    expect(composer.editor.getText()).toBe(
      choice === "server" ? "远端输入" : "本页输入",
    );
    expect(f.client.readInputDraft(f.nodeID)).toBeNull();
    if (choice === "local")
      await vi.waitFor(() =>
        expect(f.sockets[0].sent.some((p) => p.type === "yjs_submit")).toBe(
          true,
        ),
      );
    else
      expect(
        f.sockets[0].sent.filter((p) => p.type === "yjs_submit"),
      ).toHaveLength(0);
  },
);
it.each(["delete", "revoke"])(
  "%s preserves unsynchronized composer input after unbinding",
  async (action) => {
    const f = inputFixture();
    await vi.waitFor(() =>
      expect(f.client.getSnapshot().document).not.toBeNull(),
    );
    const composer = createInputComposer(f);
    const unbind = f.client.bindInputComposer(f.nodeID, composer);
    composer.editor.commands.insertContent("本页未同步");
    if (action === "delete") remoteInput(f, "", true);
    else
      f.sockets[0].receive({ type: "error", status: 403, code: "forbidden" });
    await vi.waitFor(() =>
      expect(
        f.client.getSnapshot().document?.nodes.some((n) => n.id === f.nodeID) ??
          false,
      ).toBe(false),
    );
    unbind();
    expect(JSON.stringify(f.client.readInputDraft(f.nodeID)?.draft)).toContain(
      "本页未同步",
    );
    expect(() => f.client.resolveInputDraft(f.nodeID, "local")).toThrow();
    await expect(f.client.prepareInput(f.nodeID)).rejects.toThrow();
  },
);
it("ACK before DTO waits for the matching authority revision, sharing one save among callers", async () => {
  const f = inputFixture();
  await vi.waitFor(() =>
    expect(f.client.getSnapshot().document).not.toBeNull(),
  );
  const draft = emptyPromptDraft();
  draft.document.content[0].content = [{ type: "text", text: "confirmed" }];
  f.client.changeInput(f.nodeID, draft);
  let resolved = false;
  const first = f.client.prepareInput(f.nodeID).then((value) => {
    resolved = true;
    return value;
  });
  const second = f.client.prepareInput(f.nodeID);
  await vi.waitFor(() =>
    expect(f.sockets[0].sent.some((p) => p.type === "yjs_submit")).toBe(true),
  );
  const request = f.sockets[0].sent.find((p) => p.type === "yjs_submit")!;
  const frozen = request.body as { update: string; payloadHash: string };
  f.sockets[0].receive({
    type: "yjs_ack",
    op_id: request.op_id,
    yjs_receipt: {
      ...frozen,
      status: "accepted",
      epoch: f.state.epoch,
      updateId: request.op_id,
      commitSeq: "1",
      contentRevision: "2",
      graphHash: f.state.graphHash,
      effect: "VISIBLE_CHANGE",
    },
  });
  await vi.waitFor(() =>
    expect(f.sockets[0].sent.some((p) => p.type === "sync")).toBe(true),
  );
  expect(resolved).toBe(false);
  expect(f.client.getSnapshot().dto?.revision).toBe("1");
  await acceptInputUpdate(f);
  await expect(first).resolves.toMatchObject({
    canvas_revision: 2,
    input_revision: 2,
  });
  await expect(second).resolves.toMatchObject({
    canvas_revision: 2,
    input_revision: 2,
  });
  expect(f.sockets[0].sent.filter((p) => p.type === "yjs_submit")).toHaveLength(
    1,
  );
});
it("UNKNOWN acknowledgement and permission loss reject a waiting quote without a duplicate submit", async () => {
  const f = inputFixture();
  await vi.waitFor(() =>
    expect(f.client.getSnapshot().document).not.toBeNull(),
  );
  const draft = emptyPromptDraft();
  draft.document.content[0].content = [{ type: "text", text: "unknown" }];
  f.client.changeInput(f.nodeID, draft);
  const pending = expect(f.client.prepareInput(f.nodeID)).rejects.toThrow();
  await vi.waitFor(() =>
    expect(f.sockets[0].sent.some((p) => p.type === "yjs_submit")).toBe(true),
  );
  f.sockets[0].receive({ type: "error", status: 503, code: "unavailable" });
  await pending;
  expect(f.client.getSnapshot().phase).toBe("保存结果未确认");
  await expect(f.client.prepareInput(f.nodeID)).rejects.toThrow();
  f.sockets[0].receive({
    type: "heartbeat",
    permissions: { read: true, write: false },
  });
  await vi.waitFor(() => expect(f.client.getSnapshot().writable).toBe(false));
  const changed = structuredClone(draft);
  changed.document.content[0].content = [
    { type: "text", text: "after revoke" },
  ];
  expect(f.client.changeInput(f.nodeID, changed)).toBe(false);
  expect(JSON.stringify(f.client.readInputDraft(f.nodeID))).toContain(
    "after revoke",
  );
  expect(f.sockets[0].sent.filter((p) => p.type === "yjs_submit")).toHaveLength(
    1,
  );
});
it("epoch replacement rejects a pending quote and retains IME input in recovery", async () => {
  const f = inputFixture();
  await vi.waitFor(() =>
    expect(f.client.getSnapshot().document).not.toBeNull(),
  );
  const composer = createInputComposer(f);
  f.client.bindInputComposer(f.nodeID, composer);
  composer.editor.view.dom.dispatchEvent(
    new CompositionEvent("compositionstart", { bubbles: true }),
  );
  composer.editor.view.dispatch(
    composer.editor.state.tr
      .insertText("旧版本本页稿")
      .setMeta("composition", 1),
  );
  f.sockets[0].receive({
    type: "yjs_snapshot",
    actor_id: f.actorID,
    yjs_state: {
      ...f.state,
      epoch: crypto.randomUUID(),
      document: { ...f.state.document, revision: "2" },
    },
  });
  await vi.waitFor(() =>
    expect(f.client.getSnapshot().recovery.length).toBeGreaterThan(0),
  );
  expect(JSON.stringify(f.client.getSnapshot().recovery)).toContain(
    "旧版本本页稿",
  );
  await expect(f.client.prepareInput(f.nodeID)).rejects.toThrow();
  expect(composer.editor.getText()).toBe("旧版本本页稿");
  expect(f.sockets[0].sent.filter((p) => p.type === "yjs_submit")).toHaveLength(
    0,
  );
});
it("unmount protects the visible IME DOM before a ProseMirror transaction", async () => {
  const f = inputFixture();
  await vi.waitFor(() =>
    expect(f.client.getSnapshot().document).not.toBeNull(),
  );
  const composer = createInputComposer(f);
  const unbind = f.client.bindInputComposer(f.nodeID, composer);
  composer.editor.view.dom.dispatchEvent(
    new CompositionEvent("compositionstart", { bubbles: true }),
  );
  composer.editor.view.dom.querySelector("p")!.textContent = "正在候选中的文字";
  unbind();
  expect(JSON.stringify(f.client.readInputDraft(f.nodeID)?.draft)).toContain(
    "正在候选中的文字",
  );
  expect(f.sockets[0].sent.filter((p) => p.type === "yjs_submit")).toHaveLength(
    0,
  );
});
it("a changed bound composer cannot quote the older input before onChange runs", async () => {
  const f = inputFixture();
  await vi.waitFor(() =>
    expect(f.client.getSnapshot().document).not.toBeNull(),
  );
  const composer = createInputComposer(f);
  f.client.bindInputComposer(f.nodeID, composer);
  composer.editor.commands.insertContent("尚未交给画布的输入");
  await expect(f.client.prepareInput(f.nodeID)).rejects.toThrow();
  expect(f.client.changeInput(f.nodeID, composer.snapshot().draft)).toBe(true);
  const pointer = f.client.prepareInput(f.nodeID);
  await acceptInputUpdate(f);
  await expect(pointer).resolves.toMatchObject({
    canvas_revision: 2,
    input_revision: 2,
  });
});
it("input changes after a clean wait resolves still invalidate the prepared pointer", async () => {
  const f = inputFixture();
  await vi.waitFor(() =>
    expect(f.client.getSnapshot().document).not.toBeNull(),
  );
  const pending = f.client.prepareInput(f.nodeID);
  const draft = emptyPromptDraft();
  draft.document.content[0].content = [
    { type: "text", text: "changed in this turn" },
  ];
  f.client.changeInput(f.nodeID, draft);
  await expect(pending).rejects.toThrow();
});

function timelineFixture(legacy = false) {
  return setup(false, undefined, (document) => {
    const timeline = editTimeline(emptyTimelineConfig(), {
      type: "add",
      assets: [
        {
          asset: {
            id: crypto.randomUUID(),
            project_id: document.project_id,
            kind: "video",
            duration_ms: 5000,
          },
        },
      ],
    });
    if (legacy) delete (timeline as API.TimelineConfig).imported_assets;
    document.nodes![0].node_type = "timeline";
    document.nodes![0].node_action = "tool";
    document.nodes![0].config = { timeline };
  });
}
it("timeline legacy import normalization does not cause a false CAS conflict", async () => {
  const f = timelineFixture(true);
  await vi.waitFor(() =>
    expect(f.client.getSnapshot().document).not.toBeNull(),
  );
  const before = liveTimeline(f);
  expect(before.imported_assets).toEqual([before.clips[0].asset_id]);
  expect(() =>
    f.client.changeTimeline(
      f.nodeID,
      { ...before, aspect_ratio: "1:1" },
      before,
    ),
  ).not.toThrow();
  expect(liveTimeline(f).aspect_ratio).toBe("1:1");
});
const liveTimeline = (f: ReturnType<typeof setup>) =>
  readTimeline(
    f.client.getSnapshot().document!.nodes[0].serviceData!.config!.timeline,
  );
it("timeline CAS rejects stale edits without overwriting the current shared configuration", async () => {
  const f = timelineFixture();
  await vi.waitFor(() =>
    expect(f.client.getSnapshot().document).not.toBeNull(),
  );
  const before = liveTimeline(f),
    next = { ...before, aspect_ratio: "1:1" };
  f.client.changeTimeline(f.nodeID, next, before);
  expect(() =>
    f.client.changeTimeline(f.nodeID, { ...before, fps: 60 }, before),
  ).toThrow("其他协作者");
  expect(liveTimeline(f).aspect_ratio).toBe("1:1");
  expect(liveTimeline(f).fps).toBe(30);
  expect(f.client.retainedTimeline(f.nodeID)?.fps).toBe(60);
  await expect(f.client.prepareTimeline(f.nodeID)).rejects.toThrow(
    "保留的时间线编辑",
  );
  f.client.discardRetainedTimeline(f.nodeID);
  expect(f.client.retainedTimeline(f.nodeID)).toBeNull();
});

it("keeps timeline history independent per node and from canvas history", async () => {
  const f = setup(true, undefined, (document) => {
    for (const node of document.nodes!) {
      node.node_type = "timeline";
      node.node_action = "tool";
      node.config = { timeline: emptyTimelineConfig() };
    }
  });
  await vi.waitFor(() =>
    expect(f.client.getSnapshot().document).not.toBeNull(),
  );
  const nodes = f.client.getSnapshot().document!.nodes;
  const a = nodes[0].id,
    b = nodes[1].id;
  const read = (id: string) =>
    readTimeline(
      f.client.getSnapshot().document!.nodes.find((n) => n.id === id)!
        .serviceData!.config!.timeline,
    );
  const initial = read(a),
    initialB = read(b);
  f.client.changeTimeline(a, { ...initial, aspect_ratio: "1:1" }, initial);
  f.client.changeTimeline(b, { ...read(b), aspect_ratio: "9:16" }, read(b));
  expect(f.client.getSnapshot().canUndo).toBe(false);
  expect(f.client.timelineHistoryState(a)).toEqual({
    canUndo: true,
    canRedo: false,
  });
  f.client.timelineHistory(a, "undo");
  expect(read(a)).toEqual(initial);
  expect(read(b).aspect_ratio).toBe("9:16");
  expect(f.client.timelineHistoryState(a)).toEqual({
    canUndo: false,
    canRedo: true,
  });
  f.client.timelineHistory(a, "redo");
  expect(read(a).aspect_ratio).toBe("1:1");
  f.client.timelineHistory(b, "undo");
  expect(read(b)).toEqual(initialB);
  f.client.timelineHistory(a, "undo");
  f.client.changeTimeline(a, { ...initial, fps: 60 }, initial);
  expect(f.client.timelineHistoryState(a).canRedo).toBe(false);
  expect(f.client.timelineHistoryState(b).canRedo).toBe(true);
  f.client.stop();
  expect(f.client.timelineHistoryState(a)).toEqual({
    canUndo: false,
    canRedo: false,
  });
});

it("records each completed timeline gesture separately even within the 500 ms canvas capture window", async () => {
  const f = timelineFixture();
  await vi.waitFor(() =>
    expect(f.client.getSnapshot().document).not.toBeNull(),
  );
  const before = liveTimeline(f);
  const first = editTimeline(before, {
    type: "trim-left",
    id: before.clips[0].id,
    at: 1000,
  });
  f.client.changeTimeline(f.nodeID, first, before);
  const second = editTimeline(first, {
    type: "split",
    id: first.clips[0].id,
    at: 1000,
  });
  f.client.changeTimeline(f.nodeID, second, first);
  f.client.timelineHistory(f.nodeID, "undo");
  expect(liveTimeline(f)).toEqual(first);
  f.client.timelineHistory(f.nodeID, "undo");
  expect(liveTimeline(f)).toEqual(before);
});

it("timeline removal and its local undo restore the clip and native source relation together", async () => {
  const f = setup(true, undefined, (document) => {
    const [target, source] = document.nodes!;
    const assetID = crypto.randomUUID();
    source.node_type = "video";
    source.ref_type = "media_asset";
    source.ref_id = assetID;
    source.config = {};
    target.node_type = "timeline";
    target.node_action = "tool";
    target.config = {
      timeline: editTimeline(emptyTimelineConfig(), {
        type: "add",
        assets: [
          {
            nodeID: source.id,
            asset: {
              id: assetID,
              project_id: document.project_id,
              kind: "video",
              duration_ms: 5000,
            },
          },
        ],
      }),
    };
    document.edges = [
      {
        id: crypto.randomUUID(),
        edge_type: "annotation",
        source_node_id: source.id,
        target_node_id: target.id,
      },
    ];
  });
  await vi.waitFor(() =>
    expect(f.client.getSnapshot().document).not.toBeNull(),
  );
  const before = liveTimeline(f);
  const edgeID = f.client.getSnapshot().document!.connections[0].id;
  f.client.changeTimeline(
    f.nodeID,
    editTimeline(before, { type: "remove", id: before.clips[0].id }),
    before,
  );
  expect(liveTimeline(f).clips).toEqual([]);
  expect(f.client.getSnapshot().document!.connections).toEqual([]);
  f.client.timelineHistory(f.nodeID, "undo");
  expect(liveTimeline(f)).toEqual(before);
  expect(
    f.client.getSnapshot().document!.connections.map((edge) => edge.id),
  ).toEqual([edgeID]);
  f.client.timelineHistory(f.nodeID, "redo");
  expect(liveTimeline(f).clips).toEqual([]);
  expect(f.client.getSnapshot().document!.connections).toEqual([]);
});

it("refuses timeline writes after permission loss while preserving the unaccepted intention", async () => {
  const f = timelineFixture();
  await vi.waitFor(() =>
    expect(f.client.getSnapshot().document).not.toBeNull(),
  );
  const before = liveTimeline(f);
  f.sockets[0].receive({
    type: "heartbeat",
    permissions: { read: true, write: false },
  });
  await vi.waitFor(() => expect(f.client.getSnapshot().writable).toBe(false));
  const next = { ...before, aspect_ratio: "1:1" };
  expect(() => f.client.changeTimeline(f.nodeID, next, before)).toThrow(
    "不可编辑",
  );
  expect(liveTimeline(f)).toEqual(before);
  expect(f.client.retainedTimeline(f.nodeID)).toEqual(next);
  expect(f.sockets[0].sent.some((packet) => packet.type === "yjs_submit")).toBe(
    false,
  );
});

it("timeline undo never overwrites a later collaborator's accepted timeline configuration", async () => {
  const f = timelineFixture();
  await vi.waitFor(() =>
    expect(f.client.getSnapshot().document).not.toBeNull(),
  );
  const before = liveTimeline(f);
  f.client.changeTimeline(f.nodeID, { ...before, aspect_ratio: "1:1" }, before);
  await vi.waitFor(() =>
    expect(
      f.sockets[0].sent.some((packet) => packet.type === "yjs_submit"),
    ).toBe(true),
  );
  const submitted = f.sockets[0].sent.find(
    (packet) => packet.type === "yjs_submit",
  )!.body as { update: string };
  const remote = new Y.Doc();
  Y.applyUpdate(remote, Buffer.from(f.state.state, "base64"));
  Y.applyUpdate(remote, Buffer.from(submitted.update, "base64"));
  const nodes = remote.getMap("canvas").get("nodes") as Y.Map<Y.Map<unknown>>;
  const data = nodes.get(f.nodeID)!.get("data") as Y.Map<unknown>;
  const native = structuredClone(data.get("native")) as {
    config: { timeline: API.TimelineConfig };
  };
  native.config.timeline.aspect_ratio = "9:16";
  data.set("native", native);
  f.sockets[0].receive({
    type: "yjs_snapshot",
    actor_id: f.actorID,
    yjs_state: {
      ...f.state,
      commitSeq: "2",
      state: Buffer.from(Y.encodeStateAsUpdate(remote)).toString("base64"),
      document: {
        ...serviceProjection(
          readCanvasDocument(remote),
          f.state.document as unknown as ServiceDocument,
        ),
        revision: "3",
      },
    },
  });
  await vi.waitFor(() => expect(liveTimeline(f).aspect_ratio).toBe("9:16"));
  f.client.timelineHistory(f.nodeID, "undo");
  expect(liveTimeline(f).aspect_ratio).toBe("9:16");
  remote.destroy();
});
it("timeline freeze waits for accepted authority and normalizes Go null optional fields", async () => {
  const f = timelineFixture();
  await vi.waitFor(() =>
    expect(f.client.getSnapshot().document).not.toBeNull(),
  );
  const before = liveTimeline(f);
  f.client.changeTimeline(
    f.nodeID,
    { ...before, aspect_ratio: "9:16" },
    before,
  );
  let finished = false;
  const pointer = f.client.prepareTimeline(f.nodeID).then((value) => {
    finished = true;
    return value;
  });
  await vi.waitFor(() =>
    expect(f.sockets[0].sent.some((p) => p.type === "yjs_submit")).toBe(true),
  );
  expect(finished).toBe(false);
  await acceptInputUpdate(f, (doc) => {
    const clip = doc.nodes![0].config!.timeline!.clips![0];
    Object.assign(clip, { node_id: null, crop: null });
  });
  await expect(pointer).resolves.toEqual({
    canvas_id: f.state.documentId,
    node_id: f.nodeID,
    revision: 2,
  });
  expect(f.sockets[0].sent.filter((p) => p.type === "yjs_submit")).toHaveLength(
    1,
  );
});
it("timeline changes during save invalidate the frozen export intention", async () => {
  const f = timelineFixture();
  await vi.waitFor(() =>
    expect(f.client.getSnapshot().document).not.toBeNull(),
  );
  const before = liveTimeline(f),
    next = { ...before, aspect_ratio: "1:1" };
  f.client.changeTimeline(f.nodeID, next, before);
  const rejected = expect(f.client.prepareTimeline(f.nodeID)).rejects.toThrow(
    "时间线已变化",
  );
  f.client.changeTimeline(f.nodeID, { ...next, fps: 60 }, next);
  await rejected;
});
it.each(["unknown", "epoch", "permission"])(
  "timeline freeze rejects %s during a pending save",
  async (reason) => {
    const f = timelineFixture();
    await vi.waitFor(() =>
      expect(f.client.getSnapshot().document).not.toBeNull(),
    );
    const before = liveTimeline(f);
    f.client.changeTimeline(
      f.nodeID,
      { ...before, aspect_ratio: "1:1" },
      before,
    );
    const rejected = expect(
      f.client.prepareTimeline(f.nodeID),
    ).rejects.toThrow();
    await vi.waitFor(() =>
      expect(f.sockets[0].sent.some((p) => p.type === "yjs_submit")).toBe(true),
    );
    if (reason === "unknown")
      f.sockets[0].receive({ type: "error", status: 503, code: "unavailable" });
    else if (reason === "permission")
      f.sockets[0].receive({
        type: "heartbeat",
        permissions: { read: true, write: false },
      });
    else
      f.sockets[0].receive({
        type: "yjs_snapshot",
        actor_id: f.actorID,
        yjs_state: {
          ...f.state,
          epoch: crypto.randomUUID(),
          document: { ...f.state.document, revision: "2" },
        },
      });
    await rejected;
    await expect(f.client.prepareTimeline(f.nodeID)).rejects.toThrow();
    expect(
      f.sockets[0].sent.filter((p) => p.type === "yjs_submit"),
    ).toHaveLength(1);
  },
);
it("timeline freeze refuses empty timelines and absent timeline nodes", async () => {
  const f = timelineFixture();
  await vi.waitFor(() =>
    expect(f.client.getSnapshot().document).not.toBeNull(),
  );
  const before = liveTimeline(f);
  f.client.changeTimeline(f.nodeID, { ...before, clips: [] }, before);
  await expect(f.client.prepareTimeline(f.nodeID)).rejects.toThrow(
    "请先添加素材",
  );
  await expect(f.client.prepareTimeline(crypto.randomUUID())).rejects.toThrow();
});

it("document epoch identity changes on same-ID restoration and is unavailable after stop", async () => {
  const f = timelineFixture();
  await vi.waitFor(() =>
    expect(f.client.getDocumentEpoch()).toBe(f.state.epoch),
  );
  const captured = f.client.getDocumentEpoch(),
    replacement = crypto.randomUUID();
  f.sockets[0].receive({
    type: "yjs_snapshot",
    actor_id: f.actorID,
    yjs_state: {
      ...f.state,
      epoch: replacement,
      document: { ...f.state.document, revision: "2" },
    },
  });
  await vi.waitFor(() => expect(f.client.getDocumentEpoch()).toBe(replacement));
  expect(f.client.getDocumentEpoch()).not.toBe(captured);
  f.client.stop();
  expect(f.client.getDocumentEpoch()).toBeNull();
});

function directorFixture() {
  return setup(false, undefined, (document) => {
    document.nodes![0].node_type = "director";
    document.nodes![0].node_action = "tool";
    document.nodes![0].config = { director: emptyDirectorConfig() };
  });
}
const liveDirector = (f: ReturnType<typeof setup>) =>
  readDirector(
    f.client.getSnapshot().document!.nodes[0].serviceData!.config!.director,
  );
it("director edits use an atomic scene CAS and retain a rejected concurrent intention", async () => {
  const f = directorFixture();
  await vi.waitFor(() =>
    expect(f.client.getSnapshot().document).not.toBeNull(),
  );
  const before = liveDirector(f),
    next = editDirector(before, { type: "add-object", primitive: "box" });
  f.client.changeDirector(f.nodeID, next, before, f.client.getDocumentEpoch());
  expect(() =>
    f.client.changeDirector(
      f.nodeID,
      { ...before, background: "#ffffff" },
      before,
      f.client.getDocumentEpoch(),
    ),
  ).toThrow("其他协作者");
  expect(liveDirector(f).objects).toHaveLength(1);
  expect(f.client.retainedDirector(f.nodeID)?.background).toBe("#ffffff");
  f.client.discardRetainedDirector(f.nodeID);
  expect(f.client.retainedDirector(f.nodeID)).toBeNull();
});
it("director scene save is accepted on the original queue and survives authoritative readback", async () => {
  const f = directorFixture();
  await vi.waitFor(() =>
    expect(f.client.getSnapshot().document).not.toBeNull(),
  );
  const before = liveDirector(f),
    next = editDirector(before, { type: "add-light", light: "point" });
  f.client.changeDirector(f.nodeID, next, before, f.client.getDocumentEpoch());
  const saved = f.client.waitForSaved();
  await acceptInputUpdate(f);
  await expect(saved).resolves.toMatchObject({ revision: "2" });
  expect(liveDirector(f).lights[0].id).toBe(next.lights[0].id);
  expect(f.sockets[0].sent.filter((p) => p.type === "yjs_submit")).toHaveLength(
    1,
  );
});
it("director gesture from an old epoch cannot write identical restored scene content", async () => {
  const f = directorFixture();
  await vi.waitFor(() =>
    expect(f.client.getSnapshot().document).not.toBeNull(),
  );
  const before = liveDirector(f),
    epoch = f.client.getDocumentEpoch(),
    replacement = crypto.randomUUID();
  f.sockets[0].receive({
    type: "yjs_snapshot",
    actor_id: f.actorID,
    yjs_state: {
      ...f.state,
      epoch: replacement,
      document: { ...f.state.document, revision: "2" },
    },
  });
  await vi.waitFor(() => expect(f.client.getDocumentEpoch()).toBe(replacement));
  expect(() =>
    f.client.changeDirector(
      f.nodeID,
      { ...before, background: "#ffffff" },
      before,
      epoch,
    ),
  ).toThrow("其他版本");
  expect(liveDirector(f).background).toBe(before.background);
  expect(f.sockets[0].sent.filter((p) => p.type === "yjs_submit")).toHaveLength(
    0,
  );
});
it("director permission loss refuses transform writes without submitting", async () => {
  const f = directorFixture();
  await vi.waitFor(() =>
    expect(f.client.getSnapshot().document).not.toBeNull(),
  );
  const before = liveDirector(f);
  f.sockets[0].receive({
    type: "heartbeat",
    permissions: { read: true, write: false },
  });
  await vi.waitFor(() => expect(f.client.getSnapshot().writable).toBe(false));
  expect(() =>
    f.client.changeDirector(
      f.nodeID,
      { ...before, mode: "dynamic" },
      before,
      f.client.getDocumentEpoch(),
    ),
  ).toThrow();
  expect(f.sockets[0].sent.filter((p) => p.type === "yjs_submit")).toHaveLength(
    0,
  );
});

it("preserves a typed stack, child geometry and links through delete undo redo", async () => {
  const { client } = setup(
    true,
    async () => {},
    (doc) => {
      doc.edges = [
        {
          id: crypto.randomUUID(),
          edge_type: "annotation",
          source_node_id: doc.nodes![0].id,
          target_node_id: doc.nodes![1].id,
        },
      ];
    },
  );
  await vi.waitFor(() => expect(client.getSnapshot().document).not.toBeNull());
  const graph = structuredClone(client.getSnapshot().document!);
  const original = structuredClone(graph.nodes);
  const stack = createCanvasNode("group", { x: 200, y: 200 });
  const grouped = {
    ...graph,
    nodes: createContainer(
      graph.nodes,
      new Set(graph.nodes.map((n) => n.id)),
      stack,
      "stack",
    ),
  };
  client.change(grouped, true);
  expect(
    client.getSnapshot().document!.nodes.find((n) => n.id === stack.id)
      ?.serviceData?.config?.container?.kind,
  ).toBe("stack");
  client.history("undo");
  expect(
    client
      .getSnapshot()
      .document!.nodes.map((n) => n.id)
      .sort(),
  ).toEqual(original.map((n) => n.id).sort());
  client.history("redo");
  const restored = client.getSnapshot().document!;
  expect(containerDescendantIDs(restored.nodes, stack.id).size).toBe(2);
  const ids = new Set([
    stack.id,
    ...containerDescendantIDs(restored.nodes, stack.id),
  ]);
  client.change(
    {
      ...restored,
      nodes: restored.nodes.filter((n) => !ids.has(n.id)),
      connections: restored.connections.filter(
        (e) => !ids.has(e.fromNodeId) && !ids.has(e.toNodeId),
      ),
    },
    true,
  );
  expect(client.getSnapshot().document!.nodes).toHaveLength(0);
  client.history("undo");
  const recovered = client.getSnapshot().document!;
  expect(recovered.nodes).toHaveLength(3);
  for (const node of original)
    expect(recovered.nodes.find((n) => n.id === node.id)?.position).toEqual(
      node.position,
    );
  expect(recovered.connections).toHaveLength(1);
  expect(recovered.connections).toEqual(restored.connections);
});

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

it("undoes audio clip deletion and automatic lane cleanup as one timeline history item", async () => {
  const f = timelineFixture();
  await vi.waitFor(() =>
    expect(f.client.getSnapshot().document).not.toBeNull(),
  );
  const before = liveTimeline(f);
  const added = editTimeline(before, {
    type: "add",
    assets: [
      { asset: { id: crypto.randomUUID(), kind: "audio", duration_ms: 7000 } },
    ],
  });
  f.client.changeTimeline(f.nodeID, added, before);
  const clip = added.clips.find((item) => item.kind === "audio")!;
  const removed = editTimeline(added, { type: "remove", id: clip.id });
  f.client.changeTimeline(f.nodeID, removed, added);
  expect(liveTimeline(f).tracks).toHaveLength(1);
  f.client.timelineHistory(f.nodeID, "undo");
  expect(liveTimeline(f)).toEqual(added);
  f.client.timelineHistory(f.nodeID, "redo");
  expect(liveTimeline(f)).toEqual(removed);
});

async function connectionFixture() {
  const f = setup(true);
  await vi.waitFor(() =>
    expect(f.client.getSnapshot().document).not.toBeNull(),
  );
  const graph = f.client.getSnapshot().document!;
  const target = createCanvasNode("image", { x: 800, y: 200 });
  const candidate = connectCanvasDocumentSources(
    { ...graph, nodes: [...graph.nodes, target] },
    graph.nodes.map((node) => node.id),
    target.id,
  );
  return { ...f, candidate, target };
}

it("connection admission refuses mixed pending upload without publishing a partial ready graph", async () => {
  const f = await connectionFixture();
  const upload = createCanvasNode(
    "image",
    { x: 0, y: 500 },
    { content: "blob:unfinished" },
  );
  // A local replacement may retain the old file pin while its new upload is pending.
  upload.documentData = {
    id: upload.id,
    type: "image",
    nodeTypeVersion: 1,
    title: upload.title,
    position: upload.position,
    width: upload.width,
    height: upload.height,
    zIndex: 1,
    fileReferences: [
      {
        purpose: "CONTENT",
        referenceKey: "old",
        fileId: crypto.randomUUID(),
        version: "1",
      },
    ],
  };
  const mixed = connectCanvasDocumentSources(
    { ...f.candidate, nodes: [...f.candidate.nodes, upload] },
    [upload.id],
    f.target.id,
  );
  expect(f.client.canAccept(mixed)).toBe(true);
  const before = structuredClone(f.client.getSnapshot().document);
  if (f.client.canAcceptConnectionGraph(mixed)) f.client.change(mixed, true);
  expect(f.client.getSnapshot().document).toEqual(before);
  expect(f.client.canAcceptConnectionGraph(mixed)).toBe(false);
  expect(f.sockets[0].sent.filter((p) => p.type === "yjs_submit")).toHaveLength(
    0,
  );
});

it("connection admission refuses pending composer uploads and full projection failures", async () => {
  const f = await connectionFixture();
  const pending = structuredClone(f.candidate);
  const composer = emptyPromptDraft();
  composer.references.push({
    id: "upload",
    source: {
      kind: "pending_upload",
      clientUploadId: "c",
      uploadAttemptId: "a",
    },
    bindingGeneration: 1,
    mediaKind: "image",
    role: "reference",
    order: 0,
  });
  composer.labelState.allocations.upload = { labelKind: "image", ordinal: 1 };
  composer.labelState.highWater.image = 1;
  pending.nodes[0].serviceData = {
    ...pending.nodes[0].serviceData,
    config: {
      ...pending.nodes[0].serviceData?.config,
      draft: {
        composer: {
          ...composer,
          mode: composer.mode ?? undefined,
          modelVersion: composer.modelVersion ?? undefined,
        },
      },
    },
  };
  expect(f.client.canAccept(pending)).toBe(true);
  expect(f.client.canAcceptConnectionGraph(pending)).toBe(false);
  const invalid = structuredClone(f.candidate);
  invalid.nodes[0].serviceData = {
    ...invalid.nodes[0].serviceData,
    config: { ...invalid.nodes[0].serviceData?.config, draft: {} },
  };
  expect(() => serviceDocumentFromEditor(f.state.document, invalid)).toThrow(
    "生成输入不能被移除",
  );
  expect(f.client.canAccept(invalid)).toBe(true);
  expect(f.client.canAcceptConnectionGraph(invalid)).toBe(false);
  expect(f.sockets[0].sent.filter((p) => p.type === "yjs_submit")).toHaveLength(
    0,
  );
});

it("connection admission preserves offline editing of durable sources", async () => {
  const f = await connectionFixture();
  f.sockets[0].close();
  expect(f.client.getSnapshot().connected).toBe(false);
  expect(f.client.canAcceptConnectionGraph(f.candidate)).toBe(true);
  f.client.change(f.candidate, true);
  expect(f.client.getSnapshot().document?.connections).toHaveLength(2);
  expect(f.client.getSnapshot().dirty).toBe(true);
});

it("connection batch submits once and both edges survive accepted snapshot readback", async () => {
  const f = await connectionFixture();
  expect(f.client.canAcceptConnectionGraph(f.candidate)).toBe(true);
  f.client.change(f.candidate, true);
  await vi.waitFor(() =>
    expect(
      f.sockets[0].sent.filter((p) => p.type === "yjs_submit"),
    ).toHaveLength(1),
  );
  const packet = f.sockets[0].sent.find((p) => p.type === "yjs_submit")!;
  const body = packet.body as { update: string; payloadHash: string };
  const server = new Y.Doc();
  Y.applyUpdate(server, Buffer.from(f.state.state, "base64"));
  Y.applyUpdate(server, Buffer.from(body.update, "base64"));
  const document = serviceProjection(
    readCanvasDocument(server),
    f.state.document as unknown as ServiceDocument,
  );
  expect(document.nodes).toHaveLength(3);
  expect(document.edges).toHaveLength(2);
  f.sockets[0].receive({
    type: "yjs_snapshot",
    actor_id: f.actorID,
    yjs_state: {
      ...f.state,
      commitSeq: "1",
      state: Buffer.from(Y.encodeStateAsUpdate(server)).toString("base64"),
      document: { ...document, revision: "2" },
    },
  });
  f.sockets[0].receive({
    type: "yjs_ack",
    op_id: packet.op_id,
    yjs_receipt: {
      status: "accepted",
      epoch: f.state.epoch,
      updateId: packet.op_id,
      payloadHash: body.payloadHash,
      commitSeq: "1",
      contentRevision: "2",
      graphHash: f.state.graphHash,
      effect: "VISIBLE_CHANGE",
      update: body.update,
    },
  });
  await vi.waitFor(() => expect(f.client.getSnapshot().dirty).toBe(false));
  expect(
    f.client.canAcceptConnectionGraph(f.client.getSnapshot().document!),
  ).toBe(true);
  expect(
    f.client
      .getSnapshot()
      .document?.connections.map((e) => e.id)
      .sort(),
  ).toEqual(f.candidate.connections.map((e) => e.id).sort());
  server.destroy();
});

it("connection batch rejection restores all nodes and edges and retains its intention", async () => {
  const f = await connectionFixture();
  expect(f.client.canAcceptConnectionGraph(f.candidate)).toBe(true);
  f.client.change(f.candidate, true);
  await vi.waitFor(() =>
    expect(f.sockets[0].sent.some((p) => p.type === "yjs_submit")).toBe(true),
  );
  const original = f.sockets[0].sent.find((p) => p.type === "yjs_submit")!;
  f.sockets[0].receive({
    type: "yjs_ack",
    op_id: original.op_id,
    yjs_receipt: { status: "rejected", code: "GRAPH_CONFLICT" },
  });
  await vi.waitFor(() =>
    expect(f.sockets[0].sent.some((p) => p.type === "sync")).toBe(true),
  );
  f.sockets[0].receive({
    type: "yjs_snapshot",
    actor_id: f.actorID,
    yjs_state: f.state,
  });
  await vi.waitFor(() =>
    expect(f.client.getSnapshot().recovery).toHaveLength(1),
  );
  expect(f.client.getSnapshot().document?.nodes).toHaveLength(2);
  expect(f.client.getSnapshot().document?.connections).toHaveLength(0);
  expect(f.sockets[0].sent.filter((p) => p.type === "yjs_submit")).toHaveLength(
    1,
  );
});

it("connection batch lost acknowledgement resolves original id and bytes without duplicate submit", async () => {
  const f = await connectionFixture();
  expect(f.client.canAcceptConnectionGraph(f.candidate)).toBe(true);
  f.client.change(f.candidate, true);
  await vi.waitFor(() =>
    expect(f.sockets[0].sent.some((p) => p.type === "yjs_submit")).toBe(true),
  );
  const original = f.sockets[0].sent.find((p) => p.type === "yjs_submit")!;
  f.sockets[0].close();
  window.dispatchEvent(new Event("online"));
  await vi.waitFor(() => expect(f.sockets).toHaveLength(2));
  f.sockets[1].onopen?.();
  f.sockets[1].receive({
    type: "yjs_snapshot",
    actor_id: f.actorID,
    yjs_state: f.state,
  });
  await vi.waitFor(() =>
    expect(f.sockets[1].sent.some((p) => p.type === "yjs_resolve")).toBe(true),
  );
  const resolved = f.sockets[1].sent.find((p) => p.type === "yjs_resolve")!;
  expect(resolved.op_id).toBe(original.op_id);
  expect(resolved.body).toEqual(original.body);
  expect(
    f.sockets.flatMap((s) => s.sent).filter((p) => p.type === "yjs_submit"),
  ).toHaveLength(1);
});

it("UNKNOWN blocks a new connection intention while preserving ordinary admission and the original pending identity", async () => {
  const f = await connectionFixture();
  f.client.change(f.candidate, true);
  await vi.waitFor(() =>
    expect(
      f.sockets[0].sent.some((packet) => packet.type === "yjs_submit"),
    ).toBe(true),
  );
  const original = f.sockets[0].sent.find(
    (packet) => packet.type === "yjs_submit",
  )!;
  f.sockets[0].receive({ type: "error", status: 503, code: "unavailable" });
  await vi.waitFor(() =>
    expect(f.client.getSnapshot().phase).toBe("保存结果未确认"),
  );
  const current = f.client.getSnapshot().document!;
  const target = createCanvasNode("image", { x: 1000, y: 300 });
  const retry = connectCanvasDocumentSources(
    { ...current, nodes: [...current.nodes, target] },
    [current.nodes[0].id],
    target.id,
  );
  expect(f.client.canAccept(retry)).toBe(true);
  expect(f.client.canAcceptConnectionGraph(retry)).toBe(false);
  if (f.client.canAcceptConnectionGraph(retry)) f.client.change(retry, true);
  expect(
    f.client
      .getSnapshot()
      .document!.nodes.some((node) => node.id === target.id),
  ).toBe(false);
  f.sockets[0].close();
  window.dispatchEvent(new Event("online"));
  await vi.waitFor(() => expect(f.sockets).toHaveLength(2));
  f.sockets[1].onopen?.();
  f.sockets[1].receive({
    type: "yjs_snapshot",
    actor_id: f.actorID,
    yjs_state: f.state,
  });
  await vi.waitFor(() =>
    expect(
      f.sockets[1].sent.some((packet) => packet.type === "yjs_resolve"),
    ).toBe(true),
  );
  const resolved = f.sockets[1].sent.find(
    (packet) => packet.type === "yjs_resolve",
  )!;
  expect(resolved.op_id).toBe(original.op_id);
  expect(resolved.body).toEqual(original.body);
  expect(
    f.sockets
      .flatMap((socket) => socket.sent)
      .filter((packet) => packet.type === "yjs_submit"),
  ).toHaveLength(1);
});

it("a 15-second save confirmation timeout keeps in-flight connection admission closed without changing ordinary offline admission", async () => {
  const f = await connectionFixture();
  f.client.change(f.candidate, true);
  await vi.waitFor(() =>
    expect(
      f.sockets[0].sent.some((packet) => packet.type === "yjs_submit"),
    ).toBe(true),
  );
  const original = f.sockets[0].sent.find(
    (packet) => packet.type === "yjs_submit",
  )!;
  vi.useFakeTimers();
  try {
    const waiter = expect(f.client.waitForSaved()).rejects.toThrow(
      "保存尚未确认",
    );
    await vi.advanceTimersByTimeAsync(15000);
    await waiter;
    expect(f.client.getSnapshot().phase).toBe("等待保存确认");
    const current = f.client.getSnapshot().document!;
    expect(f.client.canAccept(current)).toBe(true);
    expect(f.client.canAcceptConnectionGraph(current)).toBe(false);
    expect(
      f.sockets[0].sent.filter((packet) => packet.type === "yjs_submit"),
    ).toEqual([original]);
    expect(current.connections.map((edge) => edge.id)).toEqual(
      f.candidate.connections.map((edge) => edge.id),
    );
  } finally {
    vi.useRealTimers();
  }
});

it("ordinary upload edits can publish a ready subset, so connection admission rejects that entire candidate", async () => {
  const f = await connectionFixture();
  f.sockets[0].close();
  const upload = createCanvasNode(
    "image",
    { x: 0, y: 500 },
    { content: "blob:unfinished" },
  );
  upload.documentData = {
    id: upload.id,
    type: "image",
    nodeTypeVersion: 1,
    title: upload.title,
    position: upload.position,
    width: upload.width,
    height: upload.height,
    zIndex: 1,
    fileReferences: [
      {
        purpose: "CONTENT",
        referenceKey: "old",
        fileId: crypto.randomUUID(),
        version: "1",
      },
    ],
  };
  const mixed = connectCanvasDocumentSources(
    { ...f.candidate, nodes: [...f.candidate.nodes, upload] },
    [upload.id],
    f.target.id,
  );
  expect(mixed.connections).toHaveLength(3);
  expect(f.client.canAcceptConnectionGraph(mixed)).toBe(false);
  // Exercise the unchanged ordinary uploader to prove why its admission is insufficient.
  f.client.change(mixed, true);
  window.dispatchEvent(new Event("online"));
  await vi.waitFor(() => expect(f.sockets).toHaveLength(2));
  f.sockets[1].onopen?.();
  f.sockets[1].receive({
    type: "yjs_snapshot",
    actor_id: f.actorID,
    yjs_state: f.state,
  });
  await vi.waitFor(() =>
    expect(f.sockets[1].sent.some((p) => p.type === "yjs_submit")).toBe(true),
  );
  const packet = f.sockets[1].sent.find((p) => p.type === "yjs_submit")!;
  const server = new Y.Doc();
  Y.applyUpdate(server, Buffer.from(f.state.state, "base64"));
  Y.applyUpdate(
    server,
    Buffer.from((packet.body as { update: string }).update, "base64"),
  );
  const document = serviceProjection(
    readCanvasDocument(server),
    f.state.document as unknown as ServiceDocument,
  );
  expect(document.nodes).toHaveLength(3);
  expect(document.edges).toHaveLength(2);
  expect(document.nodes.some((node) => node.id === upload.id)).toBe(false);
  server.destroy();
});

it("connection media projection rejects unfinished local bytes and strips only a matching completed receipt preview", async () => {
  const f = await connectionFixture();
  const ordinary = f.candidate.nodes[0];
  expect(f.client.connectionMediaNode(ordinary)).toBe(ordinary);
  const upload = createCanvasNode(
    "image",
    { x: 0, y: 0 },
    { content: "blob:preview", status: "success", naturalWidth: 640 },
  );
  expect(() => f.client.connectionMediaNode(upload)).toThrow();
  const asset: API.AssetSummary = {
    id: crypto.randomUUID(),
    project_id: f.state.document.project_id,
    revision: 1,
    kind: "image",
  };
  vi.spyOn(publishMedia, "publishCanvasMedia").mockImplementation(
    async (state, _projectID, receipts) => {
      receipts.set(upload.id, {
        source: "blob:preview",
        key: "receipt",
        asset,
      });
      return {
        ...state,
        nodes: state.nodes.map((node) => ({
          ...node,
          documentData: {
            id: node.id,
            type: node.type,
            title: node.title,
            position: node.position,
            width: node.width,
            height: node.height,
            zIndex: 1,
            nodeTypeVersion: 1,
            fileReferences: [
              {
                referenceKey: asset.id!,
                fileId: asset.id!,
                version: "1",
                purpose: "CONTENT",
              },
            ],
          },
        })),
      };
    },
  );
  const graph = f.client.getSnapshot().document!;
  f.client.change({ ...graph, nodes: [...graph.nodes, upload] }, true);
  await vi.waitFor(() =>
    expect(
      f.client
        .getSnapshot()
        .document?.nodes.find((node) => node.id === upload.id)?.documentData
        ?.fileReferences?.[0]?.fileId,
    ).toBe(asset.id),
  );
  const ready = f.client
    .getSnapshot()
    .document!.nodes.find((node) => node.id === upload.id)!;
  const withDisplayMetadata = {
    ...ready,
    metadata: { ...ready.metadata, naturalWidth: 640 },
  };
  const projected = f.client.connectionMediaNode(withDisplayMetadata);
  expect(ready.metadata?.content).toBe("blob:preview");
  expect(projected.metadata?.content).toBeUndefined();
  expect(projected.documentData).toBe(ready.documentData);
  expect(projected.metadata?.naturalWidth).toBe(640);
  expect(() =>
    f.client.connectionMediaNode({
      ...ready,
      metadata: { ...ready.metadata, content: "blob:replacement" },
    }),
  ).toThrow();
  expect(() =>
    f.client.connectionMediaNode({
      ...ready,
      documentData: { ...ready.documentData!, fileReferences: [] },
    }),
  ).toThrow();
});

async function durableSessionFixture() {
  const seed = setup();
  seed.client.stop();
  const records = new Map<string, RecoveryRecord<ServiceGraph>>(),
    locks = new Set<string>();
  let failWrites = false;
  const store: CanvasPendingStore = {
    list: async (namespace) =>
      [...records]
        .filter(([key]) => key.startsWith(namespace + ":"))
        .map(([, record]) => structuredClone(record)),
    put: async (namespace, record) => {
      if (failWrites) throw new Error("QuotaExceededError");
      records.set(
        namespace + ":" + record.scope.sessionId + ":" + record.scope.epoch,
        structuredClone(record),
      );
    },
    remove: async (namespace, scope) => {
      if (failWrites) throw new Error("QuotaExceededError");
      records.delete(namespace + ":" + scope.sessionId + ":" + scope.epoch);
    },
  };
  const scopeID = crypto.randomUUID();
  const open = async (actorID = seed.actorID, epoch = seed.state.epoch) => {
    const sockets: Socket[] = [];
    const client = new CanvasYjsLiveSession({
      canvasID: seed.state.document.id!,
      projectID: seed.state.document.project_id!,
      actorID,
      scopeID,
      csrf: () => "csrf-not-journaled",
      renew: async () => {},
      recoveryJournal: (identity) =>
        openCanvasPendingJournal(identity, {
          store,
          claim: async (scope) => {
            if (locks.has(scope.sessionId)) return null;
            locks.add(scope.sessionId);
            return async () => {
              locks.delete(scope.sessionId);
            };
          },
        }),
      socket: () => {
        const socket = new Socket();
        sockets.push(socket);
        return socket as unknown as WebSocket;
      },
    });
    instances.push(client);
    client.start();
    await vi.waitFor(() => expect(sockets).toHaveLength(1));
    sockets[0].onopen?.();
    sockets[0].receive({
      type: "yjs_snapshot",
      actor_id: actorID,
      yjs_state: { ...seed.state, epoch },
    });
    await vi.waitFor(() =>
      expect(client.getSnapshot().document).not.toBeNull(),
    );
    return { client, sockets };
  };
  const close = async (client: CanvasYjsLiveSession) => {
    client.stop();
    await vi.waitFor(() => expect(locks.size).toBe(0));
  };
  return {
    ...seed,
    open,
    close,
    records,
    failWrites: () => {
      failWrites = true;
    },
  };
}
it("reopening after lost acknowledgement resolves the durably stored original packet before offering uncommitted intent recovery", async () => {
  const f = await durableSessionFixture(),
    first = await f.open();
  const next = structuredClone(first.client.getSnapshot().document!);
  next.nodes[0].position.x = 777;
  first.client.change(next, true);
  await vi.waitFor(() =>
    expect(first.sockets[0].sent.some((p) => p.type === "yjs_submit")).toBe(
      true,
    ),
  );
  const original = first.sockets[0].sent.find((p) => p.type === "yjs_submit")!;
  expect(
    [...f.records.values()].some(
      (record) => record.inFlight?.updateId === original.op_id,
    ),
  ).toBe(true);
  await f.close(first.client);
  const reopened = await f.open();
  await vi.waitFor(() =>
    expect(reopened.sockets[0].sent.some((p) => p.type === "yjs_resolve")).toBe(
      true,
    ),
  );
  const resolved = reopened.sockets[0].sent.find(
    (p) => p.type === "yjs_resolve",
  )!;
  expect(resolved.op_id).toBe(original.op_id);
  expect(resolved.body).toEqual(original.body);
  expect(reopened.sockets[0].sent.some((p) => p.type === "yjs_submit")).toBe(
    false,
  );
  reopened.sockets[0].receive({ type: "yjs_not_found", op_id: original.op_id });
  await vi.waitFor(() =>
    expect(reopened.client.getSnapshot().recovery).toHaveLength(1),
  );
  expect(reopened.client.getSnapshot().document!.nodes[0].position.x).toBe(0);
  expect(JSON.stringify(reopened.client.getSnapshot().recovery)).toContain(
    "777",
  );
  expect(reopened.sockets[0].sent.some((p) => p.type === "yjs_submit")).toBe(
    false,
  );
  expect(JSON.stringify([...f.records.values()])).not.toContain(
    "csrf-not-journaled",
  );
  const recovered = reopened.client.getSnapshot().recovery[0];
  await reopened.client.applyRecovery(
    recovered.sourceId,
    recovered.fields,
    reopened.client.commitSeq,
  );
  await vi.waitFor(() =>
    expect(
      reopened.sockets[0].sent.some((packet) => packet.type === "yjs_submit"),
    ).toBe(true),
  );
  expect(
    reopened.sockets[0].sent.find((packet) => packet.type === "yjs_submit")!
      .op_id,
  ).not.toBe(original.op_id);
  expect(reopened.client.getSnapshot().document!.nodes[0].position.x).toBe(777);
});
it("device recovery cannot cross actor identity and a forbidden response keeps its disk draft", async () => {
  const f = await durableSessionFixture(),
    first = await f.open();
  const next = structuredClone(first.client.getSnapshot().document!);
  next.nodes[0].position.x = 777;
  first.client.change(next, true);
  await vi.waitFor(() =>
    expect(first.sockets[0].sent.some((p) => p.type === "yjs_submit")).toBe(
      true,
    ),
  );
  first.sockets[0].receive({ type: "error", status: 403, code: "forbidden" });
  await vi.waitFor(() =>
    expect(first.client.getSnapshot().document).toBeNull(),
  );
  await f.close(first.client);
  expect([...f.records.values()].some((record) => record.inFlight)).toBe(true);
  const other = await f.open(crypto.randomUUID());
  expect(other.client.getSnapshot().document!.nodes[0].position.x).toBe(0);
  expect(other.sockets[0].sent.some((p) => p.type === "yjs_resolve")).toBe(
    false,
  );
});
it("a device journal quota failure blocks transport submission and never labels the draft protected", async () => {
  const f = await durableSessionFixture(),
    opened = await f.open();
  f.failWrites();
  const next = structuredClone(opened.client.getSnapshot().document!);
  next.nodes[0].position.x = 777;
  opened.client.change(next, true);
  await vi.waitFor(() =>
    expect(opened.client.getSnapshot().localPersistence).toBe("UNAVAILABLE"),
  );
  expect(opened.sockets[0].sent.some((p) => p.type === "yjs_submit")).toBe(
    false,
  );
  expect(opened.client.getSnapshot().phase).toContain("请勿关闭");
  expect(opened.client.getSnapshot().dirty).toBe(true);
});
it("a reopened accepted submission consumes its exact receipt without creating another update identity", async () => {
  const f = await durableSessionFixture(),
    first = await f.open();
  const next = structuredClone(first.client.getSnapshot().document!);
  next.nodes[0].position.x = 777;
  first.client.change(next, true);
  await vi.waitFor(() =>
    expect(first.sockets[0].sent.some((p) => p.type === "yjs_submit")).toBe(
      true,
    ),
  );
  const original = first.sockets[0].sent.find((p) => p.type === "yjs_submit")!;
  const body = original.body as { payloadHash: string; update: string };
  await f.close(first.client);
  const reopened = await f.open();
  await vi.waitFor(() =>
    expect(reopened.sockets[0].sent.some((p) => p.type === "yjs_resolve")).toBe(
      true,
    ),
  );
  reopened.sockets[0].receive({
    type: "yjs_ack",
    op_id: original.op_id,
    yjs_receipt: {
      status: "accepted",
      updateId: original.op_id,
      payloadHash: body.payloadHash,
      epoch: f.state.epoch,
      commitSeq: "1",
      contentRevision: "2",
      graphHash: "b".repeat(64),
      effect: "VISIBLE_CHANGE",
      update: body.update,
    },
  });
  await vi.waitFor(() =>
    expect(reopened.client.getSnapshot().dirty).toBe(false),
  );
  expect(reopened.client.getSnapshot().document!.nodes[0].position.x).toBe(777);
  expect(reopened.sockets[0].sent.some((p) => p.type === "yjs_submit")).toBe(
    false,
  );
  expect([...f.records.values()]).toEqual([]);
});
it("an epoch change queries the old frozen operation then retains a review draft instead of replaying old bytes", async () => {
  const f = await durableSessionFixture(),
    first = await f.open();
  const next = structuredClone(first.client.getSnapshot().document!);
  next.nodes[0].position.x = 777;
  first.client.change(next, true);
  await vi.waitFor(() =>
    expect(first.sockets[0].sent.some((p) => p.type === "yjs_submit")).toBe(
      true,
    ),
  );
  const original = first.sockets[0].sent.find((p) => p.type === "yjs_submit")!;
  await f.close(first.client);
  const epoch = crypto.randomUUID(),
    reopened = await f.open(f.actorID, epoch);
  await vi.waitFor(() =>
    expect(reopened.sockets[0].sent.some((p) => p.type === "yjs_resolve")).toBe(
      true,
    ),
  );
  expect(
    reopened.sockets[0].sent.find((p) => p.type === "yjs_resolve")!.body,
  ).toEqual(original.body);
  reopened.sockets[0].receive({ type: "yjs_not_found", op_id: original.op_id });
  await vi.waitFor(() =>
    expect(reopened.client.getSnapshot().recovery).toHaveLength(1),
  );
  expect(reopened.client.getDocumentEpoch()).toBe(epoch);
  expect(reopened.client.getSnapshot().document!.nodes[0].position.x).toBe(0);
  expect(reopened.sockets[0].sent.some((p) => p.type === "yjs_submit")).toBe(
    false,
  );
  expect([...f.records.values()].map((record) => record.scope.epoch)).toEqual([
    epoch,
  ]);
  expect([...f.records.values()][0].recoverySources[0].scope.epoch).toBe(
    f.state.epoch,
  );
});

it("audio input connection guard rejects target-local edits without blocking an unrelated composer", async () => {
  const f = inputFixture();
  await vi.waitFor(() =>
    expect(f.client.getSnapshot().document).not.toBeNull(),
  );
  const composer = createInputComposer(f);
  f.client.bindInputComposer(f.nodeID, composer);
  f.client.registerInputGuard(crypto.randomUUID(), () => false);
  expect(f.client.canAcceptAudioConnectionInput(f.nodeID)).toBe(true);
  composer.editor.commands.insertContent("尚未提交的音频指令");
  expect(f.client.canAcceptAudioConnectionInput(f.nodeID)).toBe(false);
  expect(f.sockets[0].sent.some((p) => p.type === "yjs_submit")).toBe(false);
});
it("audio input connection guard refuses composition and pending mode selection", async () => {
  const f = inputFixture();
  await vi.waitFor(() =>
    expect(f.client.getSnapshot().document).not.toBeNull(),
  );
  const composer = createInputComposer(f);
  f.client.bindInputComposer(f.nodeID, composer);
  composer.beginModeSelection({ mode: "audio", modelVersion: "audio-test" });
  expect(f.client.canAcceptAudioConnectionInput(f.nodeID)).toBe(false);
  composer.cancelModeSelection();
  expect(f.client.canAcceptAudioConnectionInput(f.nodeID)).toBe(true);
  composer.editor.view.dom.dispatchEvent(
    new CompositionEvent("compositionstart", { bubbles: true }),
  );
  expect(f.client.canAcceptAudioConnectionInput(f.nodeID)).toBe(false);
});
it("audio input connection guard refuses missing, retained, and readonly targets", async () => {
  const f = inputFixture();
  await vi.waitFor(() =>
    expect(f.client.getSnapshot().document).not.toBeNull(),
  );
  expect(f.client.canAcceptAudioConnectionInput(crypto.randomUUID())).toBe(
    false,
  );
  const remove = f.client.registerInputGuard(f.nodeID, () => false);
  expect(f.client.canAcceptAudioConnectionInput(f.nodeID)).toBe(false);
  remove();
  f.client.retainInputDraft(f.nodeID, emptyPromptDraft());
  expect(f.client.canAcceptAudioConnectionInput(f.nodeID)).toBe(false);
  f.client.resolveInputDraft(f.nodeID, "server");
  expect(f.client.canAcceptAudioConnectionInput(f.nodeID)).toBe(true);
  f.sockets[0].receive({
    type: "heartbeat",
    permissions: { read: true, write: false },
  });
  await vi.waitFor(() => expect(f.client.getSnapshot().writable).toBe(false));
  expect(f.client.canAcceptAudioConnectionInput(f.nodeID)).toBe(false);
});
it("a durable rejection after reopening retains review and permits an explicitly approved replacement operation", async () => {
  const f = await durableSessionFixture(),
    first = await f.open();
  const next = structuredClone(first.client.getSnapshot().document!);
  next.nodes[0].position.x = 777;
  first.client.change(next, true);
  await vi.waitFor(() =>
    expect(first.sockets[0].sent.some((p) => p.type === "yjs_submit")).toBe(
      true,
    ),
  );
  const original = first.sockets[0].sent.find((p) => p.type === "yjs_submit")!;
  await f.close(first.client);
  const reopened = await f.open();
  await vi.waitFor(() =>
    expect(reopened.sockets[0].sent.some((p) => p.type === "yjs_resolve")).toBe(
      true,
    ),
  );
  reopened.sockets[0].receive({
    type: "yjs_ack",
    op_id: original.op_id,
    yjs_receipt: { status: "rejected", code: "VALIDATION_FAILED" },
  });
  await vi.waitFor(() =>
    expect(reopened.sockets[0].sent.some((p) => p.type === "sync")).toBe(true),
  );
  reopened.sockets[0].receive({
    type: "yjs_snapshot",
    actor_id: f.actorID,
    yjs_state: f.state,
  });
  await vi.waitFor(() =>
    expect(reopened.client.getSnapshot().recovery).toHaveLength(1),
  );
  expect(reopened.sockets[0].sent.some((p) => p.type === "yjs_submit")).toBe(
    false,
  );
  const recovery = reopened.client.getSnapshot().recovery[0];
  await reopened.client.applyRecovery(
    recovery.sourceId,
    recovery.fields,
    reopened.client.commitSeq,
  );
  await vi.waitFor(() =>
    expect(reopened.sockets[0].sent.some((p) => p.type === "yjs_submit")).toBe(
      true,
    ),
  );
  expect(
    reopened.sockets[0].sent.find((p) => p.type === "yjs_submit")!.op_id,
  ).not.toBe(original.op_id);
});
