"use client";

import { useEffect, useLayoutEffect, useRef, useState } from "react";
import useSWR, { useSWRConfig } from "swr";
import { notifyError } from "@/components/canvas/runtime/components/feedback/error-notification";
import { ApiError, unwrap } from "@/components/canvas/runtime/lib/request";
import * as api from "@/components/canvas/runtime/api/canvasDocumentBffController";
import { useAccount } from "@/components/canvas/runtime/components/account/account-provider";
import { qualifyCanvasCommand } from "../lib/canvas-command-scope";
export type CanvasLibraryItem = CanvasContract.CanvasSummaryVO & { id: string };
export type ProjectQuery = Required<
  Omit<CanvasContract.CanvasQueryRequest, "projectId">
>;
export const canvasLibraryItem = (
  canvas: CanvasContract.CanvasSummaryVO,
): CanvasLibraryItem => ({ ...canvas, id: String(canvas.canvasId) });
export type ProjectCommandTarget = Pick<
  CanvasLibraryItem,
  | "id"
  | "revision"
  | "title"
  | "projectBindingRevision"
  | "coverFileId"
  | "coverFileVersion"
>;
type UnqualifiedCanvasVersion<T> = Omit<
  T,
  "expectedEpoch" | "expectedCommitSeq"
> & {
  expectedEpoch?: string;
  expectedCommitSeq?: string;
};
export type ProjectCommand =
  | { kind: "create"; body: Required<CanvasContract.CanvasAddRequest> }
  | {
      kind: "rename";
      body: UnqualifiedCanvasVersion<CanvasContract.CanvasEditRequest>;
    }
  | {
      kind: "delete";
      body: UnqualifiedCanvasVersion<
        Required<CanvasContract.CanvasDeleteRequest>
      >;
    };

// Capture the command ID and version once for this user action; business validation is server-side.
export function commandFor(
  kind: "create" | "rename" | "delete",
  project?: ProjectCommandTarget,
  title = "",
): ProjectCommand {
  const commandId = crypto.randomUUID();
  if (kind === "create") return { kind, body: { commandId, title } };
  if (!project) throw new Error("请先读取项目");
  return kind === "rename"
    ? {
        kind,
        body: {
          commandId,
          canvasId: project.id,
          expectedRevision: project.revision,
          expectedProjectBindingRevision: project.projectBindingRevision,
          title,
        },
      }
    : {
        kind,
        body: {
          commandId,
          id: project.id,
          expectedRevision: project.revision,
          expectedProjectBindingRevision: project.projectBindingRevision,
        },
      };
}

function notifyReadError(error: Error) {
  notifyError(error, "项目读取失败");
}

