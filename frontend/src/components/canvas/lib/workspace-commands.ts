import * as projects from "@/components/canvas/runtime/api/personalWorkspaceBffController";
import * as canvases from "@/components/canvas/runtime/api/canvasDocumentBffController";
import { ApiError, unwrap } from "@/components/canvas/runtime/lib/request";
import type { WorkspaceResourceTarget } from "./workspace-resources";
import type { ScopedMutator } from "swr";
import { qualifyCanvasCommand } from "./canvas-command-scope";

export type WorkspaceCommand = {
  target: WorkspaceResourceTarget;
  operation: "rename" | "delete";
  commandId: string;
  title?: string;
  coverFileId?: string;
  coverFileVersion?: string;
};

/** Use the originating private cache, even while the action host survives a route change. */
export function invalidateWorkspaceResource(
  mutate: ScopedMutator,
  identity: string,
  { target, operation }: WorkspaceCommand,
) {
  const actor = identity.slice("USER:".length);
  void mutate(
    (key) =>
      Array.isArray(key) &&
      key[1] === identity &&
      ((key[0] === "personal-projects" &&
        target.kind === "project" &&
        key[2] === (target.parentId ?? "root")) ||
        (key[0] === "project-canvases" &&
          target.kind === "canvas" &&
          key[2] === (target.parentId ?? "root")) ||
        (key[0] === "personal-project-workspace" &&
          target.kind === "project" &&
          ((String(key[2]) === target.id && operation !== "delete") ||
            (target.parentId != null && String(key[2]) === target.parentId))) ||
        (key[0] === "project-canvas-sources" && target.kind === "canvas") ||
        (key[0] === "canvas-document" &&
          target.kind === "canvas" &&
          String(key[3]) === target.id &&
          operation !== "delete")),
  ).catch(() => undefined);
  void mutate(
    (key) =>
      Array.isArray(key) &&
      key[1] === actor &&
      ((key[0] === "home-root-projects" &&
        target.kind === "project" &&
        !target.parentId) ||
        (key[0] === "canvas-library" && target.kind === "canvas")),
  ).catch(() => undefined);
  if (operation === "delete")
    void mutate(
      (key) =>
        Array.isArray(key) &&
        key[1] === identity &&
        ((key[0] === "personal-project-workspace" &&
          target.kind === "project" &&
          String(key[2]) === target.id) ||
          (key[0] === "canvas-document" &&
            target.kind === "canvas" &&
            String(key[3]) === target.id)),
      undefined,
      { revalidate: false },
    ).catch(() => undefined);
}

export function workspaceCommand(
  target: WorkspaceResourceTarget,
  operation: WorkspaceCommand["operation"],
  title?: string,
  cover?: { coverFileId?: string; coverFileVersion?: string },
): WorkspaceCommand {
  return {
    target: { ...target },
    operation,
    commandId: crypto.randomUUID(),
    title,
    ...cover,
  };
}

export async function readWorkspaceTarget(
  target: WorkspaceResourceTarget,
): Promise<WorkspaceResourceTarget> {
  if (target.kind === "project") {
    const value = unwrap(
      await projects.getPersonalProjectWorkspace({ projectId: target.id }),
    );
    if (String(value.projectId) !== target.id)
      throw new ApiError("项目读取结果不匹配", 50300);
    return {
      kind: "project",
      id: String(value.projectId),
      title: value.title,
      coverFileId: value.coverFileId,
      coverFileVersion: value.coverFileVersion,
      revision: value.revision,
      parentId:
        value.parentProjectId == null
          ? undefined
          : String(value.parentProjectId),
    };
  }
  const value = unwrap(
    await canvases.getCanvasDocument({ canvasId: target.id }),
  );
  if (String(value.canvasId) !== target.id)
    throw new ApiError("画布读取结果不匹配", 50300);
  return {
    kind: "canvas",
    id: String(value.canvasId),
    title: value.title,
    coverFileId: value.coverFileId,
    coverFileVersion: value.coverFileVersion,
    revision: value.revision,
    projectBindingRevision: value.projectBindingRevision,
    epoch: value.epoch,
    commitSeq: value.commitSeq,
    parentId: value.projectId == null ? undefined : String(value.projectId),
  };
}

async function receipt(command: WorkspaceCommand) {
  try {
    return command.target.kind === "project"
      ? unwrap(
          await projects.getPersonalProjectCommand({
            commandId: command.commandId,
          }),
        )
      : unwrap(
          await canvases.getCanvasCommand({ commandId: command.commandId }),
        ).result;
  } catch (error) {
    if (error instanceof ApiError && error.code === 40400) return null;
    // A receipt read failure cannot prove that an earlier write was rejected.
    throw new ApiError("暂时无法核对原操作结果，请恢复访问后继续", 50300);
  }
}

async function write(command: WorkspaceCommand, mayWrite: () => boolean) {
  const { target, operation, commandId } = command;
  const base = { commandId, expectedRevision: target.revision };
  const cover =
    command.coverFileId == null
      ? {}
      : {
          coverFileId: command.coverFileId,
          coverFileVersion: command.coverFileVersion,
        };
  if (target.kind === "project") {
    return operation === "delete"
      ? unwrap(
          await projects.deletePersonalProject({
            ...base,
            projectId: target.id,
          }),
        )
      : unwrap(
          await projects.editPersonalProject({
            ...base,
            projectId: target.id,
            title: command.title!,
            ...cover,
          }),
        );
  }
  if (target.projectBindingRevision == null)
    throw new ApiError("请先读取画布版本", 40901);
  const body = {
    ...base,
    expectedProjectBindingRevision: target.projectBindingRevision,
    expectedEpoch: target.epoch,
    expectedCommitSeq: target.commitSeq,
  };
  const prepared = await qualifyCanvasCommand(
    { ...body, canvasId: target.id },
    mayWrite,
  );
  command.target = {
    ...target,
    epoch: prepared.expectedEpoch,
    commitSeq: prepared.expectedCommitSeq,
  };
  if (!mayWrite())
    throw new ApiError("账户或画布权限已暂停，请重新验证", 40100);
  const { canvasId, ...version } = prepared;
  return operation === "delete"
    ? unwrap(await canvases.deleteCanvas({ ...version, id: canvasId }))
    : unwrap(
        await canvases.editCanvas({
          ...prepared,
          title: command.title!,
          ...cover,
        }),
      );
}

/** Unknown writes are recovered only by their frozen key and payload. */
export async function executeWorkspaceCommand(
  command: WorkspaceCommand,
  queryOnly = false,
  recoverFirst = false,
  mayWrite: () => boolean = () => true,
) {
  let result = queryOnly || recoverFirst ? await receipt(command) : null;
  if (!result && !queryOnly) {
    try {
      result = await write(command, mayWrite);
    } catch (error) {
      if (error instanceof ApiError && error.code < 50000) throw error;
      result = await receipt(command);
      if (!result) throw error;
    }
  }
  if (!result) throw new ApiError("暂未确认结果，请查询或重试原操作", 50300);
  const id = "canvasId" in result ? result.canvasId : result.projectId;
  if (String(id) !== command.target.id)
    throw new ApiError("操作回执不匹配，请继续核对原操作", 50300);
  return result;
}
