"use client";

import { useCallback, useEffect, useRef } from "react";
import type { Editor } from "@tiptap/core";
import { getCanvas } from "@/api/canvases";
import type {
  ComposerSession,
  ComposerCollaboration,
} from "@/components/generation/composer-session";
import { emptyPromptDraft } from "@/components/generation/prompt-document";
import { CanvasLive, type LiveStatus } from "./live";
import { Collaboration, type CollaborationTransition } from "./collaboration";
import { isDraft, requireCanvas, type Canvas } from "./document";
import {
  applySequenceTransaction,
  plainEditorText,
  promptSequence,
} from "./editor-ot";
import { canvasOperations, type EditTrace } from "./operation-adapters";
import {
  fieldValue,
  fieldContext,
  fieldPolicy,
  applyOperations,
  type Operation,
} from "./operations";
import { sequenceLength, type SequenceEdit } from "./sequence";

type PlainEditor = { nodeID: string; path: string[]; session: ComposerSession };
type State = {
  pending: boolean;
  unconfirmed: boolean;
  recovering: boolean;
  invalidDraft: boolean;
  rejection?: string;
};
type Callbacks = {
  update: (document: Canvas, base: Canvas, state: State) => void;
  status: (status: LiveStatus, writable?: boolean) => void;
  revoked: () => void;
  message: (message: string) => void;
};
function textDraft(value: string) {
  const draft = emptyPromptDraft();
  draft.document.content = value.split("\n").map((text) => ({
    type: "paragraph",
    ...(text ? { content: [{ type: "text" as const, text }] } : {}),
  }));
  return draft;
}
function mentionCleanup(
  doc: Parameters<typeof promptSequence>[0],
  id: string,
): SequenceEdit {
  return promptSequence(doc).map((atom) =>
    atom.type === "reference" && atom.reference_id === id
      ? { delete: 1 }
      : { retain: sequenceLength([atom]) },
  );
}

/** Owns one authenticated canvas session and mounted editor registrations. All
 * recovery data lives only in this mounted session and is never browser storage. */
