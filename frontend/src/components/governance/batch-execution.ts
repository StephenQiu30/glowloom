import { RequestError } from "@/lib/request";
import { authErrorMessage } from "@/components/auth/contracts";
export type BatchStatus =
  | "ready"
  | "running"
  | "success"
  | "submitted"
  | "failed"
  | "unknown"
  | "skipped";
export type BatchEntry<T> = {
  id: string;
  label: string;
  row: T;
  requestId: string;
  status: BatchStatus;
  message: string;
};
export type BatchOutcome = {
  message: string;
  status?: "success" | "submitted" | "skipped";
};
export function uncertain(error: unknown) {
  return (
    !(error instanceof RequestError) ||
    error.status === 0 ||
    error.status >= 500 ||
    error.status === 408
  );
}
// Writes remain sequential: later records may depend on authority affected by earlier ones.
export async function executeBatch<T>(
  entries: BatchEntry<T>[],
  run: (entry: BatchEntry<T>) => Promise<BatchOutcome>,
  update: (entry: BatchEntry<T>) => void,
  signal: AbortSignal,
) {
  let forbidden = false;
  for (const entry of entries) {
    if (signal.aborted) break;
    if (entry.status !== "ready") continue;
    if (forbidden) {
      update({
        ...entry,
        status: "skipped",
        message: "权限已失效，未发送此项操作。",
      });
      continue;
    }
    update({ ...entry, status: "running", message: "正在处理…" });
    try {
      const result = await run(entry);
      if (!signal.aborted)
        update({
          ...entry,
          status: result.status ?? "success",
          message: result.message,
        });
    } catch (error) {
      if (signal.aborted) break;
      forbidden =
        error instanceof RequestError && [401, 403].includes(error.status);
      update({
        ...entry,
        status: uncertain(error) ? "unknown" : "failed",
        message: uncertain(error)
          ? "响应未确认，请查询回执或使用原请求标识恢复。"
          : authErrorMessage(error),
      });
    }
  }
}
