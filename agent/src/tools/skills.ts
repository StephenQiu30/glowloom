import type { TextContent, ImageContent } from '@earendil-works/pi-ai';
import type { JsonObject } from '@earendil-works/pi-durable';
import type { BusinessClient, Grant } from '../business/client.js';
import type { FrozenManifest } from '../sessions/manifest.js';

export async function readPinnedSkill(
  client: BusinessClient,
  grant: Grant,
  manifest: Readonly<FrozenManifest>,
  id: string,
) {
  const pin = manifest.skills.find((s) => s.skill_id === id);
  if (
    !pin ||
    typeof pin.version !== 'string' ||
    typeof pin.resource_hash !== 'string'
  )
    throw new Error('Skill is not pinned to this run');
  await client.validate(grant);
  const loaded = await client.skill(grant, id, pin.version, pin.resource_hash);
  if (
    loaded.skill_id !== id ||
    loaded.resource_hash !== pin.resource_hash ||
    loaded.version !== pin.version ||
    typeof loaded.content !== 'string'
  )
    throw new Error('Skill resource changed');
  return {
    skill_id: id,
    version: loaded.version,
    resource_hash: loaded.resource_hash,
    content: loaded.content,
  };
}
/** Only the already authorized Go receipt may supply image bytes; there is no host file/shell fallback. */
export function businessToolContent(
  result: JsonObject,
): (TextContent | ImageContent)[] {
  if (!('content' in result))
    return [{ type: 'text', text: JSON.stringify(result) }];
  if (!Array.isArray(result.content) || !result.content.length)
    throw new Error('Invalid business tool content');
  return result.content.map((value) => {
    if (!value || typeof value !== 'object' || Array.isArray(value))
      throw new Error('Invalid business tool content');
    if (value.type === 'text' && typeof value.text === 'string')
      return { type: 'text' as const, text: value.text };
    if (
      value.type === 'image' &&
      typeof value.data === 'string' &&
      value.data.length > 0 &&
      /^[a-zA-Z0-9+/]+={0,2}$/.test(value.data) &&
      typeof value.mimeType === 'string' &&
      /^image\/(png|jpeg|webp|gif)$/.test(value.mimeType)
    )
      return {
        type: 'image' as const,
        data: value.data,
        mimeType: value.mimeType,
      };
    throw new Error('Unsupported business tool content');
  });
}
