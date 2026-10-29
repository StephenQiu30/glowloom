"use client";
import { useCallback, useEffect, useRef, useState } from "react";
import {
  readDirectorRecovery,
  saveDirectorRecovery,
  deleteDirectorRecovery,
} from "@/api/canvases";
import type { CanvasYjsLiveSession } from "../yjs-live-session";
import { directorFingerprint } from "./director-model";
import { validateDirectorDocument } from "./director-workspaces";

export async function directorRecoveryFingerprint(value: unknown) {
  const digest = await crypto.subtle.digest(
    "SHA-256",
    new TextEncoder().encode(directorFingerprint(value)),
  );
  return Array.from(new Uint8Array(digest), (byte) =>
    byte.toString(16).padStart(2, "0"),
  ).join("");
}
export type DirectorRecoveryFile = {
  version: 1;
  canvas_id: string;
  node_id: string;
  director: API.DirectorConfig;
};
export function parseDirectorRecoveryFile(
  text: string,
  canvasID: string,
  nodeID: string,
) {
  if (new TextEncoder().encode(text).length > 600_000)
    throw new Error("恢复文件过大");
  const data = JSON.parse(text) as DirectorRecoveryFile;
  if (
    data.version !== 1 ||
    data.canvas_id !== canvasID ||
    data.node_id !== nodeID
  )
    throw new Error("恢复文件不属于当前导演台");
  return validateDirectorDocument(data.director);
}
/** Private server backup is separate from authoritative Yjs acknowledgement. */
export function useDirectorRecovery(
  client: CanvasYjsLiveSession,
  canvasID: string,
  nodeID: string,
) {
  const [retained, setRetained] = useState<API.DirectorConfig | null>(() =>
    client.retainedDirector(nodeID),
  );
  const [status, setStatus] = useState("正在检查恢复稿");
  const [ready, setReady] = useState(false);
  const [owner, setOwner] = useState({ client, canvasID, nodeID });
  const state = useRef({
    canvasID,
    nodeID,
    client,
    active: true,
    revision: 0,
    generation: 0,
    queue: Promise.resolve(),
    loaded: Promise.resolve(),
  });
  useEffect(() => {
    const current = {
      canvasID,
      nodeID,
      client,
      active: true,
      revision: 0,
      generation: 0,
      queue: Promise.resolve(),
      loaded: Promise.resolve(),
    };
    state.current = current;
    current.loaded = readDirectorRecovery({ id: canvasID, nid: nodeID })
      .then((result) => {
        if (!current.active) return;
        current.revision = result.draft?.revision ?? 0;
        if (result.draft) {
          const draft = validateDirectorDocument(result.draft.director);
          setRetained((local) =>
            current.generation
              ? local
              : (client.retainedDirector(nodeID) ?? draft),
          );
          setStatus("发现未提交的恢复稿，保留30天");
        } else {
          if (!current.generation) setRetained(client.retainedDirector(nodeID));
          setStatus("");
        }
      })
      .catch(() => {
        if (current.active)
          setStatus("恢复稿暂时无法读取；请稍后重试，离开前可下载恢复文件");
      })
      .finally(() => {
        if (current.active) {
          setOwner({ client, canvasID, nodeID });
          setReady(true);
        }
      });
    return () => {
      current.active = false;
      current.generation++;
    };
  }, [client, canvasID, nodeID]);
  const retain = useCallback(
    (
      next: API.DirectorConfig,
      baseline: API.DirectorConfig,
      epoch: string | null,
    ) => {
      const current = state.current;
      if (
        !current.active ||
        current.client !== client ||
        current.canvasID !== canvasID ||
        current.nodeID !== nodeID
      )
        return;
      const generation = ++current.generation;
      client.retainDirector(nodeID, next);
      setRetained(next);
      setStatus("正在备份未提交编辑");
      current.queue = current.queue
        .catch(() => {})
        .then(async () => {
          await current.loaded;
          if (!current.active || current.generation !== generation) return;
          if (!epoch) throw new Error("画布版本未确认");
          const fingerprint = await directorRecoveryFingerprint(baseline);
          if (!current.active || current.generation !== generation) return;
          const result = await saveDirectorRecovery(
            { id: canvasID, nid: nodeID },
            {
              expected_revision: current.revision,
              epoch,
              base_fingerprint: fingerprint,
              director: next,
            },
          );
          current.revision = result.revision ?? 0;
          if (current.active && current.generation === generation)
            setStatus("恢复稿已备份，刷新后可继续处理（保留30天）");
        })
        .catch(() => {
          if (current.active && current.generation === generation)
            setStatus(
              "恢复稿尚未备份，请下载恢复文件后再离开；可能存在较新恢复稿或连接中断",
            );
        });
    },
    [client, canvasID, nodeID],
  );
  const discard = useCallback(async () => {
    const current = state.current;
    if (
      !current.active ||
      current.client !== client ||
      current.canvasID !== canvasID ||
      current.nodeID !== nodeID
    )
      return;
    const generation = ++current.generation;
    await current.loaded;
    await current.queue;
    if (
      !current.active ||
      state.current !== current ||
      current.generation !== generation
    )
      return;
    const removedRevision = current.revision;
    if (removedRevision)
      await deleteDirectorRecovery(
        { id: canvasID, nid: nodeID },
        { expected_revision: removedRevision },
      );
    if (current.revision === removedRevision) current.revision = 0;
    if (current.active && current.generation === generation) {
      client.discardRetainedDirector(nodeID);
      setRetained(null);
      setStatus("");
    }
  }, [client, canvasID, nodeID]);
  const owns =
    owner.client === client &&
    owner.canvasID === canvasID &&
    owner.nodeID === nodeID;
  return {
    retained: owns ? retained : null,
    setRetained,
    status: owns ? status : "正在检查恢复稿",
    ready: owns && ready,
    retain,
    discard,
  };
}
