export function toNarrativeReferenceInput(
  reference: API.NarrativeReferenceResponse,
): API.NarrativeReferenceInput {
  return {
    unit_version_id: reference.unit_version_id,
    channel: reference.channel,
    role: reference.role,
    coverage_mode: reference.coverage_mode,
    segment_start: reference.segment_start,
    segment_end: reference.segment_end,
    contribution: reference.contribution,
  };
}

export function narrativeReferenceKey(
  reference: API.NarrativeReferenceInput | API.NarrativeReferenceResponse,
): string {
  return JSON.stringify([
    reference.unit_version_id,
    reference.channel,
    reference.segment_start,
    reference.segment_end,
  ]);
}
