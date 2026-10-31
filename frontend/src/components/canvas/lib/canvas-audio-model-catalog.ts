import { listProjectModels } from "@/api/models";
import {
  getCreationScope,
  getScopeGeneration,
  getSessionGeneration,
} from "@/lib/request";
import type {
  CanvasAudioCatalogIdentity,
  CanvasAudioModelCatalog,
} from "./canvas-audio-connection-capability";

/** Formal catalog only: no legacy runtime config-store or inferred supplier capabilities. */
export async function readCanvasAudioModelCatalog(
  identity: CanvasAudioCatalogIdentity,
  signal?: AbortSignal,
): Promise<CanvasAudioModelCatalog & { status: "ready" }> {
  const { projectID, scopeID } = identity;
  const session = getSessionGeneration();
  const scope = getScopeGeneration();
  const assertCurrent = () => {
    if (
      !projectID ||
      !scopeID ||
      getCreationScope() !== scopeID ||
      session !== getSessionGeneration() ||
      scope !== getScopeGeneration() ||
      signal?.aborted
    )
      throw new Error("项目或登录状态已变化，请重新读取音频模型");
  };
  assertCurrent();
  const models: API.ModelResponse[] = [];
  const seen = new Set<string>();
  let cursor: string | undefined;
  do {
    assertCurrent();
    const page = await listProjectModels(
      {
        project_id: projectID,
        capability: "audio.generate",
        limit: 100,
        ...(cursor ? { cursor } : {}),
      },
      { signal },
    );
    assertCurrent();
    if (
      !Array.isArray(page.items) ||
      (page.next_cursor !== undefined && typeof page.next_cursor !== "string")
    )
      throw new Error("音频模型目录返回无效");
    models.push(...page.items);
    cursor = page.next_cursor;
    if (cursor) {
      if (seen.has(cursor)) throw new Error("音频模型目录分页重复，请重试");
      seen.add(cursor);
    }
  } while (cursor);
  return { projectID, scopeID, status: "ready", models };
}
