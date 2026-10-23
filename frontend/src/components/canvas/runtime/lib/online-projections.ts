/** Account-private stream ordering and resource versions; no timers discover business state. */
export type OnlineProjection = {
  type: "projection.changed";
  streamId: string;
  sequence: string | number;
  eventId: string;
  resourceType: string;
  resourceId: string;
  resourceVersion: string | number;
  payload: Record<string, unknown>;
};
export type ProjectionSignal =
  OnlineProjection | { type: "recover"; scope?: "search" };
const types = new Set([
  "credit.account",
  "credit.activity",
  "search.scope",
  "workflow.run",
  "generation.task",
  "canvas.source",
  "project.directory",
  "canvas.directory",
  "file.transfer",
  "file.delivery",
  "asset.import",
  "asset.export",
  "asset.review",
  "notification.mailbox",
]);
export function revision(value: unknown): bigint {
  if (
    (typeof value === "string" && /^\d{1,19}$/.test(value)) ||
    (typeof value === "number" && Number.isSafeInteger(value) && value >= 0)
  )
    return BigInt(value);
  return BigInt(-1);
}
export class ProjectionStream {
  private stream?: string;
  private sequence = BigInt(0);
  private listeners = new Set<(signal: ProjectionSignal) => void>();
  private values = new Map<string, OnlineProjection>();
  private clock = 0;
  private generation = 0;
  private searchGeneration = 0;
  private positions = new Map<string, number>();
  reset() {
    this.stream = undefined;
    this.sequence = BigInt(0);
    this.values.clear();
    this.positions.clear();
    this.generation++;
    this.searchGeneration++;
    this.clock++;
  }
  begin(stream: string) {
    if (this.stream === stream) return;
    this.reset();
    this.stream = stream;
    this.emit({ type: "recover" });
  }
  subscribe(listener: (signal: ProjectionSignal) => void) {
    this.listeners.add(listener);
    return () => {
      this.listeners.delete(listener);
    };
  }
  version() {
    return this.clock;
  }
  epoch(search = false) {
    return search ? this.searchGeneration : this.generation;
  }
  latest(after = -1) {
    return [...this.values.entries()]
      .filter(([key]) => (this.positions.get(key) ?? 0) > after)
      .map(([, event]) => event);
  }
  accept(value: unknown): boolean {
    if (!value || typeof value !== "object") return false;
    const frame = value as Record<string, unknown>;
    if (
      frame.type === "sync.required" &&
      frame.scope !== undefined &&
      frame.scope !== "search"
    )
      return false;
    if (frame.streamId !== this.stream) return true;
    const sequence = revision(frame.sequence);
    if (sequence < BigInt(1)) return false;
    if (sequence <= this.sequence) return true;
    const gap = sequence !== this.sequence + BigInt(1);
    this.sequence = sequence;
    if (!gap && frame.type === "sync.required" && frame.scope === "search") {
      this.recoverSearch();
      return true;
    }
    if (gap || frame.type === "sync.required") {
      this.generation++;
      this.searchGeneration++;
      this.clock++;
      this.emit({ type: "recover" });
    }
    if (frame.type === "sync.required") return true;
    if (
      frame.type !== "projection.changed" ||
      typeof frame.resourceType !== "string" ||
      !types.has(frame.resourceType) ||
      typeof frame.resourceId !== "string" ||
      typeof frame.eventId !== "string" ||
      revision(frame.resourceVersion) < BigInt(1) ||
      !frame.payload ||
      typeof frame.payload !== "object" ||
      Array.isArray(frame.payload)
    )
      return false;
    const event = frame as OnlineProjection;
    const key = event.resourceType + ":" + event.resourceId;
    const previous = this.values.get(key);
    if (
      previous &&
      (event.resourceType !== "notification.mailbox" ||
        previous.payload.dataEpoch === event.payload.dataEpoch) &&
      revision(previous.resourceVersion) >= revision(event.resourceVersion)
    )
      return true;
    if (this.values.size >= 4096 && !previous) {
      if (
        event.resourceType === "search.scope" ||
        [...this.values.values()].some(
          (value) => value.resourceType === "search.scope",
        )
      )
        this.recoverSearch();
      if (this.values.size >= 4096) {
        if (event.resourceType === "search.scope") {
          this.emit(event);
          return true;
        }
        this.values.clear();
        this.positions.clear();
        this.generation++;
        this.searchGeneration++;
        this.clock++;
        this.emit({ type: "recover" });
      }
    }
    this.values.set(key, event);
    this.clock++;
    this.positions.set(key, this.clock);
    this.emit(event);
    return true;
  }
  private recoverSearch() {
    for (const [key, value] of this.values) {
      if (value.resourceType === "search.scope") {
        this.values.delete(key);
        this.positions.delete(key);
      }
    }
    this.searchGeneration++;
    this.emit({ type: "recover", scope: "search" });
  }
  private emit(event: ProjectionSignal) {
    for (const listener of this.listeners) listener(event);
  }
}
export const onlineProjections = new ProjectionStream();

