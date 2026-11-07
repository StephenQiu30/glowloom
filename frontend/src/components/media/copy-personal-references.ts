import {
  createMediaTransfer,
  getMediaLibraryItem,
  getMediaTransfer,
  listMediaLibrary,
} from "@/api/media";
import { getProject } from "@/api/projects";
import { isProjectID } from "@/components/projects/project";
import { getScopeGeneration, getSessionGeneration } from "@/lib/request";

export type PersonalReferenceCopy = {
  readonly key: string;
  readonly actorID: string;
  readonly orgID: string;
  readonly projectID: string;
  readonly session: number;
  readonly scope: number;
  readonly input: API.TransferInput;
  readonly pins: readonly ReferenceCopyItem[];
  job?: API.TransferJob;
};
export type ReferenceCopyItem = {
  id: string;
  revision: number;
  asset_id: string;
  asset_revision: number;
};

async function requireSourcePins(
  pins: readonly ReferenceCopyItem[],
  signal: AbortSignal,
) {
  for (let offset = 0; offset < pins.length; offset += 8) {
    await Promise.all(
      pins.slice(offset, offset + 8).map(async (pin) => {
        const item = await getMediaLibraryItem(
          { scope: "personal", item_id: pin.id },
          { signal },
        );
        if (
          item.id !== pin.id ||
          item.revision !== pin.revision ||
          item.catalog_state !== "active" ||
          item.asset_id !== pin.asset_id ||
          item.media?.id !== pin.asset_id ||
          item.media.revision !== pin.asset_revision
        )
          throw new Error(
            "参考素材版本已变化，请重新选择；已确认的复制记录会保留。",
          );
      }),
    );
  }
}

function active(
  command: Pick<PersonalReferenceCopy, "session" | "scope">,
  signal: AbortSignal,
) {
  signal.throwIfAborted();
  if (
    command.session !== getSessionGeneration() ||
    command.scope !== getScopeGeneration()
  )
    throw new Error("账号或创作空间已变化，请重新选择参考。");
}

/** Freeze a real copy command once; callers retain it across uncertain replies. */
export async function createPersonalReferenceCopy(
  projectID: string,
  items: ReferenceCopyItem[],
  actorID: string,
  signal: AbortSignal,
): Promise<PersonalReferenceCopy> {
  const identity = {
    session: getSessionGeneration(),
    scope: getScopeGeneration(),
  };
  if (
    !isProjectID(projectID) ||
    !isProjectID(actorID) ||
    !items.length ||
    items.length > 100 ||
    new Set(items.map((i) => i.id)).size !== items.length ||
    items.some(
      (i) =>
        !isProjectID(i.id) ||
        !Number.isSafeInteger(i.revision) ||
        i.revision < 1 ||
        !isProjectID(i.asset_id) ||
        !Number.isSafeInteger(i.asset_revision) ||
        i.asset_revision < 1,
    )
  )
    throw new Error("参考条目的身份或版本无效，请重新选择。");
  const options = { signal };
  const [source, target, project] = await Promise.all([
    listMediaLibrary({ scope: "personal", page: 1, page_size: 1 }, options),
    listMediaLibrary(
      { scope: "project", project_id: projectID, page: 1, page_size: 1 },
      options,
    ),
    getProject({ pid: projectID }, options),
  ]);
  active(identity, signal);
  if (
    source.current_actor_id !== actorID ||
    target.current_actor_id !== actorID ||
    !isProjectID(source.current_org_id) ||
    source.current_org_id !== target.current_org_id ||
    source.scope?.kind !== "personal" ||
    target.scope?.kind !== "project" ||
    target.scope.project_id !== projectID ||
    project.id !== projectID ||
    !Number.isSafeInteger(project.revision) ||
    !Number.isSafeInteger(source.revision) ||
    !Number.isSafeInteger(target.revision)
  )
    throw new Error("参考或目标项目权限已变化，请重新读取。");
  await requireSourcePins(items, signal);
  active(identity, signal);
  return {
    key: crypto.randomUUID(),
    actorID,
    orgID: source.current_org_id,
    projectID,
    ...identity,
    pins: items.map((item) => Object.freeze({ ...item })),
    input: {
      source: { kind: "personal" },
      target: { kind: "project", project_id: projectID },
      items: items.map((i) => ({ id: i.id, revision: i.revision })),
      expected_source_revision: source.revision,
      expected_target_revision: target.revision,
      expected_project_revision: project.revision,
    },
  };
}

