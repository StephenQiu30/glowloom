import { isProjectID } from "./project";

export type CopyIntent = {
  source: string;
  key: string;
  body: API.ProjectCopyRequest;
  jobID?: string;
};
export type CopyJob = API.ProjectCopyResponse & {
  id: string;
  source_project_id: string;
  target_project_id: string;
  revision: number;
  status: NonNullable<API.ProjectCopyResponse["status"]>;
};
export function copyIntent(
  source: string,
  revision: number,
  name: string,
): CopyIntent {
  const target = name.trim();
  if (
    !isProjectID(source) ||
    !Number.isSafeInteger(revision) ||
    revision < 1 ||
    !target ||
    Array.from(target).length > 50
  )
    throw new Error("请输入 1–50 字的名称");
  return {
    source,
    key: crypto.randomUUID(),
    body: { expected_revision: revision, target_name: target },
  };
}
export function restoreCopyIntent(raw: string, source: string): CopyIntent {
  const value = JSON.parse(raw) as CopyIntent;
  if (
    value.source !== source ||
    !isProjectID(value.key) ||
    (value.jobID !== undefined && !isProjectID(value.jobID))
  )
    throw new Error("复制恢复记录无效");
  copyIntent(
    source,
    value.body?.expected_revision,
    value.body?.target_name ?? "",
  );
  return value;
}
export function requireCopy(
  value: unknown,
  source: string,
  jobID?: string,
): CopyJob {
  const job = value as CopyJob | null;
  if (
    !job ||
    !isProjectID(job.id) ||
    (jobID && job.id !== jobID) ||
    job.source_project_id !== source ||
    !isProjectID(job.target_project_id) ||
    job.target_project_id === source ||
    !Number.isSafeInteger(job.revision) ||
    job.revision < 1 ||
    ![
      "queued",
      "running",
      "failed",
      "cancel_requested",
      "cancelled",
      "succeeded",
    ].includes(job.status)
  )
    throw new Error("复制结果无法确认，请重新读取");
  return job;
}
