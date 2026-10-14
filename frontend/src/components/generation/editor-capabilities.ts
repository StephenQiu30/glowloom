import { fieldsForMode, initialParams } from "./contracts";
import { referenceLabel, type PromptDraft } from "./prompt-document";
export function editorIssues(
  draft: PromptDraft,
  model?: API.ModelResponse,
): string[] {
  const issues: string[] = [];
  if (
    !model ||
    model.status !== "active" ||
    model.provider_status !== "active" ||
    !model.credential_present ||
    model.current_version?.id !== draft.modelVersion
  )
    return ["请选择当前可用的模型版本；已有输入会保留。"];
  if (!draft.mode || !model.current_version.modes?.includes(draft.mode))
    issues.push("该模式当前不可用。");
  const limits = model.current_version.limits as
    | {
        max_outputs?: number;
        roles?: Record<
          string,
          {
            types?: string[];
            max_count?: number;
            min_count?: number;
            for_modes?: string[];
          }
        >;
        _total_images?: number;
      }
    | undefined;
  const physical = new Map<string, number>();
  for (const ref of draft.references) {
    const label = referenceLabel(draft.labelState, ref.id),
      role = limits?.roles?.[ref.role];
    if (ref.source.kind === "pending_upload")
      issues.push(`${label} 尚未完成上传，请重选或重试。`);
    if (
      !model.input_roles?.includes(ref.role) ||
      !role ||
      !role.types?.includes(ref.mediaKind) ||
      (role.for_modes && !role.for_modes.includes(draft.mode ?? ""))
    )
      issues.push(`${label} 的 ${ref.role} 角色不适用当前模式。`);
    if (ref.source.kind !== "pending_upload")
      for (const m of ref.source.kind === "subject"
        ? ref.source.media
        : [ref.source])
        physical.set(`${ref.role}/${m.sourceId}/${m.sourceVersion}`, 1);
  }
  for (const [name, role] of Object.entries(limits?.roles ?? {})) {
    const count = [...physical.keys()].filter((k) =>
      k.startsWith(name + "/"),
    ).length;
    if (count > (role.max_count ?? 255))
      issues.push(`${name} 超出参考数量上限。`);
    if (count < (role.min_count ?? 0)) issues.push(`${name} 需要补充参考。`);
  }
  try {
    const fields = fieldsForMode(
      model.current_version.param_schema,
      draft.mode ?? "",
    );
    for (const [key] of Object.entries(draft.params))
      if (key !== "output_count" && !fields.some((f) => f.field === key))
        issues.push(`参数 ${key} 不适用当前模式，需明确移除或调整。`);
    for (const f of fields) {
      const v = draft.params[f.field];
      if (f.required && (v === undefined || v === null || v === ""))
        issues.push(`请填写${f.label}。`);
      else if (
        v !== undefined &&
        v !== null &&
        ((f.enum && !f.enum.includes(v as string | number)) ||
          (typeof v === "number" &&
            ((f.min !== undefined && v < f.min) ||
              (f.max !== undefined && v > f.max) ||
              (f.type === "integer" && !Number.isInteger(v)))))
      )
        issues.push(`${f.label} 超出当前模型范围。`);
    }
  } catch {
    issues.push("模型参数定义不可用。");
  }
  const count = draft.params.output_count ?? 1;
  if (
    typeof count !== "number" ||
    !Number.isInteger(count) ||
    count < 1 ||
    count > Math.min(8, limits?.max_outputs ?? 1)
  )
    issues.push("候选数量超出当前模型范围。");
  if (
    !draft.document.content.some((p) =>
      p.content?.some(
        (n) => n.type === "reference" || (n.type === "text" && n.text.trim()),
      ),
    )
  )
    issues.push("请填写提示词。");
  return issues;
}
export function compatibleParams(draft: PromptDraft, model: API.ModelResponse) {
  const fields = fieldsForMode(
    model.current_version?.param_schema,
    draft.mode ?? "",
  );
  return {
    ...initialParams(fields),
    ...Object.fromEntries(
      Object.entries(draft.params).filter(
        ([k]) => k === "output_count" || fields.some((f) => f.field === k),
      ),
    ),
  };
}
