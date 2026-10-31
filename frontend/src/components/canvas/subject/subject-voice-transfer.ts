import { isProjectID } from "@/components/projects/project";
export function requireVoiceTransfer(
  value: unknown,
  actor: string,
  project: string,
  source: string,
  previous?: API.TransferJob,
): API.TransferJob {
  const job = value as API.TransferJob;
  if (
    !job ||
    !isProjectID(job.id) ||
    job.current_actor_id !== actor ||
    job.source?.kind !== "personal" ||
    job.target?.kind !== "project" ||
    job.target.project_id !== project ||
    !Number.isSafeInteger(job.revision) ||
    job.revision! < 1 ||
    !Array.isArray(job.items) ||
    job.items.length !== 1 ||
    job.items[0].source_item_id !== source ||
    job.items[0].index !== 0 ||
    ![
      "queued",
      "running",
      "succeeded",
      "partial_failed",
      "failed",
      "cancelled",
      "unknown",
      "needs_reconciliation",
      "cancel_requested",
    ].includes(job.status ?? "") ||
    (previous && (job.id !== previous.id || job.revision! < previous.revision!))
  )
    throw new Error("音频转移回执无法核对，请恢复读取");
  if (
    job.status === "succeeded" &&
    (job.items[0].status !== "succeeded" ||
      !isProjectID(job.items[0].target_asset_id) ||
      !isProjectID(job.items[0].target_item_id))
  )
    throw new Error("音频转移结果不完整");
  return job;
}
