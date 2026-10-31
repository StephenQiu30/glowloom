import {
  fieldsForMode,
  initialParams,
} from "@/components/generation/contracts";
import { modelsForCreation } from "@/components/generation/creation-contract";
import {
  emptyPromptDraft,
  type MediaKind,
  type PromptDraft,
} from "@/components/generation/prompt-document";

export type CanvasAudioCatalogIdentity = { projectID: string; scopeID: string };
export type CanvasAudioModelCatalog = CanvasAudioCatalogIdentity &
  (
    | { status: "ready"; models: readonly API.ModelResponse[] }
    | { status: "loading" | "error" }
  );
/** Subject inputs must include every resolved physical kind, not a guessed cover type. */
export type CanvasAudioConnectionInput = {
  role: string;
  mediaKinds: readonly MediaKind[];
};
export type CanvasAudioConnectionSelection = {
  modelID: string;
  draft: PromptDraft;
};
const audioReferenceMode = "omni_audio";
const mediaKinds = ["image", "video", "audio"];

function readyModels(
  catalog: CanvasAudioModelCatalog,
  identity: CanvasAudioCatalogIdentity,
) {
  if (
    !identity.projectID ||
    !identity.scopeID ||
    catalog.projectID !== identity.projectID ||
    catalog.scopeID !== identity.scopeID ||
    catalog.status !== "ready"
  )
    throw new Error("音频模型能力尚未就绪，请稍后重试");
  return catalog.models;
}
function activeAudioModel(model: API.ModelResponse) {
  return (
    !!model.id &&
    model.capability === "audio.generate" &&
    model.status === "active" &&
    model.provider_status === "active" &&
    model.credential_present === true &&
    !!model.current_version?.id &&
    modelsForCreation([model], "audio").length === 1
  );
}
function roleTypes(
  model: API.ModelResponse,
  mode: string,
  role: string,
): string[] {
  if (!model.input_roles?.includes(role)) return [];
  const contract: unknown = model.current_version?.limits?.roles?.[role];
  if (!contract || typeof contract !== "object") return [];
  const rule = contract as { types?: unknown; for_modes?: unknown };
  if (
    rule.for_modes !== undefined &&
    (!Array.isArray(rule.for_modes) ||
      !rule.for_modes.every((value) => typeof value === "string") ||
      !rule.for_modes.includes(mode))
  )
    return [];
  if (
    !Array.isArray(rule.types) ||
    !rule.types.every(
      (value) => typeof value === "string" && mediaKinds.includes(value),
    )
  )
    return [];
  return rule.types;
}
function validateParameters(draft: PromptDraft, model: API.ModelResponse) {
  const fields = fieldsForMode(
    model.current_version?.param_schema,
    draft.mode ?? "",
  );
  for (const field of fields) {
    const value = draft.params[field.field];
    if (value === undefined || value === null || value === "") {
      if (field.required) throw new Error(`请先填写${field.label}`);
      continue;
    }
    if (
      (field.type === "integer"
        ? typeof value !== "number" || !Number.isInteger(value)
        : field.type === "number"
          ? typeof value !== "number" || !Number.isFinite(value)
          : typeof value !== field.type) ||
      (field.enum && !field.enum.includes(value as string | number)) ||
      (typeof value === "number" &&
        ((field.min !== undefined && value < field.min) ||
          (field.max !== undefined && value > field.max)))
    )
      throw new Error(`${field.label}不符合当前模型能力`);
  }
  const outputCount = draft.params.output_count ?? 1;
  const maxOutputs = model.current_version?.limits?.max_outputs ?? 1;
  if (
    typeof outputCount !== "number" ||
    !Number.isInteger(outputCount) ||
    outputCount < 1 ||
    typeof maxOutputs !== "number" ||
    outputCount > Math.min(8, maxOutputs)
  )
    throw new Error("音频候选数量不符合当前模型能力");
  if (
    Object.keys(draft.params).some(
      (key) =>
        key !== "output_count" && !fields.some((field) => field.field === key),
    )
  )
    throw new Error("音频草稿参数与当前模式不一致");
}

/** Stable catalog order owns the default; sources never influence model selection. */
export function createCanvasAudioConnectionDraft(
  catalog: CanvasAudioModelCatalog,
  identity: CanvasAudioCatalogIdentity,
): CanvasAudioConnectionSelection {
  for (const model of readyModels(catalog, identity)) {
    if (
      !activeAudioModel(model) ||
      !model.current_version!.modes?.includes(audioReferenceMode) ||
      !model.input_roles?.some(
        (role) => roleTypes(model, audioReferenceMode, role).length > 0,
      )
    )
      continue;
    try {
      const draft = emptyPromptDraft();
      draft.modelVersion = model.current_version!.id!;
      draft.mode = audioReferenceMode;
      draft.params = {
        ...initialParams(
          fieldsForMode(
            model.current_version!.param_schema,
            audioReferenceMode,
          ),
        ),
        output_count: 1,
      };
      validateParameters(draft, model);
      return { modelID: model.id!, draft };
    } catch {
      // An unusable release cannot provide the initial valid audio draft.
    }
  }
  throw new Error("当前项目没有可用于连线创建的全能配音模型");
}

/** Authoring preview only. Provider quoting still owns counts, duration and mutual exclusion. */
export function assertCanvasAudioConnectionCapability(
  catalog: CanvasAudioModelCatalog,
  identity: CanvasAudioCatalogIdentity,
  draft: PromptDraft | undefined,
  inputs: readonly CanvasAudioConnectionInput[],
): API.ModelResponse {
  const models = readyModels(catalog, identity);
  const model = models.find(
    (item) => item.current_version?.id === draft?.modelVersion,
  );
  if (
    !draft ||
    !model ||
    !activeAudioModel(model) ||
    draft.mode !== audioReferenceMode ||
    !model.current_version!.modes?.includes(draft.mode)
  )
    throw new Error("请先为音频节点选择有效的模型和模式");
  validateParameters(draft, model);
  const existing = draft.references.map((ref): CanvasAudioConnectionInput => {
    if (ref.source.kind === "pending_upload")
      throw new Error("音频参考素材尚未就绪");
    return {
      role: ref.role,
      mediaKinds:
        ref.source.kind === "subject" || ref.source.kind === "canvas_subject"
          ? ref.source.media.map((item) => item.mediaKind)
          : [ref.mediaKind],
    };
  });
  for (const input of [...existing, ...inputs]) {
    const accepted = roleTypes(model, draft.mode, input.role);
    if (
      !input.mediaKinds.length ||
      input.mediaKinds.some((kind) => !accepted.includes(kind))
    )
      throw new Error("当前音频模型模式不支持这些参考素材");
  }
  return model;
}

/** Fetch subject pins only when a current formal audio release can consume that role. */
export function canvasAudioCatalogAcceptsSubjects(
  catalog: CanvasAudioModelCatalog,
  identity: CanvasAudioCatalogIdentity,
): boolean {
  try {
    return readyModels(catalog, identity).some(
      (model) =>
        activeAudioModel(model) &&
        model.current_version!.modes?.includes(audioReferenceMode) &&
        roleTypes(model, audioReferenceMode, "subject").length > 0,
    );
  } catch {
    return false;
  }
}

/** The generated transport omits absent selections; this command only persists a real selected release. */
export function canvasAudioComposerForPersistence(
  draft: PromptDraft,
): API.PromptDraft {
  if (!draft.modelVersion || draft.mode !== audioReferenceMode)
    throw new Error("音频生成草稿缺少有效模型或模式");
  return {
    ...structuredClone(draft),
    modelVersion: draft.modelVersion,
    mode: draft.mode,
  };
}