export function useProjects(query: ProjectQuery) {
  const { account, ready, signingOut } = useAccount();
  return useSWR(
    ready && account && !signingOut
      ? [
          "canvas-library",
          String(account.id),
          query.current,
          query.pageSize,
          query.keyword,
          query.sortField,
          query.sortOrder,
        ]
      : null,
    async () => {
      const page = unwrap(await api.listCanvases(query));
      return {
        ...page,
        pages: Math.max(1, Math.ceil(Number(page.total) / Number(page.size))),
        records: page.records.map(canvasLibraryItem),
      };
    },
    {
      shouldRetryOnError: false,
      keepPreviousData: false,
      onError: notifyReadError,
    },
  );
}
export function useProjectCommand() {
  const { mutate } = useSWRConfig();
  const { account, ready, signingOut } = useAccount();
  const actor = ready && account && !signingOut ? String(account.id) : null;
  const liveActor = useRef(actor);
  useLayoutEffect(() => {
    liveActor.current = actor;
  }, [actor]);
  const commandActor = useRef<string | null>(null);
  const lock = useRef(false);
  const pending = useRef<ProjectCommand | null>(null);
  const [busy, setBusy] = useState(false);
  const [unknown, setUnknown] = useState(false);
  const [error, setError] = useState<ApiError | null>(null);
  useEffect(() => {
    if (!busy && !unknown) return;
    const protect = (event: BeforeUnloadEvent) => {
      event.preventDefault();
      event.returnValue = "";
    };
    window.addEventListener("beforeunload", protect);
    const protectNavigation = (event: MouseEvent) => {
      if ((event.target as HTMLElement).closest("a[href]")) {
        event.preventDefault();
        event.stopPropagation();
        const failure = new ApiError(
          "请先查询当前项目操作结果，再离开页面",
          50300,
        );
        setError(failure);
        notifyError(failure, "项目操作待确认");
      }
    };
    document.addEventListener("click", protectNavigation, true);
    return () => {
      window.removeEventListener("beforeunload", protect);
      document.removeEventListener("click", protectNavigation, true);
    };
  }, [busy, unknown]);

  const findReceipt = async (command: ProjectCommand) => {
    try {
      return unwrap(
        await api.getCanvasCommand({ commandId: command.body.commandId }),
      ).result;
    } catch (failure) {
      if (failure instanceof ApiError && failure.code === 40400) return null;
      throw failure;
    }
  };
  const run = async (command?: ProjectCommand, queryOnly = false) => {
    if (lock.current) return null;
    if (!actor || (pending.current && commandActor.current !== actor)) {
      notifyError(
        new ApiError("请使用发起操作的账户登录后继续", 40100),
        "画布操作已暂停",
      );
      return null;
    }
    if (!pending.current) commandActor.current = actor;
    // An unresolved write can only be queried or retried with its frozen payload.
    if (!pending.current) pending.current = command ?? null;
    const operation = pending.current;
    if (!operation) return null;
    lock.current = true;
    setBusy(true);
    setError(null);
    try {
      let result = unknown || queryOnly ? await findReceipt(operation) : null;
      if (!result && !queryOnly) {
        try {
          const mayWrite = () => liveActor.current === commandActor.current;
          let response: Awaited<ReturnType<typeof api.addCanvas>>;
          if (operation.kind === "create")
            response = await api.addCanvas(operation.body);
          else if (operation.kind === "rename") {
            const body = await qualifyCanvasCommand(operation.body, mayWrite);
            operation.body = body;
            if (!mayWrite())
              throw new ApiError("账户已变化，请重新验证", 40100);
            response = await api.editCanvas(body);
          } else {
            const body = await qualifyCanvasCommand(operation.body, mayWrite);
            operation.body = body;
            if (!mayWrite())
              throw new ApiError("账户已变化，请重新验证", 40100);
            response = await api.deleteCanvas(body);
          }
          result = unwrap(response);
        } catch (failure) {
          if (failure instanceof ApiError && failure.code < 50000)
            throw failure;
          result = await findReceipt(operation);
          if (!result) throw failure;
        }
      }
      if (!result)
        throw new ApiError(
          "尚未查到操作结果。可继续查询，或重试原操作；请保留当前页面。",
          50300,
        );
      if (liveActor.current !== commandActor.current) {
        setUnknown(true);
        return null;
      }
      pending.current = null;
      setUnknown(false);
      if (operation.kind === "delete") {
        void mutate(
          (key) =>
            Array.isArray(key) &&
            key[0] === "canvas-document" &&
            key[1] === `USER:${commandActor.current}` &&
            String(key[3]) === String(result.canvasId),
          undefined,
          { revalidate: false },
        ).catch(() => undefined);
      }
      // Receipts are original snapshots; reread instead of installing an old snapshot as current state.
      void mutate(
        (key) =>
          Array.isArray(key) &&
          ((key[0] === "canvas-library" && key[1] === commandActor.current) ||
            (key[0] === "project-canvases" &&
              key[1] === `USER:${commandActor.current}`) ||
            (operation.kind !== "delete" &&
              key[0] === "canvas-document" &&
              key[1] === `USER:${commandActor.current}` &&
              String(key[3]) === String(result.canvasId))),
      ).catch(() => undefined);
      return { ...result, id: String(result.canvasId) };
    } catch (failure) {
      const apiError =
        failure instanceof ApiError
          ? failure
          : new ApiError("服务连接失败，请查询原操作结果", 50300);
      const uncertain = apiError.code >= 50000;
      if (!uncertain) pending.current = null;
      setUnknown(uncertain);
      setError(apiError);
      notifyError(apiError, uncertain ? "项目操作待确认" : "项目操作未完成");
      return null;
    } finally {
      lock.current = false;
      setBusy(false);
    }
  };
  const clearError = () => {
    if (!pending.current) setError(null);
  };
  return { busy, unknown, error, run, clearError };
}