export function useCanvasCollaboration(
  initial: Canvas,
  enabled: boolean,
  callbacks: Callbacks,
) {
  const seed = useRef(initial);
  useEffect(() => {
    seed.current = initial;
  }, [initial]);
  const latest = useRef(callbacks);
  useEffect(() => {
    latest.current = callbacks;
  }, [callbacks]);
  const controller = useRef<Collaboration | null>(null),
    socket = useRef<CanvasLive | null>(null);
  const prompts = useRef(new Map<string, ComposerSession>()),
    plain = useRef(new Map<string, PlainEditor>()),
    documents = useRef(new Map<string, Editor>());
  const recovery = useRef<Canvas | undefined>(undefined),
    extra = useRef<{ before: Canvas; desired: Canvas } | undefined>(undefined);
  const update = useRef<
    (event?: CollaborationTransition, reset?: boolean) => void
  >(() => {});
  const composing = useRef(new Set<string>());
  const resetEditors = useRef(false);
  const stop = useRef<() => void>(() => {});
  const syncEditors = (
    doc: Canvas,
    event?: CollaborationTransition,
    reset = false,
    only?: { kind: "prompt" | "plain"; key: string },
  ) => {
    for (const [id, session] of prompts.current) {
      if (only && (only.kind !== "prompt" || only.key !== id)) continue;
      const node = doc.nodes.find((n) => n.id === id);
      if (
        !node ||
        !isDraft(node) ||
        !node.config.draft.composer ||
        session.editor.isDestroyed
      )
        continue;
      let tr = session.editor.state.tr;
      if (reset)
        tr = tr.replaceWith(
          0,
          tr.doc.content.size,
          session.editor.schema.nodeFromJSON(
            node.config.draft.composer.document,
          ).content,
        );
      else
        for (const op of event?.operations ?? []) {
          if (op.node_id !== id) continue;
          if (
            op.type === "text" &&
            op.path?.join(".") === "config.draft.composer.document"
          )
            applySequenceTransaction(tr, op.text ?? []);
          if (
            op.type === "remove" &&
            op.path?.join(".") === "config.draft.composer.references"
          )
            applySequenceTransaction(
              tr,
              mentionCleanup(tr.doc.toJSON(), op.item_id!),
            );
        }
      session.applyAuthoritative(node.config.draft.composer, tr);
    }
    for (const [key, { nodeID, path, session }] of plain.current) {
      if (only && (only.kind !== "plain" || only.key !== key)) continue;
      const node = doc.nodes.find((n) => n.id === nodeID);
      if (!node || session.editor.isDestroyed) continue;
      const value = fieldValue(node, path).value;
      if (typeof value !== "string") continue;
      let tr = session.editor.state.tr;
      if (reset)
        tr = tr.replaceWith(
          0,
          tr.doc.content.size,
          session.editor.schema.nodeFromJSON(textDraft(value).document).content,
        );
      else
        for (const op of event?.operations ?? [])
          if (
            op.node_id === nodeID &&
            op.type === "text" &&
            op.path?.join(".") === path.join(".")
          )
            applySequenceTransaction(tr, op.text ?? [], true);
      if (plainEditorText(tr.doc) !== value)
        throw new Error("unmapped_authoritative_document");
      session.applyAuthoritative(
        { ...session.snapshot().draft, document: tr.doc.toJSON() },
        tr,
      );
    }
    for (const [id, editor] of documents.current) {
      if (only) continue;
      const node = doc.nodes.find((n) => n.id === id);
      if (!node || editor.isDestroyed) continue;
      if (reset) {
        editor.commands.setContent(node.config.text ?? "", {
          contentType: "markdown",
          emitUpdate: false,
        });
        continue;
      }
      const ops = (event?.operations ?? []).filter(
        (o) =>
          o.node_id === id &&
          o.type === "text" &&
          o.path?.join(".") === "config.text",
      );
      if (ops.length) {
        const tr = editor.state.tr;
        for (const op of ops) applySequenceTransaction(tr, op.text ?? [], true);
        editor.view.dispatch(
          tr.setMeta("preventUpdate", true).setMeta("addToHistory", false),
        );
      }
    }
  };
  useEffect(() => {
    if (!enabled) return;
    const initial = seed.current;
    let active = true,
      status: LiveStatus = "connecting",
      writable = false,
      actor = "",
      needResync = false,
      reading = false,
      muted = false,
      protocolBlocked = false;
    let shown = initial;
    const abort = new AbortController();
    const session = new Collaboration(
      initial,
      (event) => {
        if (!active || muted) return;
        update.current(event, resetEditors.current);
        resetEditors.current = false;
        void verifyResync();
      },
      (envelope) => live.sendCollaboration(envelope),
      {
        debounceMs: 900,
        validate: (doc) => {
          try {
            requireCanvas(doc, initial.project_id, initial.id);
            return true;
          } catch {
            return false;
          }
        },
      },
    );
    controller.current = session;
    update.current = (event, reset = false) => {
      if (!active) return;
      if (
        (session.recovery.length || session.pending?.invalid) &&
        !recovery.current
      )
        recovery.current = shown;
      const recovering = !!recovery.current || !!extra.current || needResync;
      const display = recovery.current ?? session.document;
      try {
        requireCanvas(session.base, initial.project_id, initial.id);
        if (!recovering) syncEditors(display, event, reset);
      } catch (error) {
        recovery.current ??= shown;
        latest.current.message(
          error instanceof Error
            ? error.message
            : "远端编辑无法映射，已保留未确认内容。",
        );
      }
      shown = recovery.current ?? display;
      latest.current.update(shown, session.base, {
        pending: !!session.pending,
        unconfirmed: !!session.pending && (status !== "live" || !writable),
        recovering: !!recovery.current || !!extra.current || needResync,
        invalidDraft: session.invalidDraft,
        rejection: session.lastRejection?.code,
      });
    };
    async function verifyResync() {
      if (!active || !needResync || reading || !session.canVerifyResync) return;
      reading = true;
      try {
        const fresh = requireCanvas(
          await getCanvas({ id: initial.id }, { signal: abort.signal }),
          initial.project_id,
          initial.id,
        );
        if (!active) return;
        muted = true;
        session.verifiedResync(fresh);
        if (!session.recovery.length && !extra.current)
          recovery.current = undefined;
        muted = false;
        needResync = false;
        resetEditors.current = true;
        update.current(undefined, !recovery.current);
        resetEditors.current = false;
        live.stop();
        live.start();
      } catch (error) {
        muted = false;
        if (active)
          latest.current.message(
            error instanceof Error
              ? error.message
              : "重同步尚未完成，保留原请求与编辑。",
          );
      } finally {
        reading = false;
      }
    }
    const live = new CanvasLive(initial.id, {
      snapshot: (value) => {
        const fresh = requireCanvas(value, initial.project_id, initial.id);
        if (!session.dirty) {
          session.verifiedResync(fresh);
          update.current(undefined, true);
        }
      },
      status: (value, canWrite) => {
        status = value;
        if (canWrite !== undefined) writable = canWrite && !protocolBlocked;
        session.connection(value === "live", writable, actor);
        latest.current.status(
          value,
          canWrite === undefined ? undefined : writable,
        );
        void verifyResync();
      },
      revoked: () => latest.current.revoked(),
      collaboration: {
        revision: () => session.base.revision,
        pending: () => session.pending,
        actor: (value) => {
          actor = value;
        },
        changes: (changes) => session.changes(changes),
        receipt: (value) => session.receipt(value),
        resync: () => {
          recovery.current ??= session.dirty ? shown : undefined;
          needResync = true;
          session.requireResync();
          void verifyResync();
        },
        error: (code) => {
          if (code !== "canvas_read_only") protocolBlocked = true;
          writable = false;
          session.connection(status === "live", false, actor);
          latest.current.status(status, false);
          latest.current.message(
            code === "canvas_read_only"
              ? "画布当前只读，原请求和未确认编辑仍保留。"
              : code,
          );
        },
      },
    });
    socket.current = live;
    live.start();
    const cleanup = () => {
      active = false;
      shown = { ...shown, nodes: [], edges: [] };
      abort.abort();
      live.stop();
      session.dispose();
      controller.current = null;
      socket.current = null;
      prompts.current.clear();
      plain.current.clear();
      documents.current.clear();
      composing.current.clear();
      recovery.current = undefined;
      extra.current = undefined;
      update.current = () => {};
    };
    stop.current = cleanup;
    return cleanup;
  }, [initial.id, initial.project_id, enabled]);
  const preserve = (before: Canvas, desired: Canvas, error: unknown) => {
    recovery.current ??= desired;
    extra.current ??= { before, desired };
    latest.current.message(
      error instanceof Error ? error.message : "无法提交本次编辑，已保留内容。",
    );
    update.current();
  };
  const suspend = useCallback(() => stop.current(), []);
  return {
    controller,
    suspend,
    edit: (desired: Canvas, trace: EditTrace = {}) => {
      const session = controller.current;
      if (!session) throw new Error("connection_read_only");
      const before = session.document;
      try {
        const ops = canvasOperations(before, desired, trace);
        session.enqueue(ops);
        if (!ops.length) update.current();
      } catch (error) {
        preserve(before, desired, error);
      }
    },
    flush: () => {
      const session = controller.current;
      if (!session) return Promise.reject(new Error("connection_read_only"));
      if (session.pending)
        socket.current?.sendCollaboration(
          { key: session.pending.key, body: session.pending.body },
          true,
        );
      return session.flush();
    },
    history: (direction: "undo" | "redo") =>
      controller.current?.history(direction) ?? false,
    composition: (owner: string, active: boolean) => {
      if (active) composing.current.add(owner);
      else composing.current.delete(owner);
      controller.current?.composition(composing.current.size > 0);
    },
    bindPrompt: (id: string, session: ComposerSession) => {
      prompts.current.set(id, session);
      const doc = controller.current?.document;
      if (doc) syncEditors(doc, undefined, true, { kind: "prompt", key: id });
      return () => {
        if (prompts.current.get(id) === session) prompts.current.delete(id);
      };
    },
    bindPlain: (nodeID: string, path: string[], session: ComposerSession) => {
      const key = nodeID + path.join(".");
      const entry = { nodeID, path, session };
      plain.current.set(key, entry);
      const doc = controller.current?.document;
      if (doc) syncEditors(doc, undefined, true, { kind: "plain", key });
      return () => {
        if (plain.current.get(key) === entry) plain.current.delete(key);
      };
    },
    bindDocument: (id: string, editor: Editor) => {
      documents.current.set(id, editor);
      return () => {
        if (documents.current.get(id) === editor) documents.current.delete(id);
      };
    },
    owner: (id: string): ComposerCollaboration => ({
      history: {
        undo: () => controller.current?.history("undo") ?? false,
        redo: () => controller.current?.history("redo") ?? false,
        canUndo: () => controller.current?.canUndo ?? false,
        canRedo: () => controller.current?.canRedo ?? false,
      },
      pendingReference: (ref) =>
        !controller.current?.base.nodes
          .find((n) => n.id === id)
          ?.config.draft?.composer?.references.some((r) => r.id === ref),
      composition: (active) => {
        if (active) composing.current.add(id);
        else composing.current.delete(id);
        controller.current?.composition(composing.current.size > 0);
      },
    }),
    copyRecovery: () =>
      navigator.clipboard.writeText(
        JSON.stringify(
          recovery.current ?? controller.current?.document,
          null,
          2,
        ),
      ),
    discard: () => {
      const session = controller.current;
      if (!session || session.dirty) throw new Error("request_unconfirmed");
      recovery.current = undefined;
      extra.current = undefined;
      resetEditors.current = true;
      session.discardRecovery();
      resetEditors.current = false;
      update.current(undefined, true);
    },
    retryFields: async () => {
      const session = controller.current,
        branch = recovery.current;
      if (!session || !branch || session.dirty)
        throw new Error("request_unconfirmed");
      const saved = session.recovery.flatMap((r) => r.operations);
      if (!saved.length)
        throw new Error("请复制保留内容，载入最新版本后重新编辑。");
      if (extra.current)
        throw new Error(
          "本次编辑缺少安全映射，请复制保留内容后在最新画布重新编辑。",
        );
      if (
        saved.some(
          (op) =>
            !["text", "set", "migrate_composer", "noop"].includes(op.type),
        )
      )
        throw new Error(
          "结构编辑需要在最新画布重新操作；可先复制完整保留内容。",
        );
      if (
        saved.some(
          (op) =>
            op.type === "set" &&
            ![
              "title",
              "z_index",
              "parent_id",
              "position",
              "size",
              "config.collapsed",
              "config.text_source",
            ].includes(op.path?.join(".") ?? ""),
        )
      )
        throw new Error(
          "模型、素材来源或工具参数冲突需在最新版本重新操作；可先复制保留内容。",
        );
      let current = session.base;
      const ops: Operation[] = [];
      const textDone = new Set<string>();
      for (const original of saved) {
        if (original.type === "noop") continue;
        const node = current.nodes.find((n) => n.id === original.node_id),
          old = branch.nodes.find((n) => n.id === original.node_id);
        if (!node || !old)
          throw new Error("目标节点已删除，请复制保留内容并新建节点。");
        if (original.type === "migrate_composer") {
          if (isDraft(node) && !node.config.draft.composer) {
            const op = {
              ...original,
              expected: structuredClone(node.config.draft),
            };
            current = applyOperations(current, [op]);
            ops.push(op);
          }
          continue;
        }
        const path = original.path!,
          rule = fieldPolicy(path);
        let op: Operation;
        if (original.type === "text") {
          const key = node.id + path.join(".");
          if (textDone.has(key)) continue;
          textDone.add(key);
          const value = fieldValue(old, path).value,
            now = fieldValue(node, path).value;
          const atoms =
            path.at(-1) === "document"
              ? promptSequence(value)
              : typeof value === "string" && value
                ? [{ type: "text" as const, text: value }]
                : [];
          const length =
            path.at(-1) === "document"
              ? sequenceLength(promptSequence(now))
              : String(now ?? "").length;
          const edit: SequenceEdit = [];
          if (atoms.length) edit.push({ insert: atoms });
          if (length) edit.push({ delete: length });
          op = { type: "text", node_id: node.id, path, text: edit };
        } else {
          const value = fieldValue(old, path, rule),
            now = fieldValue(node, path, rule);
          op = {
            type: "set",
            node_id: node.id,
            path,
            ...(value.exists ? { value: value.value } : { unset: true }),
            expected_exists: now.exists,
            ...(now.exists ? { expected: now.value } : {}),
          };
        }
        if (rule.context) op.context = fieldContext(node, path, rule);
        current = applyOperations(current, [op]);
        ops.push(op);
      }
      // The user explicitly reviewed full field replacements. Reinitializing this
      // recovery view is permitted; ordinary remote editing always uses PM steps.
      requireCanvas(current, initial.project_id, initial.id);
      recovery.current = undefined;
      resetEditors.current = true;
      session.discardRecovery();
      session.enqueue(ops);
      update.current(undefined, true);
      resetEditors.current = false;
      return session.flush();
    },
  };
}
