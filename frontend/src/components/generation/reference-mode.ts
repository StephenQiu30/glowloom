import { fieldsForMode } from "./contracts";
import type { PromptDraft } from "./prompt-document";

/** Resolve the provider mode from its declared input contract, not UI mode names. */
export function referenceMode(
  draft: PromptDraft,
  model: API.ModelResponse | undefined,
): string | null {
  if (
    !model?.current_version ||
    model.current_version.id !== draft.modelVersion
  )
    return draft.mode ?? null;
  const roles = model.current_version.limits?.roles as
    | Record<
        string,
        { types?: string[]; for_modes?: string[]; min_count?: number }
      >
    | undefined;
  const modes = model.current_version.modes ?? [];
  const inputs = new Map<string, Set<string>>();
  for (const ref of draft.references) {
    const sources = inputs.get(ref.role) ?? new Set<string>();
    const media =
      ref.source.kind === "subject"
        ? ref.source.media
        : ref.source.kind === "media"
          ? [ref.source]
          : [];
    for (const source of media)
      sources.add(`${source.sourceId}/${source.sourceVersion}`);
    inputs.set(ref.role, sources);
  }
  const eligible = modes.filter((mode) => {
    if (
      draft.references.some((ref) => {
        const role = roles?.[ref.role];
        return (
          !model.input_roles?.includes(ref.role) ||
          !role?.types?.includes(ref.mediaKind) ||
          (role.for_modes && !role.for_modes.includes(mode))
        );
      })
    )
      return false;
    return Object.entries(roles ?? {}).every(([name, role]) => {
      if (!role.min_count || (role.for_modes && !role.for_modes.includes(mode)))
        return true;
      return (inputs.get(name)?.size ?? 0) >= role.min_count;
    });
  });
  if (
    draft.mode &&
    [
      "music_song",
      "music_instrumental",
      "avatar",
      "motion",
      "omni_audio",
    ].includes(draft.mode) &&
    modes.includes(draft.mode)
  )
    return draft.mode;
  if (eligible.includes("omni_reference")) return "omni_reference";
  return (
    eligible[0] ??
    (modes.length === 1 && ["avatar", "motion"].includes(modes[0])
      ? modes[0]
      : null)
  );
}

/** A model/mode owns its parameter schema. Replace that schema atomically with selection. */
export function referenceGeneration(
  draft: PromptDraft,
  model: API.ModelResponse | undefined,
  previous?: Pick<PromptDraft, "modelVersion" | "mode">,
): Pick<PromptDraft, "mode" | "params"> {
  const mode = referenceMode(draft, model);
  if (
    !model?.current_version ||
    model.current_version.id !== draft.modelVersion ||
    !mode ||
    (previous?.modelVersion === draft.modelVersion && previous.mode === mode)
  )
    return { mode, params: draft.params };
  let fields;
  try {
    fields = fieldsForMode(model.current_version.param_schema, mode);
  } catch {
    // Invalid contracts remain blocked by editorIssues; never fabricate a schema.
    return { mode, params: draft.params };
  }
  const params: PromptDraft["params"] = {};
  for (const field of fields) {
    if (field.field === "output_count") continue;
    const value = draft.params[field.field];
    const valid =
      value !== undefined &&
      value !== null &&
      (field.type === "integer"
        ? typeof value === "number" && Number.isInteger(value)
        : field.type === "number"
          ? typeof value === "number" && Number.isFinite(value)
          : typeof value === field.type) &&
      (!field.enum || field.enum.includes(value as string | number)) &&
      (typeof value !== "number" ||
        ((field.min === undefined || value >= field.min) &&
          (field.max === undefined || value <= field.max)));
    if (valid) params[field.field] = value;
    else if (field.default !== undefined) params[field.field] = field.default;
  }
  const maxOutputs = Number(model.current_version.limits?.max_outputs ?? 1);
  const max = Number.isFinite(maxOutputs)
    ? Math.max(1, Math.min(8, Math.floor(maxOutputs)))
    : 1;
  const count = draft.params.output_count;
  params.output_count =
    typeof count === "number" && Number.isInteger(count)
      ? Math.max(1, Math.min(max, count))
      : 1;
  return { mode, params };
}
