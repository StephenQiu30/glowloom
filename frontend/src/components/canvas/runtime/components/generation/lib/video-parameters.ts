const legacyDefaults = new Set(["default", "-1"]);

function resolutionRank(value: string) {
  const amount = Number.parseFloat(value);
  if (!Number.isFinite(amount)) return Number.POSITIVE_INFINITY;
  return value.toLowerCase().endsWith("k") ? amount * 1000 : amount;
}

/** Use the selected model's capabilities instead of the catalog's common subset. */
export function videoParameterFields(
  fields: CanvasContract.Parameter[],
  capabilities?: CanvasContract.ModelNativeCapabilitiesVO,
): CanvasContract.Parameter[] {
  const min = capabilities?.minDuration;
  const max = capabilities?.maxDuration;
  return fields.flatMap((field) => {
    let values = (field.values ?? []).filter(
      (value) => !legacyDefaults.has(value),
    );
    if (field.key === "resolution" && capabilities?.resolutions)
      values = capabilities.resolutions;
    if (field.key === "ratio" && capabilities?.ratios)
      values = capabilities.ratios;
    if (
      field.key === "duration" &&
      typeof min === "number" &&
      typeof max === "number" &&
      Number.isInteger(min) &&
      Number.isInteger(max) &&
      min > 0 &&
      max >= min
    ) {
      values = Array.from({ length: max - min + 1 }, (_, index) =>
        String(min + index),
      );
    }
    values = values.filter((value) => !legacyDefaults.has(value));
    if (!values.length) return [];
    const defaultValue =
      field.key === "ratio"
        ? values.includes("16:9")
          ? "16:9"
          : (values.find((value) => value !== "adaptive") ?? values[0])
        : field.key === "resolution"
          ? [...values].sort(
              (left, right) => resolutionRank(left) - resolutionRank(right),
            )[0]
          : values[0];
    return [{ ...field, values, defaultValue }];
  });
}

/** Normalize only the removed ordinary-video sentinels; keep other drafts for validation. */
export function videoFieldValue(
  field: CanvasContract.Parameter,
  value?: string | number,
) {
  const selected = value == null ? "" : String(value);
  return !selected || legacyDefaults.has(selected)
    ? String(field.defaultValue ?? field.values?.[0] ?? "")
    : selected;
}

export function buildVideoParameters(
  draft: Record<string, string>,
  fields: CanvasContract.Parameter[],
): { parameters: Record<string, string | number>; error?: string } {
  const parameters: Record<string, string | number> = {};
  for (const field of fields) {
    if (!field.key) continue;
    const value = videoFieldValue(field, draft[field.key]);
    if (
      !field.values?.includes(value) ||
      (field.numeric && !Number.isFinite(Number(value)))
    )
      return {
        parameters,
        error: `当前模型不支持所选${field.label ?? field.key}，请重新选择。`,
      };
    parameters[field.key] = field.numeric ? Number(value) : value;
  }
  return { parameters };
}
