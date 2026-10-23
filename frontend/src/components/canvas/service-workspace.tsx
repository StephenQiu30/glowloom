"use client";
import { useEffect, useRef, useState } from "react";
import Link from "next/link";
import { useSearchParams } from "next/navigation";
import { getMediaPreview } from "@/api/media";
import { getCanvas } from "@/api/canvases";
import { useOptionalSession, SessionGate } from "@/components/auth/session";
import { useSpace, SpaceGate } from "@/components/spaces/space-context";
import { Button } from "@/components/ui/button";
import { isProjectID } from "@/components/projects/project";
import { getCSRFToken, RequestError } from "@/lib/request";
import { requirePreview } from "@/components/media/library";
import { CanvasEditor } from "./editor";
import { CanvasLiveSession, type LiveStatus } from "./live-session";
import { fromServiceDocument, graphOperations } from "./service-document";
import { downloadFile } from "./document";
import type { CanvasDocument } from "./types";

function Workspace({
  canvasID,
  projectID,
  actorID,
  scopeID,
}: {
  canvasID: string;
  projectID: string;
  actorID: string;
  scopeID: string;
}) {
  const [document, setDocument] = useState<CanvasDocument | null>(null);
  const [name, setName] = useState("画布");
  const [status, setStatus] = useState<LiveStatus>("connecting");
  const [message, setMessage] = useState("");
  const [attempt, setAttempt] = useState(0);
  const [history, setHistory] = useState<{ undo: string[]; redo: string[] }>({
    undo: [],
    redo: [],
  });
  const client = useRef<CanvasLiveSession | null>(null);
  const dto = useRef<API.DocumentDTO | null>(null);
  const preview = useRef(new Map<string, { url: string; expires: number }>());
  const recovery = useRef<CanvasDocument | null>(null);
  const map = (value: API.DocumentDTO) => {
    const doc = fromServiceDocument(value, canvasID, projectID);
    doc.nodes = doc.nodes.map((node) =>
      node.metadata?.assetId
        ? {
            ...node,
            metadata: {
              ...node.metadata,
              content: preview.current.get(node.metadata.assetId)?.url ?? "",
            },
          }
        : node,
    );
    return doc;
  };
  useEffect(() => {
    let alive = true;
    const requests = new AbortController();
    const previews = new Set<string>();
    function snapshot(value: API.DocumentDTO) {
      if (!alive) return;
      dto.current = value;
      setName(value.name || "未命名画布");
      const mapped = map(value);
      setDocument(mapped);
      for (const node of mapped.nodes) {
        const id = node.metadata?.assetId;
        if (
          !id ||
          previews.has(id) ||
          (preview.current.get(id)?.expires ?? 0) > Date.now() + 30000
        )
          continue;
        previews.add(id);
        void getMediaPreview(
          { pid: projectID, asset_id: id },
          { signal: requests.signal },
        )
          .then((result) => {
            const url = requirePreview(result, id);
            if (!alive) return;
            preview.current.set(id, {
              url,
              expires: Date.parse(result.expires_at!),
            });
            if (dto.current) setDocument(map(dto.current));
          })
          .catch(() => {
            if (alive) setMessage("部分素材预览暂不可用，可重新连接刷新");
          })
          .finally(() => previews.delete(id));
      }
    }
    const session = new CanvasLiveSession({
      canvasID,
      actorID,
      scopeID,
      csrf: getCSRFToken,
      snapshot,
      status: (next, reason) => {
        if (alive) {
          setStatus(next);
          setMessage(reason ?? "");
        }
      },
    });
    client.current = session;
    void getCanvas({ id: canvasID }, { signal: requests.signal })
      .then((value) => {
        if (!alive) return;
        fromServiceDocument(value, canvasID, projectID);
        session.start();
      })
      .catch((error: unknown) => {
        if (!alive) return;
        setStatus("error");
        setMessage(
          error instanceof RequestError
            ? `无法读取画布：${error.code}`
            : error instanceof Error
              ? error.message
              : "无法读取画布",
        );
      });
    return () => {
      alive = false;
      requests.abort();
      session.stop();
      client.current = null;
    };
    // Each identity tuple owns its connection and media cache; remounted by the parent.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [canvasID, projectID, actorID, scopeID, attempt]);
  const labels: Record<LiveStatus, string> = {
    connecting: "正在连接",
    ready: "已保存",
    saving: "正在保存",
    unconfirmed: "保存结果未确认",
    readonly: "只读",
    error: "暂时无法打开",
  };
  return (
    <div className="relative h-dvh">
      {document && (
        <CanvasEditor
          service={{
            document,
            name,
            projectID,
            status: labels[status],
            writable: status === "ready",
            undo: history.undo.length > 0,
            redo: history.redo.length > 0,
            commit: async (before, after) => {
              const operations = graphOperations(before, after);
              if (!operations.length) return before;
              recovery.current = after;
              const op = await client.current!.submit(
                operations,
                undefined,
                before.serviceRevision,
              );
              setHistory((h) => ({
                undo: [...h.undo.slice(-49), op],
                redo: [],
              }));
              recovery.current = null;
              return map(dto.current!);
            },
            history: async (direction) => {
              const original = history[direction].at(-1);
              if (!original) return document;
              const op = await client.current!.submit([], original);
              setHistory((h) =>
                direction === "undo"
                  ? { undo: h.undo.slice(0, -1), redo: [...h.redo, op] }
                  : { undo: [...h.undo, op], redo: h.redo.slice(0, -1) },
              );
              return map(dto.current!);
            },
          }}
        />
      )}
      {!document && (
        <div className="flex h-full flex-col items-center justify-center gap-4 p-6">
          <h1 className="text-xl">{labels[status]}</h1>
          <p className="text-sm text-muted-foreground">{message}</p>
          <Button asChild variant="secondary">
            <Link href="/projects">返回画布列表</Link>
          </Button>
          {status === "error" && (
            <Button onClick={() => setAttempt((n) => n + 1)}>重试</Button>
          )}
        </div>
      )}
      {document && (status !== "ready" || message) && (
        <div
          role="status"
          className="absolute top-24 left-1/2 z-50 flex max-w-[calc(100%-2rem)] -translate-x-1/2 flex-wrap items-center gap-2 rounded-lg bg-popover p-3 text-sm shadow-sm"
        >
          <span>{message || labels[status]}</span>
          {(status === "error" || status === "readonly") && (
            <Button asChild variant="ghost">
              <Link href="/projects">返回列表</Link>
            </Button>
          )}
          {status === "unconfirmed" && (
            <Button
              variant="secondary"
              onClick={() =>
                downloadFile(
                  new Blob([JSON.stringify(recovery.current ?? document)], {
                    type: "application/json",
                  }),
                  "canvas-recovery.json",
                )
              }
            >
              导出本页修改
            </Button>
          )}
          {status === "error" && (
            <Button
              variant="secondary"
              onClick={() => setAttempt((n) => n + 1)}
            >
              重试连接
            </Button>
          )}
        </div>
      )}
    </div>
  );
}
function Entry() {
  const search = useSearchParams();
  const session = useOptionalSession();
  const space = useSpace();
  const canvasID = search.get("canvas_id"),
    projectID = search.get("project_id");
  if (!isProjectID(canvasID) || !isProjectID(projectID))
    return (
      <div className="flex h-dvh flex-col items-center justify-center gap-4">
        <h1>请选择一张画布</h1>
        <Button asChild>
          <Link href="/projects">我的画布</Link>
        </Button>
      </div>
    );
  if (!session || !space.selected || space.pending) return null;
  return (
    <Workspace
      key={`${session.actor.id}/${space.selected.id}/${canvasID}`}
      canvasID={canvasID}
      projectID={projectID}
      actorID={session.actor.id}
      scopeID={space.selected.id}
    />
  );
}
export function ServiceCanvasWorkspace() {
  return (
    <SessionGate>
      <SpaceGate>
        <Entry />
      </SpaceGate>
    </SessionGate>
  );
}
