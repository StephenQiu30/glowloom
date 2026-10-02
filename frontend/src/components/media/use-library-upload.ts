"use client";
import { useEffect, useRef, useState } from "react";
import { ApiError } from "@/lib/request";
import { type LibraryAsset, type LibraryIdentity } from "./library-model";
import { freshLibrary } from "./library-queries";
import { unknownLibraryWrite } from "./library-intent";
import {
  loadLibraryUploads,
  requireOriginalUpload,
  saveLibraryUploads,
  uploadFingerprint,
  validateLibraryFiles,
  type LibraryUploadIntent,
} from "./library-upload-intent";
import { uploadLibraryOriginal } from "./library-upload-query";

type Entry = {
  intent: LibraryUploadIntent;
  status: "pending" | "uploading" | "unknown" | "rejected" | "confirmed";
  loaded: number;
  total?: number;
  error?: string;
  asset?: LibraryAsset;
};
export function useLibraryUpload(
  identity: LibraryIdentity,
  onUploaded: () => void | Promise<void>,
) {
  const [entries, setEntries] = useState<Entry[]>([]),
    [ready, setReady] = useState(false),
    [busy, setBusy] = useState(false),
    [error, setError] = useState<string>(),
    [storageError, setStorageError] = useState<string>();
  const journal = useRef<LibraryUploadIntent[]>([]),
    mounted = useRef(false),
    running = useRef(false),
    stopping = useRef(false),
    controllers = useRef(new Map<string, AbortController>()),
    completion = useRef(onUploaded),
    snapshot = useRef<Entry[]>([]);
  useEffect(() => {
    completion.current = onUploaded;
  }, [onUploaded]);
  const { origin, actorId, orgId, libraryId, scope } = identity,
    kind = scope.kind,
    pid = scope.kind === "project" ? scope.project_id : undefined;
  function updateEntries(update: (old: Entry[]) => Entry[]) {
    snapshot.current = update(snapshot.current);
    if (mounted.current) setEntries(snapshot.current);
  }
  useEffect(() => {
    mounted.current = true;
    const controller = new AbortController();
    const current: LibraryIdentity = {
      origin,
      actorId,
      orgId,
      libraryId,
      scope: kind === "project" ? { kind, project_id: pid! } : { kind },
    };
    void freshLibrary(current, controller.signal)
      .then(() => {
        const stored = loadLibraryUploads(sessionStorage, current);
        if (!controller.signal.aborted) {
          journal.current = stored;
          snapshot.current = stored.map((intent) => ({
            intent,
            status: "unknown",
            loaded: 0,
          }));
          setEntries(snapshot.current);
          setReady(true);
        }
      })
      .catch((cause) => {
        if (!controller.signal.aborted) {
          setStorageError(
            cause instanceof Error ? cause.message : "原上传意图读取失败。",
          );
          setReady(true);
        }
      });
    const active = controllers.current;
    return () => {
      mounted.current = false;
      controller.abort();
      active.forEach((owner) => owner.abort());
    };
  }, [origin, actorId, orgId, libraryId, kind, pid]);
  const locked =
    busy ||
    !ready ||
    Boolean(storageError) ||
    entries.some((entry) => entry.status !== "confirmed");
  useEffect(() => {
    if (!locked) return;
    const warn = (event: BeforeUnloadEvent) => {
      event.preventDefault();
      event.returnValue = "";
    };
    window.addEventListener("beforeunload", warn);
    return () => window.removeEventListener("beforeunload", warn);
  }, [locked]);
  function mark(key: string, fields: Partial<Entry>) {
    updateEntries((old) =>
      old.map((entry) =>
        entry.intent.key === key ? { ...entry, ...fields } : entry,
      ),
    );
  }
  function persist(next: LibraryUploadIntent[]) {
    saveLibraryUploads(sessionStorage, identity, next);
    journal.current = next;
  }
  async function send(
    intent: LibraryUploadIntent,
    file: File,
    replay: boolean,
  ) {
    const controller = new AbortController();
    controllers.current.set(intent.key, controller);
    let sent = false;
    mark(intent.key, { status: "uploading", loaded: 0, error: undefined });
    try {
      await freshLibrary(identity, controller.signal);
      if (replay) await requireOriginalUpload(file, intent);
      const stored = loadLibraryUploads(sessionStorage, identity).find(
        (original) => original.key === intent.key,
      );
      if (!stored || JSON.stringify(stored) !== JSON.stringify(intent))
        throw new Error("原上传存储与本页原键不一致，请先恢复存储。");
      if (controller.signal.aborted)
        throw new Error("本地发送已中止；请核验原键，服务端结果尚未确认。");
      sent = true;
      const asset = await uploadLibraryOriginal(
        identity,
        file,
        intent,
        controller.signal,
        (loaded, total) => mark(intent.key, { loaded, total }),
      );
      persist(
        journal.current.filter((original) => original.key !== intent.key),
      );
      mark(intent.key, { status: "confirmed", asset, error: undefined });
      try {
        await completion.current();
      } catch {
        mark(intent.key, {
          error: "原上传已确认，但当前列表尚未刷新，请重新读取。",
        });
      }
    } catch (cause) {
      const rejected =
        sent &&
        !unknownLibraryWrite(cause) &&
        !(
          replay &&
          cause instanceof ApiError &&
          [401, 403, 404].includes(cause.status)
        );
      mark(intent.key, {
        status: rejected ? "rejected" : "unknown",
        error:
          cause instanceof Error
            ? cause.message
            : "服务端结果尚未确认，请核验原键。",
      });
    } finally {
      controllers.current.delete(intent.key);
    }
  }
  async function start(files: File[]) {
    if (running.current || journal.current.length || storageError || !ready)
      return;
    running.current = true;
    stopping.current = false;
    setBusy(true);
    setError(undefined);
    try {
      const validation = validateLibraryFiles(files);
      if (!validation.valid) throw new Error(validation.errors.join(" "));
      await freshLibrary(identity);
      if (loadLibraryUploads(sessionStorage, identity).length)
        throw new Error("已有未确认的原上传，请先恢复原键。");
      const batch: LibraryUploadIntent[] = [];
      for (const file of files)
        batch.push({
          key: crypto.randomUUID(),
          ...(await uploadFingerprint(file)),
          local_review_confirmed: true,
        });
      if (!mounted.current) return;
      journal.current = batch;
      updateEntries(() =>
        batch.map((intent) => ({ intent, status: "pending", loaded: 0 })),
      );
      try {
        persist(batch);
      } catch (cause) {
        setStorageError(
          cause instanceof Error ? cause.message : "原上传意图无法保存。",
        );
        return;
      }
      let next = 0;
      await Promise.all(
        Array.from({ length: Math.min(4, files.length) }, async () => {
          while (next < files.length && !stopping.current && mounted.current) {
            const index = next++;
            await send(batch[index], files[index], false);
          }
        }),
      );
      if (stopping.current)
        updateEntries((old) =>
          old.map((entry) =>
            entry.status === "pending"
              ? {
                  ...entry,
                  status: "unknown",
                  error: "此原键尚未得到确认，请人工核验。",
                }
              : entry,
          ),
        );
    } catch (cause) {
      if (mounted.current)
        setError(cause instanceof Error ? cause.message : "上传未完成。");
    } finally {
      running.current = false;
      if (mounted.current) setBusy(false);
    }
  }
  async function replay(key: string, file: File | null) {
    if (running.current || storageError || !ready) return;
    const original = journal.current.find((entry) => entry.key === key);
    if (!original) return;
    if (!file) {
      setError("请重新选择确切原文件；当前不会发送其他文件。");
      return;
    }
    running.current = true;
    setBusy(true);
    setError(undefined);
    try {
      await send(original, file, true);
    } finally {
      running.current = false;
      if (mounted.current) setBusy(false);
    }
  }
  async function restoreStorage() {
    if (running.current) return;
    running.current = true;
    setBusy(true);
    try {
      await freshLibrary(identity);
      const stored = loadLibraryUploads(sessionStorage, identity);
      if (
        stored.some(
          (original) =>
            !journal.current.some((entry) => entry.key === original.key),
        ) &&
        journal.current.length
      )
        throw new Error("存储存在另一份原键，当前不能覆盖。");
      if (!journal.current.length) {
        journal.current = stored;
        updateEntries(() =>
          stored.map((intent) => ({ intent, status: "unknown", loaded: 0 })),
        );
      }
      persist(journal.current);
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
  async function discardRejected(key: string) {
    if (
      running.current ||
      snapshot.current.find((entry) => entry.intent.key === key)?.status !==
        "rejected"
    )
      return;
    running.current = true;
    setBusy(true);
    try {
      await freshLibrary(identity);
      persist(journal.current.filter((entry) => entry.key !== key));
      updateEntries((old) => old.filter((entry) => entry.intent.key !== key));
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "原键未能释放。");
    } finally {
      running.current = false;
      if (mounted.current) setBusy(false);
    }
  }
  function stopSending() {
    stopping.current = true;
    controllers.current.forEach((controller) => controller.abort());
  }
  return {
    entries,
    ready,
    busy,
    locked,
    error,
    storageError,
    start,
    replay,
    restoreStorage,
    discardRejected,
    stopSending,
  };
}
