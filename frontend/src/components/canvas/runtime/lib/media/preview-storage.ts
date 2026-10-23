/** Binary previews live for this browser session and do not replace the file service. */
export function createPreviewStorage() {
  const items = new Map<string, unknown>();
  return {
    async getItem<T>(key: string): Promise<T | null> {
      return (items.get(key) as T | undefined) ?? null;
    },
    async setItem<T>(key: string, value: T): Promise<T> {
      items.set(key, value);
      return value;
    },
    async removeItem(key: string) {
      items.delete(key);
    },
    async iterate<T, TResult = unknown>(
      callback: (value: T, key: string, index: number) => TResult,
    ) {
      let index = 0;
      for (const [key, value] of items) callback(value as T, key, ++index);
    },
    async keys() {
      return Array.from(items.keys());
    },
  };
}
