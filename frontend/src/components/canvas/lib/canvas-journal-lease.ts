import type { CollaborationScope } from "@glowloom/canvas-collab/session";

/** Epoch changes must not let another window take the same live editing session. */
export function claimCanvasJournal(
  scope: Omit<CollaborationScope, "epoch">,
): Promise<(() => Promise<void>) | null> {
  return new Promise((resolve, reject) => {
    const locks = globalThis.navigator?.locks;
    if (!locks) {
      reject(new Error("设备恢复协调不可用"));
      return;
    }
    const completed = locks.request(
      `glowloom-canvas-yjs:${JSON.stringify([
        scope.origin,
        scope.actorKey,
        scope.canvasId,
        scope.sessionId,
      ])}`,
      { ifAvailable: true },
      async (lock) => {
        if (!lock) {
          resolve(null);
          return;
        }
        await new Promise<void>((release) =>
          resolve(() => {
            release();
            return completed.then(() => {});
          }),
        );
      },
    );
    void completed.catch(reject);
  });
}
