// UI intent only. The project service validates ownership and target on every command.
const memory = new Map<string, string>();
export function uploadTarget(owner: string, id: string, fallback: string) {
  const key = `asset-upload-target:${owner}:${id}`;
  const saved = memory.get(key) ?? globalThis.localStorage?.getItem(key);
  const value = saved && /^(GENERAL|[1-9]\d*)$/.test(saved) ? saved : fallback;
  memory.set(key, value);
  globalThis.localStorage?.setItem(key, value);
  return value;
}
