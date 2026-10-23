"use client";
import { useEffect, useRef, useState } from "react";
import Link from "next/link";
import { useSearchParams } from "next/navigation";
import { getCanvas } from "@/api/canvases";
import { useOptionalSession, SessionGate } from "@/components/auth/session";
import { useSpace, SpaceGate } from "@/components/spaces/space-context";
import { Button } from "@/components/ui/button";
import { isProjectID } from "@/components/projects/project";
import { getCSRFToken } from "@/lib/request";
import { ContentState } from "@/components/layout/content-state";
import { readErrorMessage } from "@/lib/read-error";
import { CanvasEditor } from "./editor";
import { ensureSessionFresh } from "@/components/auth/session-renewal";
import { canvasViewKey } from "./view-state";
import { CanvasLiveSession, type LiveStatus } from "./live-session";
import { serviceGraph } from "./service-graph";
import type { CanvasEditorState } from "./lib/canvas-document-adapter";

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
  const [snapshot, setSnapshot] = useState<{
    dto: API.DocumentDTO;
    graph: CanvasEditorState;
  } | null>(null);
  const [status, setStatus] = useState<LiveStatus>("connecting");
  const [message, setMessage] = useState("");
  const [attempt, setAttempt] = useState(0);
  const [history, setHistory] = useState<{ undo: string[]; redo: string[] }>({
    undo: [],
    redo: [],
  });
  const latest = useRef<{
    dto: API.DocumentDTO;
    graph: CanvasEditorState;
  } | null>(null);
  const client = useRef<CanvasLiveSession | null>(null);
  useEffect(() => {
    let active = true;
    const abort = new AbortController();
    const connection = new CanvasLiveSession({
      canvasID,
      actorID,
      scopeID,
      csrf: getCSRFToken,
      async renewSession(signal) {
        const session = await ensureSessionFresh(signal, true);
        if (session.actor.id !== actorID) throw new Error("画布账号已变更");
      },
      snapshot(dto) {
        if (active) {
          const value = { dto, graph: serviceGraph(dto, canvasID, projectID) };
          latest.current = value;
          setSnapshot(value);
        }
      },
      status(value, reason) {
        if (active) {
          setStatus(value);
          setMessage(reason ?? "");
        }
      },
    });
    client.current = connection;
    void getCanvas({ id: canvasID }, { signal: abort.signal })
      .then((dto) => {
        if (!active) return;
        const graph = serviceGraph(dto, canvasID, projectID);
        setSnapshot({ dto, graph });
        connection.start();
      })
      .catch((cause: unknown) => {
        if (active) {
          setStatus("error");
          setMessage(readErrorMessage(cause));
        }
      });
    return () => {
      active = false;
      abort.abort();
      connection.stop();
      client.current = null;
    };
  }, [canvasID, projectID, actorID, scopeID, attempt]);
  const labels: Record<LiveStatus, string> = {
    connecting: "正在连接",
    ready: "已保存",
    saving: "正在保存",
    unconfirmed: "保存结果未确认",
    readonly: "只读",
    error: "连接不可用",
  };
  async function navigateHistory(direction: "undo" | "redo") {
    const original = history[direction].at(-1);
    if (!original || !client.current) return;
    try {
      const id = await client.current.submit([], original);
      setHistory((current) =>
        direction === "undo"
          ? { undo: current.undo.slice(0, -1), redo: [...current.redo, id] }
          : { undo: [...current.undo, id], redo: current.redo.slice(0, -1) },
      );
    } catch (cause) {
      setMessage(cause instanceof Error ? cause.message : "历史操作未完成");
    }
  }
  return (
    <div className="relative h-dvh">
      {snapshot ? (
        <CanvasEditor
          key={`${canvasID}/${actorID}/${scopeID}`}
          service={{
            document: snapshot.graph,
            canvasID,
            projectID,
            revision: snapshot.dto.revision!,
            name: snapshot.dto.name ?? "未命名画布",
            status: labels[status],
            writable:
              status === "ready" && snapshot.dto.permissions?.write === true,
            viewStateKey: canvasViewKey(actorID, scopeID, projectID, canvasID),
            commit: async (operations, revision) => {
              if (!client.current) throw new Error("画布连接尚未就绪");
              const id = await client.current.submit(
                operations,
                undefined,
                revision,
              );
              setHistory((current) => ({
                undo: [...current.undo.slice(-49), id],
                redo: [],
              }));
              if (!latest.current) throw new Error("保存回执缺少画布快照");
              return {
                document: latest.current.graph,
                revision: latest.current.dto.revision!,
              };
            },
            undo: {
              canUndo: status === "ready" && history.undo.length > 0,
              canRedo: status === "ready" && history.redo.length > 0,
              undo: () => void navigateHistory("undo"),
              redo: () => void navigateHistory("redo"),
            },
          }}
        />
      ) : (
        <ContentState
          className="h-full"
          state={status === "error" ? "error" : "loading"}
          title={status === "error" ? "暂时无法打开画布" : "正在打开画布"}
          description={status === "error" ? message : "正在连接你的创作空间。"}
          actions={
            <>
              <Button asChild variant="secondary">
                <Link href="/projects">返回画布列表</Link>
              </Button>
              {status === "error" && (
                <Button
                  onClick={() => {
                    setStatus("connecting");
                    setMessage("");
                    setAttempt((value) => value + 1);
                  }}
                >
                  重试
                </Button>
              )}
            </>
          }
        />
      )}
      {snapshot &&
        (message || status === "unconfirmed" || status === "error") && (
          <div
            role="status"
            className="absolute bottom-16 left-1/2 z-50 flex max-w-lg -translate-x-1/2 items-center gap-2 rounded-lg bg-popover p-3 text-sm shadow-sm"
          >
            <span>{message || labels[status]}</span>
            <Button variant="secondary" onClick={() => client.current?.retry()}>
              重新连接并查证
            </Button>
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
      <ContentState
        className="h-dvh"
        title="请选择一张画布"
        description="从画布列表打开已有创作，或新建画布。"
        actions={
          <Button asChild>
            <Link href="/projects">我的画布</Link>
          </Button>
        }
      />
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