function requireJob(
  value: unknown,
  command: PersonalReferenceCopy,
): API.TransferJob {
  const job = value as API.TransferJob;
  if (
    !job ||
    !isProjectID(job.id) ||
    job.current_actor_id !== command.actorID ||
    job.current_org_id !== command.orgID ||
    job.source?.kind !== "personal" ||
    job.target?.kind !== "project" ||
    job.target.project_id !== command.projectID ||
    !Number.isSafeInteger(job.revision) ||
    job.revision! < 1 ||
    !Array.isArray(job.items) ||
    job.items.length !== command.input.items!.length ||
    (command.job &&
      (command.job.id !== job.id || job.revision! < command.job.revision!)) ||
    job.items.some(
      (item, index) =>
        item.index !== index ||
        item.source_item_id !== command.input.items![index]!.id,
    )
  )
    throw new Error("素材复制回执不匹配，请保留原请求并核对。");
  return job;
}

function delay(signal: AbortSignal) {
  return new Promise<void>((resolve, reject) => {
    signal.throwIfAborted();
    const abort = () => {
      clearTimeout(timer);
      reject(new DOMException("已取消读取", "AbortError"));
    };
    const timer = setTimeout(() => {
      signal.removeEventListener("abort", abort);
      resolve();
    }, 1000);
    signal.addEventListener("abort", abort, { once: true });
  });
}

/** Read only successful target identities. No source ID can masquerade as a target. */
export async function executePersonalReferenceCopy(
  command: PersonalReferenceCopy,
  signal: AbortSignal,
): Promise<API.AssetSummary[]> {
  active(command, signal);
  const options = { signal };
  let value: unknown = command.job
    ? await getMediaTransfer({ job_id: command.job.id! }, options)
    : await createMediaTransfer(command.input, {
        ...options,
        headers: { "Idempotency-Key": command.key },
      });
  for (;;) {
    active(command, signal);
    const job = requireJob(value, command);
    command.job = job;
    if (job.status === "succeeded") {
      await requireSourcePins(command.pins, signal);
      active(command, signal);
      const assets = await Promise.all(
        job.items!.map(async (item) => {
          if (
            item.status !== "succeeded" ||
            !isProjectID(item.target_item_id) ||
            !isProjectID(item.target_asset_id)
          )
            throw new Error("部分参考仍未复制成功。");
          const detail = await getMediaLibraryItem(
            {
              scope: "project",
              project_id: command.projectID,
              item_id: item.target_item_id,
            },
            options,
          );
          active(command, signal);
          const a = detail.media;
          if (
            detail.id !== item.target_item_id ||
            detail.catalog_state !== "active" ||
            detail.asset_id !== item.target_asset_id ||
            a?.id !== item.target_asset_id ||
            !Number.isSafeInteger(a.revision) ||
            Number(a.revision) < 1 ||
            !["image", "video", "document"].includes(a.kind ?? "")
          )
            throw new Error("项目参考尚未就绪，请重读原复制结果。");
          return {
            ...a,
            project_id: command.projectID,
            duration_ms: a.duration_ms ?? undefined,
            width: a.width ?? undefined,
            height: a.height ?? undefined,
          };
        }),
      );
      active(command, signal);
      return assets;
    }
    if (
      !["queued", "running"].includes(job.status ?? "") ||
      job.execution_unconfirmed ||
      job.needs_reconciliation
    )
      throw new Error(
        "素材复制尚未完成，请保留原记录并到资产转移记录核对后重试。",
      );
    await delay(signal);
    value = await getMediaTransfer({ job_id: job.id! }, options);
  }
}
