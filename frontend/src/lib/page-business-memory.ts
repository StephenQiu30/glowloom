import {
  getSessionGeneration,
  getScopeGeneration,
  subscribeContextChange,
} from "./request";

// Page-local request continuity. Never reads legacy browser records.
const values = new Map<string, string>();
let owner = "";
subscribeContextChange(() => values.clear());
function current() {
  const next = `${getSessionGeneration()}/${getScopeGeneration()}`;
  if (next !== owner) {
    values.clear();
    owner = next;
  }
  return values;
}
export const pageBusinessMemory = {
  get length() {
    return current().size;
  },
  getItem(key: string) {
    return current().get(key) ?? null;
  },
  setItem(key: string, value: string) {
    if (typeof window === "undefined")
      throw new Error("page_memory_unavailable");
    const records = current();
    const size = [...records].reduce(
      (n, [k, v]) => n + (k === key ? 0 : k.length + v.length),
      key.length + value.length,
    );
    if ((!records.has(key) && records.size >= 256) || size > 8 * 1024 * 1024)
      throw new Error("page_memory_budget");
    records.set(key, value);
  },
  removeItem(key: string) {
    current().delete(key);
  },
  clear() {
    values.clear();
  },
};
