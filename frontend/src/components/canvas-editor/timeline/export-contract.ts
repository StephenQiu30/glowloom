import { requirePreview } from "@/components/media/library";
import { isProjectID } from "@/components/projects/project";

export const MediaExportStatus = {
  Queued: "queued",
  Running: "running",
  CancelRequested: "cancel_requested",
  ReviewRequired: "review_required",
  Succeeded: "succeeded",
  Failed: "failed",
  Cancelled: "cancelled",
} as const satisfies Record<string, API.MediatoolStatus>;
export const artifactKind = (kind?: string) =>
  ["media_bundle", "fcpxml", "xmeml", "jianying"].includes(kind ?? "");
const artifactVersion = (kind?: string) =>
  ({ media_bundle: "1", fcpxml: "1.10", xmeml: "5", jianying: "134.0.0" })[
    kind as "media_bundle" | "fcpxml" | "xmeml" | "jianying"
  ];
export function requireTimelineExport(
  value: API.ExportJobResponse,
  projectID: string,
  canvasID: string,
  nodeID: string,
  previous?: API.ExportJobResponse,
): API.ExportJobResponse {
  if (
    (previous &&
      (value.id !== previous.id ||
        value.revision! < previous.revision! ||
        value.source?.revision !== previous.source?.revision ||
        value.output_kind !== previous.output_kind)) ||
    !isProjectID(value.id) ||
    value.project_id !== projectID ||
    value.source?.canvas_id !== canvasID ||
    value.source?.node_id !== nodeID ||
    !Number.isSafeInteger(value.revision) ||
    value.revision! < 1 ||
    !Number.isSafeInteger(value.source?.revision) ||
    value.source!.revision! < 1 ||
    !["video", "audio", "media_bundle", "fcpxml", "xmeml", "jianying"].includes(
      value.output_kind ?? "",
    ) ||
    (["review_required", "succeeded"].includes(value.status ?? "") &&
      (!(artifactKind(value.output_kind)
        ? isProjectID(value.artifact_id) && !value.asset_id
        : isProjectID(value.asset_id) && !value.artifact_id) ||
        !/^[a-f0-9]{64}$/i.test(value.sha256 ?? ""))) ||
    ![
      "queued",
      "running",
      "cancel_requested",
      "review_required",
      "succeeded",
      "failed",
      "cancelled",
    ].includes(value.status ?? "")
  )
    throw new Error("导出任务身份不匹配");
  return value;
}
export function requireTimelinePreview(
  value: API.ExportPreview,
  job: API.ExportJobResponse,
) {
  if (
    value.job_id !== job.id ||
    value.revision !== job.revision ||
    value.sha256 !== job.sha256
  )
    throw new Error("导出预览版本不匹配");
  if (artifactKind(job.output_kind)) {
    const artifact = value.artifact;
    if (
      !artifact ||
      artifact.id !== job.artifact_id ||
      artifact.job_id !== job.id ||
      artifact.project_id !== job.project_id ||
      artifact.attempt !== job.attempt ||
      artifact.format !== job.output_kind ||
      artifact.format_version !== artifactVersion(job.output_kind) ||
      artifact.sha256 !== job.sha256 ||
      artifact.mime_type !== "application/zip" ||
      artifact.file_name !== `timeline-${job.id}.zip` ||
      !Number.isSafeInteger(artifact.byte_size) ||
      artifact.byte_size! < 1 ||
      artifact.byte_size! > 500 * 1024 * 1024 ||
      !Number.isFinite(Date.parse(artifact.created_at ?? ""))
    )
      throw new Error("工程产物身份不匹配");
    if (
      !value.url ||
      !Number.isFinite(Date.parse(value.expires_at ?? "")) ||
      Date.parse(value.expires_at!) <= Date.now()
    )
      throw new Error("工程下载地址已失效");
    const url = new URL(value.url);
    if (
      !["http:", "https:"].includes(url.protocol) ||
      url.username ||
      url.password
    )
      throw new Error("工程下载地址无效");
  } else {
    if (
      value.asset?.id !== job.asset_id ||
      value.asset?.project_id !== job.project_id ||
      value.asset?.kind !== job.output_kind
    )
      throw new Error("导出预览版本不匹配");
    requirePreview(value, job.asset_id!);
  }
  return value;
}
export function timelineJianyingLaunchURL(
  value: API.ExportPreview,
  job: API.ExportJobResponse,
) {
  if (job.output_kind !== "jianying" || job.status !== "succeeded")
    throw new Error("剪映工程尚未准备完成");
  requireTimelinePreview(value, job);
  const launch = new URL(value.launch_url ?? "");
  if (
    launch.origin !== window.location.origin ||
    !["http:", "https:"].includes(launch.protocol) ||
    launch.pathname !== "/api/mediaexports/launch" ||
    launch.username ||
    launch.password ||
    launch.hash ||
    [...launch.searchParams.keys()].length !== 1 ||
    !/^[a-f0-9]{64}$/.test(launch.searchParams.get("token") ?? "")
  )
    throw new Error("剪映启动地址无效");
  return (
    "videocut://com.ies.videocut/uganchor/anchor_point/load_pippit_draft?featureEntry=" +
    encodeURIComponent(
      JSON.stringify({
        sence: "main",
        sence_context: {},
        feature: "load_pippit_draft",
        enter_from: "glowloom_web",
        feature_context: { json_url: launch.href },
      }),
    )
  );
}
export async function verifyTimelineDownload(blob: Blob, sha256: string) {
  if (!(blob instanceof Blob) || !blob.size) throw new Error("导出文件为空");
  const digest = await crypto.subtle.digest(
    "SHA-256",
    await blob.arrayBuffer(),
  );
  const actual = Array.from(new Uint8Array(digest), (byte) =>
    byte.toString(16).padStart(2, "0"),
  ).join("");
  if (actual !== sha256.toLowerCase())
    throw new Error("导出文件校验不匹配，请重新下载");
}
export async function verifyArtifactDownload(
  blob: Blob,
  artifact: API.Artifact,
) {
  if (blob.size !== artifact.byte_size || blob.type !== "application/zip")
    throw new Error("工程文件类型或大小不匹配");
  await verifyTimelineDownload(blob, artifact.sha256!);
  const signature = new Uint8Array(await blob.slice(0, 4).arrayBuffer());
  if (signature.join(",") !== "80,75,3,4")
    throw new Error("工程文件不是有效的ZIP包");
}
