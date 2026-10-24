import { pageBusinessMemory } from "@/lib/page-business-memory";
// UI intent only. The project service validates ownership and target on every command.
export function uploadTarget(owner: string, id: string, fallback: string) {
  const key = `asset-upload-target:${owner}:${id}`;
  const saved = pageBusinessMemory.getItem(key);
  const value = saved && /^(GENERAL|[1-9]\d*)$/.test(saved) ? saved : fallback;
  pageBusinessMemory.setItem(key, value);
  return value;
}