/** A waiting upload follows pushed state, with one recovery read per stream recovery. */
export async function waitForFileTransfer<
  T extends { status?: string; syncRevision?: string | number },
>(
  id: string,
  initial: T,
  read: () => Promise<T>,
  signal: AbortSignal,
): Promise<T> {
  if (!["VERIFYING", "INITIALIZING"].includes(initial.status ?? ""))
    return initial;
  return new Promise<T>((resolve, reject) => {
    let current = initial;
    let busy = false;
    let attempts = 0;
    let finished = false;
    let timer: ReturnType<typeof setTimeout> | undefined;
    let epoch = onlineProjections.epoch();
    const finish = (error?: unknown) => {
      if (finished) return;
      finished = true;
      unsubscribe();
      clearTimeout(deadline);
      clearTimeout(timer);
      signal.removeEventListener("abort", aborted);
      if (error) reject(error);
      else resolve(current);
    };
    const aborted = () =>
      finish(signal.reason ?? new DOMException("Aborted", "AbortError"));
    const refresh = async () => {
      timer = undefined;
      if (finished || busy || attempts >= 3) return;
      busy = true;
      attempts++;
      try {
        const readingEpoch = onlineProjections.epoch();
        const next = await read();
        if (finished) return;
        if (readingEpoch !== onlineProjections.epoch())
          throw new Error("连接已更新");
        if (
          epoch !== readingEpoch ||
          revision(next.syncRevision) >= revision(current.syncRevision)
        )
          current = next;
        epoch = readingEpoch;
        attempts = 0;
        check();
      } catch (error) {
        if (attempts >= 3) finish(error);
        else if (!finished)
          timer = setTimeout(() => {
            void refresh();
          }, attempts * 1000);
      } finally {
        busy = false;
      }
    };
    const check = () => {
      if (!["VERIFYING", "INITIALIZING"].includes(current.status ?? ""))
        finish();
    };
    const unsubscribe = onlineProjections.subscribe((event) => {
      if (event.type === "recover") {
        if (event.scope === "search") return;
        if (!timer) void refresh();
        return;
      }
      if (
        event.resourceType !== "file.transfer" ||
        event.resourceId !== id ||
        revision(event.resourceVersion) <= revision(current.syncRevision)
      )
        return;
      current = {
        ...current,
        ...event.payload,
        syncRevision: event.resourceVersion,
      };
      check();
    });
    const deadline = setTimeout(
      () => finish(new Error("文件仍在核验，完成后可从上传记录继续")),
      60000,
    );
    signal.addEventListener("abort", aborted, { once: true });
    if (signal.aborted) aborted();
    else {
      const latest = onlineProjections
        .latest()
        .find(
          (event) =>
            event.resourceType === "file.transfer" && event.resourceId === id,
        );
      if (
        latest &&
        revision(latest.resourceVersion) > revision(current.syncRevision)
      ) {
        current = {
          ...current,
          ...latest.payload,
          syncRevision: latest.resourceVersion,
        };
        check();
      }
    }
  });
}
