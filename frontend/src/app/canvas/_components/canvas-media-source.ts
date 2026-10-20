import {
  getMediaPreview,
  listMediaLibrary,
  downloadLibraryMediaAsset,
} from "@/api/media";
import { RequestError } from "@/lib/request";
import { isProjectID } from "@/components/projects/project";

/** Resolve the actual library identity before downloading through the generated client. */
export async function canvasMediaSource(
  projectID: string,
  assetID: string,
  signal: AbortSignal,
) {
  const preview = await getMediaPreview(
    { pid: projectID, asset_id: assetID },
    { signal },
  );
  if (preview.asset?.id !== assetID || preview.asset.project_id !== projectID)
    throw new RequestError("invalid_receipt");
  for (let page = 1; ; page++) {
    const result = await listMediaLibrary(
      {
        scope: "project",
        project_id: projectID,
        kind: preview.asset.kind,
        page,
        page_size: 120,
      },
      { signal },
    );
    if (
      result.scope?.kind !== "project" ||
      result.scope.project_id !== projectID ||
      !Array.isArray(result.items) ||
      !Number.isSafeInteger(result.total) ||
      result.total! < 0 ||
      result.page !== page
    )
      throw new RequestError("invalid_receipt");
    const item = result.items.find(
      (item) => item.asset_id === assetID && item.catalog_state === "active",
    );
    if (item) {
      if (!isProjectID(item.id)) throw new RequestError("invalid_receipt");
      const blob = await downloadLibraryMediaAsset(
        { item_id: item.id, scope: "project", project_id: projectID },
        { signal, responseType: "blob" },
      );
      if (!(blob instanceof Blob) || !blob.size)
        throw new RequestError("invalid_receipt");
      return { blob, asset: preview.asset };
    }
    if (!result.items.length || page * 120 >= result.total!)
      throw new Error("该素材已不在当前项目素材库，请重新选择来源。");
  }
}
