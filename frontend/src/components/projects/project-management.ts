import {
  archiveProject,
  deleteProject,
  getProject,
  listProjects,
  restoreProject,
  unarchiveProject,
} from "@/api/projects";
import { RequestError } from "@/lib/request";
import { requireProject, requireProjectList, type Project } from "./project";
import { projectPresentationStore } from "./project-presentation-store";

export const projectActions = {
  archive: { label: "归档", request: archiveProject },
  unarchive: { label: "取消归档", request: unarchiveProject },
  recycle: { label: "移入回收站", request: deleteProject },
  restore: { label: "恢复", request: restoreProject },
} as const;
export type ProjectAction = keyof typeof projectActions;
export class ProjectManagementError extends Error {
  constructor(
    public readonly outcome: "rejected" | "unknown",
    message: string,
  ) {
    super(message);
  }
}
export function canManageProject(p: Project, action: ProjectAction) {
  if (action === "restore")
    return (
      p.is_delete === true &&
      (p.status === "active" || p.status === "archived") &&
      !!p.purge_after &&
      Date.parse(p.purge_after) >
        Date.parse(p.server_time ?? new Date().toISOString())
    );
  if (p.is_delete !== false) return false;
  return action === "archive"
    ? p.status === "active"
    : action === "unarchive"
      ? p.status === "archived"
      : p.status === "active" || p.status === "archived";
}
function achieved(p: Project, action: ProjectAction, before: Project) {
  return action === "recycle"
    ? p.is_delete === true && p.status === before.status
    : p.is_delete === false &&
        p.status ===
          (action === "restore"
            ? before.status
            : action === "archive"
              ? "archived"
              : "active");
}
export function projectManagementMessage(reason: unknown) {
  if (reason instanceof RequestError) {
    const messages: Record<string, string> = {
      revision_conflict: "画布已发生变化，请重新读取后操作。",
      inflight_work: "画布仍有进行中的任务，请待任务结束后操作。",
      restore_expired: "已超过30天恢复期限，无法恢复。",
      state_conflict: "画布状态已变化，请重新读取列表。",
      idempotency_key_reused: "原操作校验冲突，请重新读取列表核对。",
    };
    return (
      messages[reason.code] ??
      (reason.status === 401
        ? "登录已失效，请重新登录。"
        : reason.status === 403 || reason.status === 404
          ? "当前无权访问此画布，请重新读取列表核对。"
          : "结果未确认，请重新读取列表核对。")
    );
  }
  return reason instanceof Error
    ? reason.message
    : "结果未确认，请重新读取列表核对。";
}
async function reconcile(p: Project, action: ProjectAction, guard: () => void) {
  if (action !== "recycle") {
    const result = requireProject(await getProject({ pid: p.id }));
    guard();
    return result.id === p.id &&
      result.revision > p.revision &&
      achieved(result, action, p)
      ? result
      : null;
  }
  // Ordinary detail reads deliberately hide recycled projects. Search every
  // authorized page, including expired tombstones, without issuing another write.
  let cursor: string | undefined;
  const visited = new Set<string>();
  do {
    guard();
    const page = requireProjectList(
      await listProjects({
        deleted: true,
        include_expired: true,
        limit: 50,
        ...(cursor ? { cursor } : {}),
      }),
    );
    guard();
    const result = page.items.find((item) => item.id === p.id);
    if (result)
      return result.revision > p.revision && achieved(result, action, p)
        ? result
        : null;
    cursor = page.next_cursor ?? undefined;
    if (cursor && visited.has(cursor))
      throw new Error("回收站分页结果未确认。");
    if (cursor) visited.add(cursor);
  } while (cursor);
  return null;
}
/** One command per item. A lost receipt is reconciled by a read, never a new key. */
export async function manageProject(
  p: Project,
  action: ProjectAction,
  guard: () => void,
  namespace?: string,
  retryPending = false,
) {
  guard();
  const pending = namespace
    ? projectPresentationStore.management(namespace, p.id)
    : undefined;
  if (pending) {
    const observed = await reconcile(
      pending.project,
      pending.action,
      guard,
    ).catch(() => null);
    guard();
    if (observed) {
      projectPresentationStore.dropManagement(namespace!, p.id, pending.key);
      if (pending.action === action) return observed;
    } else if (!retryPending || pending.action !== action)
      throw new ProjectManagementError(
        "unknown",
        "原操作结果未确认，请先重新读取列表核对。",
      );
  }
  const replay =
    retryPending && pending?.action === action
      ? projectPresentationStore.management(namespace!, p.id)
      : undefined;
  const current =
    replay?.project ??
    (action === "restore"
      ? requireProject(p)
      : requireProject(await getProject({ pid: p.id })));
  guard();
  if (
    !replay &&
    (current.id !== p.id ||
      current.revision !== p.revision ||
      !canManageProject(current, action))
  )
    throw new ProjectManagementError(
      "rejected",
      "画布已发生变化或恢复已过期，请重新读取后操作。",
    );
  const key = replay?.key ?? crypto.randomUUID();
  if (namespace && !replay)
    projectPresentationStore.setManagement(namespace, p.id, {
      key,
      action,
      project: current,
    });
  try {
    const result = requireProject(
      await projectActions[action].request(
        { pid: p.id },
        { expected_revision: current.revision },
        {
          headers: { "Idempotency-Key": key },
          sessionRecovery: "never",
        },
      ),
    );
    guard();
    if (
      result.id !== p.id ||
      result.revision <= current.revision ||
      !achieved(result, action, current)
    )
      throw new RequestError("invalid_receipt");
    if (namespace)
      projectPresentationStore.dropManagement(namespace, p.id, key);
    return result;
  } catch (reason) {
    guard();
    if (
      reason instanceof RequestError &&
      reason.status >= 400 &&
      reason.status < 500 &&
      reason.status !== 408
    ) {
      if (namespace)
        projectPresentationStore.dropManagement(namespace, p.id, key);
      throw reason;
    }
    const observed = await reconcile(current, action, guard).catch(() => null);
    guard();
    if (observed) {
      if (namespace)
        projectPresentationStore.dropManagement(namespace, p.id, key);
      return observed;
    }
    throw new ProjectManagementError(
      "unknown",
      "结果未确认，请重新读取列表后核对。",
    );
  }
}
