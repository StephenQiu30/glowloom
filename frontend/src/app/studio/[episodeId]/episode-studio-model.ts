import {
  Blocks,
  Clapperboard,
  FileText,
  ImageIcon,
  ListChecks,
  type LucideIcon,
} from "lucide-react";

export type EpisodePanel = "script" | "assets" | "storyboard" | "media" | "tasks";

export const episodePanels: Array<{
  id: EpisodePanel;
  label: string;
  description: string;
  icon: LucideIcon;
}> = [
  { id: "script", label: "剧本结构", description: "版本、提取与人工决议", icon: FileText },
  { id: "assets", label: "资产准备", description: "角色、场景与声音", icon: Blocks },
  { id: "storyboard", label: "分镜设计", description: "镜头、规格与准备度", icon: Clapperboard },
  { id: "media", label: "媒体", description: "私有上传与探测", icon: ImageIcon },
  { id: "tasks", label: "任务", description: "状态恢复与失败原因", icon: ListChecks },
];

export const stageLabels: Record<API.EpisodeProductionSnapshot["current_stage"], string> = {
  script_import: "剧本导入",
  structure_review: "结构确认",
  asset_preparation: "资产准备",
  storyboard_preparation: "分镜准备",
};

export const scriptStatusLabels: Record<API.ScriptSummary["status"], string> = {
  not_started: "未导入",
  published: "待提取",
  extracting: "提取中",
  extraction_blocked: "提取受阻",
  review_required: "待决议",
  confirmation_required: "待确认结构",
  set_current_required: "待设为当前",
  confirmed: "结构已确认",
  unavailable: "暂时不可用",
};

export const taskStatusLabels: Record<API.TaskResponse["status"], string> = {
  queued: "排队中",
  running: "执行中",
  waiting_provider: "等待外部服务",
  succeeded: "已完成",
  failed: "失败",
  cancelled: "已取消",
  unknown: "待对账",
};

export const candidateKindLabels: Record<API.ExtractionCandidateResponse["kind"], string> = {
  scene: "场次",
  dialogue: "对白",
  asset: "资产",
  shot: "镜头建议",
  continuity: "连续性",
};

export const assetKindLabels: Record<API.AssetResponse["kind"], string> = {
  character: "角色",
  location: "场景",
  prop: "道具",
  costume: "服装",
  visual_style: "视觉风格",
  voice: "声音",
};

export function proposalTitle(candidate: API.ExtractionCandidateResponse): string {
  const proposal = candidate.proposal;
  if (proposal.kind === "scene") return proposal.heading;
  if (proposal.kind === "dialogue") return `${proposal.speaker_candidate}：${proposal.text}`;
  if (proposal.kind === "asset") return proposal.name;
  if (proposal.kind === "shot") return proposal.title;
  return proposal.issue;
}

export function proposalDescription(candidate: API.ExtractionCandidateResponse): string {
  const proposal = candidate.proposal;
  if (proposal.kind === "scene") return `${proposal.location} · ${proposal.time_of_day} · ${proposal.summary}`;
  if (proposal.kind === "dialogue") return proposal.performance_note ?? "无额外表演备注";
  if (proposal.kind === "asset") return proposal.description;
  if (proposal.kind === "shot") return proposal.purpose;
  return proposal.suggestion;
}

export function taskTone(status: API.TaskResponse["status"]): string {
  if (status === "succeeded") return "border-emerald-200 bg-emerald-50 text-emerald-700";
  if (status === "failed" || status === "unknown") return "border-rose-200 bg-rose-50 text-rose-700";
  if (status === "cancelled") return "border-slate-200 bg-slate-100 text-slate-600";
  return "border-border bg-muted text-foreground";
}

export function mediaKindFromFile(file: File): API.UploadDeclaration["kind"] {
  if (file.type.startsWith("image/")) return "image";
  if (file.type.startsWith("video/")) return "video";
  if (file.type.startsWith("audio/")) return "audio";
  if (/\.(?:txt|md)$/i.test(file.name)) return "document";
  return "subtitle";
}

export async function sha256File(file: File): Promise<string> {
  const bytes = await file.arrayBuffer();
  const digest = await crypto.subtle.digest("SHA-256", bytes);
  return Array.from(new Uint8Array(digest), (byte) =>
    byte.toString(16).padStart(2, "0"),
  ).join("");
}
