import { modelAvailableForMode } from "@/components/generation/model-availability";
import type { PromptDraft } from "@/components/generation/prompt-document";
import type { ConversationSkillPin } from "./conversation-context";

export const shotDeliveryCandidateKind = "shot_delivery";
const projectFormatLabels = new Map([
  ["fcpxml", "FCPXML"],
  ["xmeml", "XMEML"],
  ["jianying", "剪映"],
]);
function record(value: unknown): value is Record<string, unknown> {
  return !!value && typeof value === "object" && !Array.isArray(value);
}

export function readShotDeliveryProposal(value: unknown) {
  if (
    !record(value) ||
    value.kind !== shotDeliveryCandidateKind ||
    !record(value.input)
  )
    return;
  const input = value.input;
  if (
    typeof value.asset_id !== "string" ||
    !/^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i.test(
      value.asset_id,
    ) ||
    typeof input.source_start_ms !== "number" ||
    !Number.isSafeInteger(input.source_start_ms) ||
    input.source_start_ms < 0 ||
    typeof input.duration_ms !== "number" ||
    !Number.isSafeInteger(input.duration_ms) ||
    input.duration_ms <= 0 ||
    typeof input.fps !== "number" ||
    !Number.isSafeInteger(input.fps) ||
    input.fps <= 0 ||
    typeof input.aspect_ratio !== "string" ||
    !input.aspect_ratio ||
    typeof input.project_format !== "string"
  )
    return;
  const format = projectFormatLabels.get(input.project_format);
  if (!format) return;
  return {
    assetID: value.asset_id,
    sourceStartMS: input.source_start_ms,
    durationMS: input.duration_ms,
    aspectRatio: input.aspect_ratio,
    fps: input.fps,
    format,
  };
}

export function isVersionedSkill(
  skill: API.SkillVersion,
): skill is API.SkillVersion & ConversationSkillPin {
  return Boolean(skill.skill_id && skill.version && skill.resource_hash);
}

export function isSameSkillVersion(
  a: ConversationSkillPin,
  b: ConversationSkillPin,
) {
  return (
    a.skill_id === b.skill_id &&
    a.version === b.version &&
    a.resource_hash === b.resource_hash
  );
}

export function selectSkillVersion(
  pins: ConversationSkillPin[],
  { skill_id, version, resource_hash }: ConversationSkillPin,
) {
  return [
    ...pins.filter((pin) => pin.skill_id !== skill_id),
    { skill_id, version, resource_hash },
  ];
}

