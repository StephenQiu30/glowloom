import { afterEach, describe, expect, it, vi } from "vitest";
import { waitFor } from "@testing-library/react";
import type { Operation } from "@/components/canvas/service-document";
import { CanvasAutosaveSession } from "@/components/canvas/autosave-session";
import { serviceGraph } from "@/components/canvas/service-graph";
import type { CanvasEditorState } from "@/components/canvas/lib/canvas-document-adapter";

const canvas = "11111111-1111-4111-8111-111111111111";
const project = "22222222-2222-4222-8222-222222222222";
const node = "33333333-3333-4333-8333-333333333333";
function graph(): CanvasEditorState {
  return serviceGraph(
    {
      id: canvas,
      project_id: project,
      revision: "5",
      schema_version: 1,
      operation_version: 1,
      permissions: { read: true, write: true },
      nodes: [
        {
          id: node,
          node_type: "text",
          node_action: "resource",
          title: "原始",
          x: 0,
          y: 0,
          width: 320,
          height: 200,
          config: { format: "plain", format_version: 1, text: "正文" },
        },
      ],
      edges: [],
    },
    canvas,
    project,
  );
}
function title(state: CanvasEditorState, value: string) {
  return { ...state, nodes: state.nodes.map((n) => ({ ...n, title: value })) };
}
function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (cause: Error) => void;
  const promise = new Promise<T>((yes, no) => {
    resolve = yes;
    reject = no;
  });
  return { promise, resolve, reject };
}
function setup() {
  const original = graph();
  const request = deferred<{ document: CanvasEditorState; revision: string }>();
  const commit = vi.fn<
    (
      operations: Operation[],
      revision: string,
      strictBase?: boolean,
    ) => typeof request.promise
  >(() => request.promise);
  const session = new CanvasAutosaveSession({
    document: original,
    revision: "5",
    projectID: project,
    commit,
  });
  session.receive(original, "5", true, true);
  return { session, original, commit, request };
}
afterEach(() => vi.unstubAllGlobals());
describe("single canvas automatic save queue", () => {
  it("ignores the initial unchanged editor callback before authorization", async () => {
    const original = graph();
    const commit = vi.fn();
    const session = new CanvasAutosaveSession({
      document: original,
      revision: "5",
      projectID: project,
      commit,
    });
    session.edit(structuredClone(original), true);
    expect(session.getSnapshot().error).toBe("");
    session.receive(original, "5", true, true);
    session.edit(title(original, "可以编辑"), true);
    await waitFor(() => expect(commit).toHaveBeenCalledOnce());
    session.dispose();
  });
  it("finishes composition after an older receipt without replacing the input surface", async () => {
    const { session, original, commit, request } = setup();
    const first = title(original, "第一笔");
    session.edit(first, true);
    await waitFor(() => expect(commit).toHaveBeenCalledOnce());
    const composing = title(first, "组合");
    session.edit(composing, false);
    const surface = session.getSnapshot().replacement;
    request.resolve({ document: first, revision: "6" });
    await waitFor(() => expect(session.getSnapshot().phase).toBe("editing"));
    expect(session.getSnapshot().replacement).toBe(surface);
    expect(commit).toHaveBeenCalledOnce();
    session.edit(title(composing, "组合完成"), true);
    await waitFor(() => expect(commit).toHaveBeenCalledTimes(2));
    expect(commit.mock.calls[1][1]).toBe("6");
    session.dispose();
  });
  it("does not require browser business persistence to save or reopen", async () => {
    const fail = () => {
      throw new Error("persistent business storage disabled");
    };
    for (const name of [
      "localStorage",
      "sessionStorage",
      "indexedDB",
      "caches",
    ])
      vi.stubGlobal(name, new Proxy({}, { get: fail }));
    const { session, original, commit, request } = setup();
    session.edit(title(original, "只在服务端持久化"), true);
    await waitFor(() => expect(commit).toHaveBeenCalledOnce());
    const durable = {
      document: title(original, "只在服务端持久化"),
      revision: "6",
    };
    request.resolve(durable);
    await waitFor(() => expect(session.getSnapshot().phase).toBe("saved"));
    session.dispose();
    const reopened = new CanvasAutosaveSession({
      ...durable,
      projectID: project,
      commit,
    });
    expect(reopened.getSnapshot().document.nodes[0].title).toBe(
      "只在服务端持久化",
    );
    reopened.dispose();
  });
  it("allows an authorized preview projection in a read-only canvas without treating it as a write", () => {
    const { session, original } = setup();
    const media = {
      ...original,
      nodes: original.nodes.map((n) => ({
        ...n,
        type: "image",
        metadata: { content: "" },
      })),
    };
    session.receive(media, "6", false, false);
    session.edit(
      {
        ...media,
        nodes: media.nodes.map((n) => ({
          ...n,
          metadata: { content: "https://media.example/authorized-preview" },
        })),
      },
      true,
    );
    expect(session.getSnapshot().error).toBe("");
    expect(session.getSnapshot().dirty).toBe(false);
    session.dispose();
  });
  it("submits a completed edit without a click, idle timer or polling", async () => {
    const { session, original, commit, request } = setup();
    session.edit(title(original, "第一笔"), true);
    await waitFor(() => expect(commit).toHaveBeenCalledOnce());
    expect(commit).toHaveBeenCalledWith(
      [{ type: "set", node_id: node, path: ["title"], value: "第一笔" }],
      "5",
    );
    expect(session.getSnapshot().dirty).toBe(true);
    request.resolve({ document: title(original, "第一笔"), revision: "6" });
    await waitFor(() => expect(session.getSnapshot().phase).toBe("saved"));
    session.dispose();
  });
  it("keeps Q editable while P waits and never lets an old ACK clear it", async () => {
    const { session, original, commit, request } = setup();
    const second = deferred<{
      document: CanvasEditorState;
      revision: string;
    }>();
    commit
      .mockImplementationOnce(() => request.promise)
      .mockImplementationOnce(() => second.promise);
    session.edit(title(original, "第一笔"), true);
    await waitFor(() => expect(commit).toHaveBeenCalledOnce());
    session.edit(title(original, "第二笔"), true);
    request.resolve({ document: title(original, "第一笔"), revision: "6" });
    await waitFor(() => expect(commit).toHaveBeenCalledTimes(2));
    expect(session.getSnapshot().dirty).toBe(true);
    expect(commit.mock.calls[1]).toEqual([
      [{ type: "set", node_id: node, path: ["title"], value: "第二笔" }],
      "6",
    ]);
    second.resolve({ document: title(original, "第二笔"), revision: "7" });
    await waitFor(() => expect(session.getSnapshot().phase).toBe("saved"));
    session.dispose();
  });
  it("waits for the completed IME/gesture transaction", async () => {
    const { session, original, commit, request } = setup();
    session.edit(title(original, "组"), false);
    await Promise.resolve();
    expect(commit).not.toHaveBeenCalled();
    expect(session.getSnapshot().dirty).toBe(true);
    session.edit(title(original, "组合输入"), true);
    await waitFor(() => expect(commit).toHaveBeenCalledOnce());
    request.resolve({ document: title(original, "组合输入"), revision: "6" });
    session.dispose();
  });
  it("retains an offline draft and sends automatically after the same session is ready", async () => {
    const { session, original, commit, request } = setup();
    session.receive(original, "5", true, false);
    session.edit(title(original, "断线输入"), true);
    await Promise.resolve();
    expect(commit).not.toHaveBeenCalled();
    session.receive(original, "5", true, true);
    await waitFor(() => expect(commit).toHaveBeenCalledOnce());
    request.resolve({ document: title(original, "断线输入"), revision: "6" });
    await waitFor(() => expect(session.getSnapshot().phase).toBe("saved"));
    session.dispose();
  });
  it("retains a rejected draft, does not loop, and discards only on explicit choice", async () => {
    const { session, original, commit, request } = setup();
    session.edit(title(original, "保留输入"), true);
    await waitFor(() => expect(commit).toHaveBeenCalledOnce());
    request.reject(new Error("修改未保存：compare_failed"));
    await waitFor(() => expect(session.getSnapshot().phase).toBe("recovery"));
    session.receive(original, "5", true, true);
    await Promise.resolve();
    expect(commit).toHaveBeenCalledOnce();
    expect(session.getSnapshot().document.nodes[0].title).toBe("保留输入");
    session.useAccepted();
    expect(session.getSnapshot().phase).toBe("saved");
    session.dispose();
  });
  it("preserves an independent remote position while submitting a local title", async () => {
    const { session, original, commit, request } = setup();
    session.receive(original, "5", true, false);
    session.edit(title(original, "本地标题"), true);
    const remote = {
      ...original,
      nodes: original.nodes.map((n) => ({ ...n, position: { x: 70, y: 80 } })),
    };
    session.receive(remote, "6", true, true);
    await waitFor(() => expect(commit).toHaveBeenCalledOnce());
    expect(commit.mock.calls[0][1]).toBe("6");
    expect(session.getSnapshot().document.nodes[0].position).toEqual({
      x: 70,
      y: 80,
    });
    request.resolve({ document: title(remote, "本地标题"), revision: "7" });
    session.dispose();
  });
  it("does not overwrite a competing remote field during offline recovery", async () => {
    const { session, original, commit } = setup();
    session.receive(original, "5", true, false);
    session.edit(title(original, "本地"), true);
    session.receive(title(original, "远端"), "6", true, true);
    await Promise.resolve();
    expect(commit).not.toHaveBeenCalled();
    expect(session.getSnapshot().phase).toBe("recovery");
    expect(session.getSnapshot().document.nodes[0].title).toBe("本地");
    session.dispose();
  });
  it("saves independent text while media is uploading and ignores a deleted target's late result", async () => {
    const original = graph();
    const media = {
      id: "44444444-4444-4444-8444-444444444444",
      type: "image",
      title: "待上传",
      position: { x: 400, y: 0 },
      width: 200,
      height: 200,
      metadata: { content: "blob:only-in-memory" },
    };
    const uploaded = deferred<CanvasEditorState>();
    const publish = vi.fn(() => uploaded.promise);
    const accepted = title(original, "独立编辑");
    const commit = vi
      .fn()
      .mockResolvedValue({ document: accepted, revision: "6" });
    const session = new CanvasAutosaveSession({
      document: original,
      revision: "5",
      projectID: project,
      commit,
      publish,
    });
    session.receive(original, "5", true, true);
    session.edit({ ...accepted, nodes: [...accepted.nodes, media] }, true);
    await waitFor(() => expect(commit).toHaveBeenCalledOnce());
    expect(JSON.stringify(commit.mock.calls)).not.toContain("blob:");
    await waitFor(() => expect(session.getSnapshot().phase).toBe("uploading"));
    expect(session.getSnapshot().dirty).toBe(true);
    session.edit(
      { ...session.getSnapshot().document, nodes: accepted.nodes },
      true,
    );
    uploaded.resolve({ ...accepted, nodes: [media] });
    await waitFor(() => expect(session.getSnapshot().phase).toBe("saved"));
    expect(session.getSnapshot().document.nodes).toHaveLength(1);
    expect(commit).toHaveBeenCalledOnce();
    session.dispose();
  });
  it("does not freeze an ongoing IME when its completed-transaction budget is full", async () => {
    const original = graph();
    const session = new CanvasAutosaveSession({
      document: original,
      revision: "5",
      projectID: project,
      commit: vi.fn(),
      maxTransactions: 2,
    });
    session.receive(original, "5", true, false);
    session.edit(title(original, "一次"), true);
    session.edit(title(original, "组"), false);
    session.edit(title(original, "组合"), false);
    session.edit(title(original, "组合完成"), true);
    expect(session.getSnapshot().error).toBe("");
    expect(session.getSnapshot().document.nodes[0].title).toBe("组合完成");
    expect(session.getSnapshot().writable).toBe(false);
    session.dispose();
  });
  it("deletes a pending node after a canonical server receipt without a false recovery conflict", async () => {
    const { session, original, commit, request } = setup();
    session.edit(title(original, "已发送"), true);
    await waitFor(() => expect(commit).toHaveBeenCalledOnce());
    session.edit({ ...original, nodes: [] }, true);
    const canonical = title(original, "已发送");
    canonical.nodes[0].serviceData = {
      ...canonical.nodes[0].serviceData!,
      title: "已发送",
      content_version: "2",
    };
    canonical.nodes[0].documentData = {
      ...canonical.nodes[0].documentData!,
      title: "已发送",
    };
    request.resolve({ document: canonical, revision: "6" });
    await waitFor(() => expect(commit).toHaveBeenCalledTimes(2));
    expect(commit.mock.calls[1][0]).toEqual([
      { type: "delete_node", node_id: node },
    ]);
    expect(session.getSnapshot().error).toBe("");
    session.dispose();
  });
  it("requires review against the current remote revision before reapplying rejected input", async () => {
    const { session, original, commit, request } = setup();
    session.receive(original, "5", true, false);
    session.edit(title(original, "我的标题"), true);
    session.receive(title(original, "他人的标题"), "6", true, true);
    const review = session.reviewRecovery();
    expect(review.document.nodes[0].title).toBe("我的标题");
    session.receive(title(original, "再次更新"), "7", true, true);
    expect(() => session.applyRecovery(review)).toThrow("已更新");
    expect(commit).not.toHaveBeenCalled();
    const latest = session.reviewRecovery();
    session.applyRecovery(latest);
    await waitFor(() => expect(commit).toHaveBeenCalledOnce());
    expect(commit.mock.calls[0][1]).toBe("7");
    expect(commit.mock.calls[0][2]).toBe(true);
    request.resolve({ document: title(original, "我的标题"), revision: "8" });
    await waitFor(() => expect(session.getSnapshot().phase).toBe("saved"));
    session.dispose();
  });
  it("requires another review if the server advances before a reviewed recovery can send", async () => {
    const { session, original, commit } = setup();
    session.receive(original, "5", true, false);
    session.edit(title(original, "我的标题"), true);
    session.receive(title(original, "他人的标题"), "6", true, false);
    session.applyRecovery(session.reviewRecovery());
    session.receive(title(original, "第三个标题"), "7", true, true);
    await Promise.resolve();
    expect(commit).not.toHaveBeenCalled();
    expect(session.getSnapshot().phase).toBe("recovery");
    expect(session.getSnapshot().document.nodes[0].title).toBe("我的标题");
    session.dispose();
  });
  it("does not recreate a remotely deleted node through a stale recovery review", () => {
    const { session, original } = setup();
    session.receive(original, "5", true, false);
    session.edit(title(original, "我的标题"), true);
    session.receive({ ...original, nodes: [] }, "6", true, true);
    expect(() => session.reviewRecovery()).toThrow();
    session.dispose();
  });
  it("does not revive or notify a disposed editing session after a late ACK", async () => {
    const { session, original, commit, request } = setup();
    const notify = vi.fn();
    session.subscribe(notify);
    session.edit(title(original, "第一笔"), true);
    await waitFor(() => expect(commit).toHaveBeenCalledOnce());
    session.dispose();
    const count = notify.mock.calls.length;
    request.resolve({ document: title(original, "第一笔"), revision: "6" });
    await Promise.resolve();
    await Promise.resolve();
    expect(notify).toHaveBeenCalledTimes(count);
  });
});
