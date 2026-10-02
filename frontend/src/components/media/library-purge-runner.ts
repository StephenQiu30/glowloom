import type { LibraryIdentity } from "./library-model";
import { purgeTerminal, type PurgeJob } from "./library-purge-model";
import { getLibraryPurge } from "./library-purge-query";
export function requirePurgeForeground(
  identity: LibraryIdentity,
  signal: AbortSignal,
) {
  signal.throwIfAborted();
  if (
    document.visibilityState !== "visible" ||
    window.location.origin !== identity.origin
  )
    throw new Error(
      "页面已离开前台或范围改变，完整清理计划已停止。回到原页面后请明确继续并重新核验。",
    );
}
function pause(signal: AbortSignal) {
  return new Promise<void>((resolve, reject) => {
    signal.throwIfAborted();
    const abort = () => {
      clearTimeout(timer);
      signal.removeEventListener("abort", abort);
      reject(new Error("完整计划已停止，请保留原子键并明确核验后继续。"));
    };
    const timer = setTimeout(() => {
      signal.removeEventListener("abort", abort);
      resolve();
    }, 1500);
    signal.addEventListener("abort", abort, { once: true });
  });
}
export async function waitPurgeTerminal(
  identity: LibraryIdentity,
  job: PurgeJob,
  signal: AbortSignal,
) {
  const started = Date.now();
  while (true) {
    requirePurgeForeground(identity, signal);
    const actual = await getLibraryPurge(identity, job.id, signal);
    requirePurgeForeground(identity, signal);
    if (
      actual.needs_reconciliation ||
      actual.execution_unconfirmed ||
      actual.status === "needs_reconciliation"
    )
      throw new Error(
        "上一子批结果未知或执行未确认，计划已停止。保留原键，明确对账/取消后再核验真实终态。",
      );
    if (purgeTerminal(actual)) return actual;
    if (Date.now() - started > 10 * 60 * 1000)
      throw new Error(
        "原子批等待超过本次前台核验时限，计划已停止。请核对实际状态后明确继续。",
      );
    await pause(signal);
  }
}
