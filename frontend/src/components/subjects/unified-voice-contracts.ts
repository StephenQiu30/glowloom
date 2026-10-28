import { isProjectID } from "@/components/projects/project";
import { RequestError } from "@/lib/request";
import type { SelectedSubjectMedia } from "./unified-contracts";

export type SubjectVoiceModel = Required<
  Omit<API.VoiceModelOption, "default_voice_key" | "voices">
> & { voices: Required<API.VoiceOption>[]; default_voice_key?: string };
export type SubjectVoiceOptions = Required<
  Omit<API.VoiceOptions, "reason" | "models">
> & { reason?: string; models: SubjectVoiceModel[] };
export type SubjectVoiceInput = Required<
  Pick<
    API.CreateVoiceJobInput,
    "draft_intent_id" | "release_id" | "voice_key" | "name" | "text"
  >
>;
export type SubjectVoiceJob = Required<
  Omit<API.VoiceJob, "error_code" | "result" | "status">
> & {
  status:
    "queued" | "dispatching" | "ingesting" | "succeeded" | "failed" | "unknown";
  error_code?: string;
  result?: SelectedSubjectMedia;
};
const record = (value: unknown): value is Record<string, unknown> =>
  !!value && typeof value === "object" && !Array.isArray(value);
const nonempty = (value: unknown): value is string =>
  typeof value === "string" && !!value.trim();
const positive = (value: unknown) =>
  Number.isSafeInteger(value) && Number(value) > 0;
const fail = () => {
  throw new RequestError("invalid_receipt");
};

export function requireSubjectVoiceOptions(
  value: unknown,
  actor: string,
  scope: string,
): SubjectVoiceOptions {
  if (
    !record(value) ||
    value.current_actor_id !== actor ||
    value.current_scope_id !== scope ||
    typeof value.available !== "boolean" ||
    !Array.isArray(value.models)
  )
    return fail();
  const models = value.models;
  for (const model of models) {
    if (
      !record(model) ||
      !isProjectID(model.release_id) ||
      !isProjectID(model.model_version_id) ||
      !nonempty(model.model_key) ||
      !nonempty(model.display_name) ||
      !Array.isArray(model.voices) ||
      !model.voices.length ||
      model.voices.some(
        (voice) =>
          !record(voice) || !nonempty(voice.key) || !nonempty(voice.name),
      ) ||
      new Set(model.voices.map((voice) => voice.key)).size !==
        model.voices.length ||
      (model.default_voice_key !== undefined &&
        !model.voices.some((voice) => voice.key === model.default_voice_key))
    )
      return fail();
  }
  if (
    new Set(models.map((model) => model.release_id)).size !== models.length ||
    (value.available && !models.length)
  )
    return fail();
  return value as SubjectVoiceOptions;
}

export function requireSubjectVoiceJob(
  value: unknown,
  actor: string,
  scope: string,
  input: SubjectVoiceInput,
  modelVersion: string,
  previous?: SubjectVoiceJob,
): SubjectVoiceJob {
  if (
    !record(value) ||
    !isProjectID(value.id) ||
    value.current_actor_id !== actor ||
    value.current_scope_id !== scope ||
    value.draft_intent_id !== input.draft_intent_id ||
    value.release_id !== input.release_id ||
    value.model_version_id !== modelVersion ||
    value.voice_key !== input.voice_key ||
    value.name !== input.name ||
    value.text !== input.text ||
    (value.error_code !== undefined && typeof value.error_code !== "string") ||
    !positive(value.revision) ||
    ![
      "queued",
      "dispatching",
      "ingesting",
      "succeeded",
      "failed",
      "unknown",
    ].includes(String(value.status)) ||
    ![value.created_at, value.updated_at].every(
      (date) => typeof date === "string" && Number.isFinite(Date.parse(date)),
    ) ||
    (previous &&
      (value.id !== previous.id || Number(value.revision) < previous.revision))
  )
    return fail();
  if (value.result !== undefined) {
    const media = value.result;
    if (
      value.status !== "succeeded" ||
      !record(media) ||
      !isProjectID(media.asset_id) ||
      !positive(media.asset_revision) ||
      media.kind !== "audio" ||
      !nonempty(media.mime_type) ||
      !media.mime_type.startsWith("audio/") ||
      !nonempty(media.file_name) ||
      media.library_item_id !== media.asset_id ||
      media.library_source !== "subject"
    )
      return fail();
  } else if (value.status === "succeeded") return fail();
  return value as SubjectVoiceJob;
}

export function subjectVoiceError(code?: string) {
  if (code === "voice_storage_unavailable")
    return "对象存储尚未配置或就绪，请先完成TOS配置与存储迁移。";
  if (
    code === "voice_generation_unavailable" ||
    code === "doubao_speech_not_configured" ||
    code === "volcengine_speech_not_configured"
  )
    return "请先接入火山引擎豆包语音模型并配置可用音色。";
  if (code === "forbidden") return "当前账号无权使用此语音配置。";
  if (code === "unauthorized") return "登录已失效，请重新登录。";
  if (code === "invalid_receipt") return "音色任务回执无法核对，请恢复读取。";
  if (code === "invalid_request") return "请核对所选模型、音色和试听文本。";
  return "音色生成未完成，请检查配置后重试。";
}