export interface SubmitCheck {
  draft: PromptDraft;
  planningModel?: API.ModelResponse;
  authorized: boolean;
  budget: string;
  busy: boolean;
  active: boolean;
}
export const terminalStatuses = new Set([
  "completed",
  "failed",
  "stopped",
  "rejected",
]);
export function draftText(draft: PromptDraft) {
  return draft.document.content
    .map((p) =>
      (p.content ?? [])
        .map((n) =>
          n.type === "text" ? n.text : n.type === "hardBreak" ? "\n" : "",
        )
        .join(""),
    )
    .join("\n");
}
export function budgetMicros(value: string): number | undefined {
  if (!/^(0|[1-9]\d{0,6})(\.\d{1,6})?$/.test(value)) return;
  const [whole, fraction = ""] = value.split(".");
  const micros =
    BigInt(whole!) * BigInt(1000000) + BigInt(fraction.padEnd(6, "0"));
  return micros > BigInt(0) && micros <= BigInt(1000000000000)
    ? Number(micros)
    : undefined;
}
export function canSubmitConversation({
  draft,
  planningModel: model,
  authorized,
  budget,
  busy,
  active,
}: SubmitCheck): { allowed: boolean; reason?: string } {
  const blocked = (reason: string) => ({ allowed: false, reason });
  if (!authorized) return blocked("登录并选择有权限的项目后发送");
  if (busy) return blocked("正在核对本次提交");
  if (active) return blocked("请先完成或停止当前创作");
  if (
    !model ||
    model.capability !== "text.generate" ||
    !modelAvailableForMode(model, "text_to_text") ||
    !model.current_version?.id ||
    !model.current_price?.id
  )
    return blocked("请选择可用的规划模型");
  if (budgetMicros(budget) === undefined)
    return blocked("请输入有效的预算上限");
  let media = 0,
    visuals = 0,
    subjectText = false;
  const counts: Record<string, number> = {};
  const roles: unknown = model.current_version.limits?.roles;
  for (const ref of draft.references) {
    if (ref.source.kind === "pending_upload")
      return blocked("等待参考上传完成，或移除待上传项");
    const pins =
      ref.source.kind === "media"
        ? [{ mediaKind: ref.mediaKind }]
        : ref.source.media;
    if (ref.source.kind !== "media" && ref.source.description.trim())
      subjectText = true;
    for (const pin of pins) {
      counts[pin.mediaKind] = (counts[pin.mediaKind] ?? 0) + 1;
      media++;
      if (!["image", "video", "document"].includes(pin.mediaKind))
        return blocked("当前规划模型不支持这份参考，请更换模型或调整素材");
      if (counts[pin.mediaKind]! > 30)
        return blocked("每次最多读取30张图片、30个视频和30份文档");
      visuals +=
        pin.mediaKind === "video" ? 3 : pin.mediaKind === "image" ? 1 : 0;
    }
  }
  if (visuals > 0) {
    const raw =
      roles && typeof roles === "object"
        ? (roles as Record<string, unknown>).reference_image
        : undefined;
    const limit =
      raw && typeof raw === "object"
        ? (raw as { types?: unknown; max_count?: unknown })
        : undefined;
    if (
      !model.input_roles?.includes("reference_image") ||
      !limit ||
      !Array.isArray(limit.types) ||
      !limit.types.includes("image") ||
      typeof limit.max_count !== "number" ||
      visuals > limit.max_count
    )
      return blocked(
        "当前规划模型不能读取全部参考画面（每视频抽取3帧），请减少素材或更换模型",
      );
  }
  if (!draftText(draft).trim() && !media && !subjectText)
    return blocked("输入新要求，或添加可用的参考素材");
  return { allowed: true };
}
export function compareCursor(left: string, right: string) {
  const [a = "0", b = "0"] = left.split(":"),
    [c = "0", d = "0"] = right.split(":");
  if (BigInt(a) !== BigInt(c)) return BigInt(a) < BigInt(c) ? -1 : 1;
  return Number(b) - Number(d);
}
export function mergeMessages(
  existing: API.PublicMessage[],
  incoming: API.PublicMessage[],
): API.PublicMessage[] {
  const parts = new Map<string, API.PublicMessage>();
  for (const part of [...existing, ...incoming])
    parts.set(`${part.run_id}:${part.message_id}:${part.part_index}`, part);
  return [...parts.values()].sort(
    (a, b) =>
      compareCursor(a.sequence, b.sequence) || a.part_index - b.part_index,
  );
}
export interface ConversationRun {
  snapshot: API.RunSnapshot;
  messages: API.PublicMessage[];
  older?: { before: string; through: string };
}
export function mergeRun(
  current: ConversationRun | undefined,
  incoming: API.RunSnapshot,
  resetHistory = false,
): ConversationRun {
  const page = incoming.messages_page;
  const lastKnown = current?.messages.at(-1);
  const firstIncoming = page.items[0];
  const gap =
    !!firstIncoming &&
    (!lastKnown ||
      compareCursor(firstIncoming.sequence, lastKnown.sequence) > 0 ||
      (firstIncoming.sequence === lastKnown.sequence &&
        firstIncoming.part_index > lastKnown.part_index + 1));
  const fresh =
    !current ||
    compareCursor(incoming.snapshot_cursor, current.snapshot.snapshot_cursor) >=
      0;
  const older =
    !current || resetHistory || (fresh && gap)
      ? page.next_cursor
        ? { before: page.next_cursor, through: page.through }
        : current?.older
      : current.older;
  const state =
    current &&
    current.snapshot.state.execution_revision >
      incoming.state.execution_revision
      ? current.snapshot.state
      : incoming.state;
  return {
    snapshot: { ...incoming, state },
    messages: mergeMessages(current?.messages ?? [], page.items),
    ...(older ? { older } : {}),
  };
}
export function readMessageParts(
  value: unknown,
  runID: string,
): API.PublicMessage[] {
  if (!Array.isArray(value)) return [];
  return value.filter(
    (item): item is API.PublicMessage =>
      !!item &&
      typeof item === "object" &&
      item.run_id === runID &&
      typeof item.message_id === "string" &&
      typeof item.content === "string" &&
      typeof item.sequence === "string" &&
      /^\d+:\d+$/.test(item.sequence) &&
      Number.isSafeInteger(item.part_index) &&
      item.part_index >= 0 &&
      typeof item.kind === "string",
  );
}
