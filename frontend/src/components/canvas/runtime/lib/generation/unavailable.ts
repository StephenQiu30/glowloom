/** Generation awaits the project → agent contract; no provider calls happen in a browser. */
export async function unavailableGeneration<T>(
  signal?: AbortSignal,
): Promise<T> {
  if (signal?.aborted) throw new DOMException("请求已取消", "AbortError");
  throw new Error("生成服务尚未接入");
}
