"use client";
import { useEffect, useRef, useState } from "react";
import { getWorkspace, requireScriptScope } from "./source-queries";
import { unknownSourceWrite, type ScriptScope } from "./source-intent";
import {
  clearDocumentUpload,
  documentFileFingerprint,
  loadDocumentUpload,
  requireOriginalDocument,
  saveDocumentUpload,
  type DocumentUploadIntent,
} from "./document-upload-intent";
import { uploadDocument, type DocumentAsset } from "./document-media";

export function useDocumentUpload(
  scope: ScriptScope,
  onUploaded: (asset: DocumentAsset) => void | Promise<void>,
) {
  const [intent, setIntent] = useState<DocumentUploadIntent | null>(null);
  const pending = useRef<DocumentUploadIntent | null>(null);
  const [ready, setReady] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string>();
  const [storageError, setStorageError] = useState<string>();
  const [loaded, setLoaded] = useState(0);
  const [total, setTotal] = useState<number>();
  const [uploaded, setUploaded] = useState<DocumentAsset>();
  const active = useRef<AbortController | null>(null);
  const running = useRef(false);
  const mounted = useRef(false);
  const completion = useRef(onUploaded);
  useEffect(() => {
    completion.current = onUploaded;
  }, [onUploaded]);
  const { projectId, origin, actorId, orgId } = scope;
  useEffect(() => {
    mounted.current = true;
    const controller = new AbortController();
    void getWorkspace(projectId, controller.signal)
      .then((head) => {
        requireScriptScope(head, { projectId, origin, actorId, orgId });
        const original = loadDocumentUpload(sessionStorage, {
          projectId,
          origin,
          actorId,
          orgId,
        });
        if (!mounted.current || controller.signal.aborted) return;
        pending.current = original;
        setIntent(original);
        setReady(true);
      })
      .catch((cause) => {
        if (mounted.current && !controller.signal.aborted)
          setStorageError(
            cause instanceof Error ? cause.message : "原文上传意图读取失败。",
          );
      });
    return () => {
      mounted.current = false;
      controller.abort();
      active.current?.abort();
    };
  }, [projectId, origin, actorId, orgId]);
  const locked = busy || Boolean(intent || storageError) || !ready;
  useEffect(() => {
    if (!locked) return;
    const warn = (event: BeforeUnloadEvent) => {
      event.preventDefault();
      event.returnValue = "";
    };
    window.addEventListener("beforeunload", warn);
    return () => window.removeEventListener("beforeunload", warn);
  }, [locked]);
  function retain(original: DocumentUploadIntent | null) {
    pending.current = original;
    if (mounted.current) setIntent(original);
  }
  async function fresh(signal?: AbortSignal) {
    requireScriptScope(await getWorkspace(scope.projectId, signal), scope);
  }
  async function restoreStorage() {
    if (running.current) return;
    running.current = true;
    setBusy(true);
    try {
      await fresh();
      const stored = loadDocumentUpload(sessionStorage, scope);
      if (stored && pending.current && stored.key !== pending.current.key)
        throw new Error("存储中的原上传与本页原键不同，请保留两份事实后核验。");
      const original = pending.current ?? stored;
      if (original) saveDocumentUpload(sessionStorage, original);
      retain(original);
      setStorageError(undefined);
      setError(undefined);
      setReady(true);
    } catch (cause) {
      setStorageError(cause instanceof Error ? cause.message : "存储未恢复。");
    } finally {
      running.current = false;
      if (mounted.current) setBusy(false);
    }
  }
  async function run(file: File | null, replay: boolean) {
    if (
      running.current ||
      storageError ||
      !ready ||
      (!replay && pending.current)
    )
      return;
    running.current = true;
    setBusy(true);
    setError(undefined);
    setLoaded(0);
    setTotal(undefined);
    const controller = new AbortController();
    active.current = controller;
    let sent = false;
    try {
      if (!file)
        throw new Error("请重新选择确切原文件；原键已保留，不会提交其他文件。");
      await fresh(controller.signal);
      let original = pending.current;
      if (replay) {
        if (!original) throw new Error("没有可恢复的原文上传意图。");
        await requireOriginalDocument(file, original);
        const stored = loadDocumentUpload(sessionStorage, scope);
        if (!stored || JSON.stringify(stored) !== JSON.stringify(original))
          throw new Error("浏览器存储与原上传不一致，请先恢复存储。");
      } else {
        if (loadDocumentUpload(sessionStorage, scope))
          throw new Error("已有未确认的原上传，请先恢复原键。");
        original = {
          ...scope,
          version: 1,
          key: crypto.randomUUID(),
          ...(await documentFileFingerprint(file)),
          localReviewConfirmed: true,
        };
        retain(original);
        try {
          saveDocumentUpload(sessionStorage, original);
        } catch (cause) {
          setStorageError(
            cause instanceof Error ? cause.message : "原上传存储失败。",
          );
          throw cause;
        }
      }
      sent = true;
      const asset = await uploadDocument(
        scope.projectId,
        file,
        original.key,
        scope.origin,
        controller.signal,
        (bytes, size) => {
          if (mounted.current) {
            setLoaded(bytes);
            setTotal(size);
          }
        },
      );
      clearDocumentUpload(sessionStorage, scope);
      retain(null);
      if (mounted.current) {
        setUploaded(asset);
        try {
          await completion.current(asset);
        } catch {
          setError("原件上传已确认，文档列表刷新未完成。请重新读取文档列表。");
        }
      }
    } catch (cause) {
      if (sent && !replay && !unknownSourceWrite(cause)) {
        try {
          clearDocumentUpload(sessionStorage, scope);
          retain(null);
        } catch (problem) {
          if (mounted.current)
            setStorageError(
              problem instanceof Error
                ? problem.message
                : "原上传存储清理失败。",
            );
        }
      }
      if (mounted.current)
        setError(
          cause instanceof Error
            ? cause.message
            : "上传尚未确认，请使用原键核验。",
        );
    } finally {
      active.current = null;
      running.current = false;
      if (mounted.current) setBusy(false);
    }
  }
  return {
    ready,
    busy,
    locked,
    intent,
    error,
    storageError,
    loaded,
    total,
    uploaded,
    submit: (file: File) => run(file, false),
    replay: (file: File | null) => run(file, true),
    restoreStorage,
    stopWaiting: () => active.current?.abort(),
  };
}
