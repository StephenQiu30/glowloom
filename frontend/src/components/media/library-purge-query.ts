import { ApiError } from "@/lib/request";
import { z } from "zod";
import * as media from "@/gen/api/media";
import { getProject } from "@/components/project/queries";
import {
  defaultLibraryFilter,
  libraryUUID,
  type LibraryIdentity,
} from "./library-model";
import { freshLibrary, getLibraryDetail, listLibrary } from "./library-queries";
import {
  purgeItemsSchema,
  purgePlanTargetsSchema,
  purgeIntentSchema,
  readPurgeJob,
  type PurgeReview,
  type PurgeIntent,
} from "./library-purge-model";
export async function reviewLibraryPurge(
  identity: LibraryIdentity,
  selection?: { id: string; revision: number }[],
  signal?: AbortSignal,
  allowEmpty = false,
): Promise<PurgeReview> {
  const [start, project] = await Promise.all([
    freshLibrary(identity, signal),
    identity.scope.kind === "project"
      ? getProject(identity.scope.project_id, signal)
      : Promise.resolve(null),
  ]);
  if (project && (project.status !== "active" || project.is_delete))
    throw new Error("当前项目只读，不能永久清理。");
  const items: PurgeReview["items"] = [];
  if (selection) {
    const checked = purgeItemsSchema.parse(selection);
    let next = 0;
    await Promise.all(
      Array.from({ length: Math.min(8, checked.length) }, async () => {
        while (next < checked.length) {
          const index = next++,
            selected = checked[index],
            detail = await getLibraryDetail(identity, selected.id, signal);
          if (
            detail.catalog_state !== "trashed" ||
            detail.revision !== selected.revision
          )
            throw new Error("所选素材已变化，请关闭并重新选择、审阅。");
          items[index] = {
            id: detail.id,
            revision: detail.revision,
            title: detail.title,
          };
        }
      }),
    );
  } else {
    const filter = {
      ...defaultLibraryFilter(identity.scope),
      catalog_state: "trashed" as const,
      page_size: identity.scope.kind === "personal" ? 120 : 80,
      order: "name_asc" as const,
    };
    const first = await listLibrary(identity.scope, filter, signal, identity),
      total = first.total;
    if (total > 2000)
      throw new Error(
        `回收站共 ${total} 项，超过完整计划2000项/10子批预算。清空已停止；请分范围处理后重新审阅全部，不会截断清理。`,
      );
    const pages = Array.from(
      { length: Math.max(1, Math.ceil(total / filter.page_size)) },
      () => first,
    );
    let next = 1;
    await Promise.all(
      Array.from({ length: Math.min(8, pages.length - 1) }, async () => {
        while (next < pages.length) {
          const index = next++;
          pages[index] = await listLibrary(
            identity.scope,
            { ...filter, page: index + 1 },
            signal,
            identity,
          );
        }
      }),
    );
    for (const [index, result] of pages.entries()) {
      if (result.revision !== start.revision || result.total !== total)
        throw new Error(
          "回收站在读取时发生变化，清空已停止。请重新读取全部并审阅。",
        );
      const expected = Math.min(
        filter.page_size,
        Math.max(0, total - index * filter.page_size),
      );
      if (result.items.length !== expected)
        throw new ApiError(502, "invalid_response");
      items.push(
        ...result.items.map((item) => ({
          id: item.id,
          revision: item.revision,
          title: item.title,
        })),
      );
    }
    if (items.length !== total) throw new ApiError(502, "invalid_response");
    if (!items.length && !allowEmpty)
      throw new Error("当前整个回收站为空，没有可清理条目。");
    if (items.length)
      purgePlanTargetsSchema.parse(
        items.map(({ id, revision }) => ({ id, revision })),
      );
  }
  const [end, endProject] = await Promise.all([
    freshLibrary(identity, signal),
    project ? getProject(project.id, signal) : Promise.resolve(null),
  ]);
  if (
    start.revision !== end.revision ||
    (project &&
      (!endProject ||
        endProject.revision !== project.revision ||
        endProject.status !== "active" ||
        endProject.is_delete))
  )
    throw new Error("素材库或项目版本已变化，清理已停止，请重新审阅。");
  return {
    mode: selection ? "selected" : "all",
    revision: start.revision,
    projectRevision: project?.revision ?? 0,
    items,
  };
}
const scopeParams = (identity: LibraryIdentity) => ({
  scope: identity.scope.kind,
  ...(identity.scope.kind === "project"
    ? { project_id: identity.scope.project_id }
    : {}),
});
export async function listLibraryPurges(
  identity: LibraryIdentity,
  page: number,
  signal?: AbortSignal,
) {
  await freshLibrary(identity, signal);
  const response = z
    .object({
      current_actor_id: libraryUUID,
      current_org_id: libraryUUID,
      page: z.literal(page),
      page_size: z.literal(20),
      items: z.array(z.unknown()).max(20),
    })
    .strict()
    .safeParse(
      await media.listMediaPurges(
        { ...scopeParams(identity), page, page_size: 20 },
        { signal },
      ),
    );
  if (
    !response.success ||
    response.data.current_actor_id !== identity.actorId ||
    response.data.current_org_id !== identity.orgId
  )
    throw new ApiError(502, "invalid_response");
  const items = response.data.items.map((job) => readPurgeJob(job, identity));
  if (new Set(items.map((job) => job.id)).size !== items.length)
    throw new ApiError(502, "invalid_response");
  return items;
}
export async function getLibraryPurge(
  identity: LibraryIdentity,
  id: string,
  signal?: AbortSignal,
) {
  libraryUUID.parse(id);
  await freshLibrary(identity, signal);
  return readPurgeJob(
    await media.getMediaPurge({ job_id: id }, { signal }),
    identity,
    id,
  );
}
export async function runLibraryPurge(
  intent: PurgeIntent,
  signal?: AbortSignal,
) {
  const checked = purgeIntentSchema.parse(intent),
    options = {
      signal,
      headers: { "Idempotency-Key": checked.key, Origin: checked.origin },
    };
  const response: unknown =
    checked.action === "create"
      ? await media.createMediaPurge(checked.body, options)
      : checked.action === "cancel"
        ? await media.cancelMediaPurge(
            { job_id: checked.jobId },
            checked.body,
            options,
          )
        : await media.reconcileMediaPurge(
            { job_id: checked.jobId },
            checked.body,
            options,
          );
  const job = readPurgeJob(
    response,
    checked,
    checked.action === "create" ? undefined : checked.jobId,
  );
  if (
    checked.action === "create" &&
    (job.items.length !== checked.body.items.length ||
      job.items.some(
        (item, index) => item.item_id !== checked.body.items[index].id,
      ))
  )
    throw new ApiError(502, "invalid_response");
  return job;
}
