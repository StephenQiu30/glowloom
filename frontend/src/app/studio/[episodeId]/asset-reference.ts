export function toAssetReferenceRequest(
  reference: API.AssetReferenceResponse,
): API.AssetReferenceRequest {
  return {
    slot_key: reference.slot_key,
    role: reference.role,
    asset_version_id: reference.asset_version_id,
    subject_key: reference.subject_key,
  };
}
